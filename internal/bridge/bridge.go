package bridge

import (
	"context"
	"fmt"
	"os"
	"whats-gtk/internal/backend"
	"whats-gtk/internal/core"
	"whats-gtk/internal/database"
	"whats-gtk/internal/events"
	"whats-gtk/internal/ui"
	"whats-gtk/internal/ui/chat"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"go.mau.fi/whatsmeow/types"
	meowEvents "go.mau.fi/whatsmeow/types/events"
)

// Bridge is the thin orchestrator that wires together all sub-services.
// It creates, configures, and connects: EventHandler, ChatController,
// MessageService, Renderer, ContactService, MediaService, InputManager,
// and MessagePipeline.
type Bridge struct {
	Backend  *backend.Backend
	App      *ui.App
	DB       *database.AppDB
	EventBus *events.EventBus
	ctx      context.Context

	// Sub-services
	Events   *EventHandler
	Chat     *ChatController
	Messages *MessageService
	Render   *Renderer
	Contacts *ContactService
	Media    *MediaService
	Input    *core.InputManager
	Pipeline *core.MessagePipeline
}

// NewBridge creates all sub-services and wires them together.
func NewBridge(b *backend.Backend, a *ui.App, db *database.AppDB, ctx context.Context, bus *events.EventBus) *Bridge {
	pipeline := core.NewMessagePipeline()
	input := core.NewInputManager()
	contacts := NewContactService(b, db, ctx)
	media := NewMediaService(b, db, ctx)
	msgs := NewMessageService(db, b, contacts, a, ctx)
	rend := NewRenderer(a, db, contacts, b, msgs, ctx, bus)
	chat := NewChatController(b, a, db, msgs, contacts, media, ctx)
	evts := NewEventHandler(b, a, db, msgs, chat, contacts, media, rend, pipeline, ctx)

	// Wire back-references (these can't be set in constructors due to circular init)
	rend.Chat = chat
	chat.Renderer = rend
	msgs.GetSelectedJID = chat.SelectedJID

	br := &Bridge{
		Backend:  b,
		App:      a,
		DB:       db,
		EventBus: bus,
		ctx:      ctx,
		Events:   evts,
		Chat:     chat,
		Messages: msgs,
		Render:   rend,
		Contacts: contacts,
		Media:    media,
		Input:    input,
		Pipeline: pipeline,
	}

	br.registerDefaultHooks()
	br.setupUIHandlers()
	br.setupServiceHandlers()

	return br
}

// Start is the application entry point: creates the media directory,
// sets the event handler, connects the backend, and loads the initial sidebar.
func (br *Bridge) Start(ctx context.Context) {
	os.MkdirAll("media", 0755)
	br.Backend.SetEventHandler(br.Events.HandleEvent)
	br.Backend.Connect()
	go func() {
		c, err := br.DB.GetAllContacts(100)
		if err == nil && len(c) > 0 {
			br.Render.RefreshSidebar(c)
		}
	}()
}

// registerDefaultHooks registers pipeline hooks for auto-downloading stickers
// and rendering incoming messages to the UI.
func (br *Bridge) registerDefaultHooks() {
	// Hook to auto-download stickers
	br.Pipeline.AddHook(func(ctx context.Context, msg *meowEvents.Message) error {
		if stkr := msg.Message.GetStickerMessage(); stkr != nil {
			go br.Chat.HandleDownloadMedia(msg.Info.ID)
		}
		return nil
	})

	// Hook to publish incoming messages to EventBus
	br.Pipeline.AddHook(func(ctx context.Context, msg *meowEvents.Message) error {
		br.EventBus.Publish(events.Event{
			Type: events.EventMessageReceived,
			Data: events.LiveMessagePayload{
				Msg:       msg,
				IsSyncing: br.Events.IsSyncing(),
			},
		})
		return nil
	})
}

// setupUIHandlers wires all UI callbacks to the ChatController and InputManager.
func (br *Bridge) setupUIHandlers() {
	br.App.Sidebar.OnChatSelected = br.Chat.HandleChatSelected
	br.App.Sidebar.OnSearch = br.Chat.HandleSearch
	br.WireChatView(br.App.ChatView)
	br.App.Window.Connect("notify::is-active", br.Chat.HandleWindowActive)

	br.App.OnKeyPressed = br.handleKeyPressed
	br.App.OnSearchRequested = func() {
		var searchDialog *ui.SearchDialog
		onSearch := func(query string) {
			if query == "" {
				glib.IdleAdd(func() { searchDialog.Populate(nil, nil) })
				return
			}

			glib.IdleAdd(func() { searchDialog.SetLoading(true) })

			go func() {
				msgs, err := br.DB.SearchMessages(query, 50)
				if err != nil {
					fmt.Println("Error searching messages:", err)
					glib.IdleAdd(func() { searchDialog.SetLoading(false) })
					return
				}
				
				glib.IdleAdd(func() {
					if msgs == nil {
						msgs = []database.Message{}
					}
					searchDialog.Populate(msgs, func(msg database.Message) {
						targetJID, _ := types.ParseJID(msg.ChatJID)
						br.Chat.HandleChatSelected(msg.ChatJID)
						br.Render.RefreshMessagesAround(targetJID, msg.ID)
					})
				})
			}()
		}
		glib.IdleAdd(func() {
			searchDialog = ui.NewSearchDialog(&br.App.Window.Window, onSearch, nil)
			searchDialog.Window.Present()
		})
	}
	br.App.OnModifiersChanged = func(mods gdk.ModifierType) {
		show := mods&gdk.ControlMask != 0
		glib.IdleAdd(func() {
			if br.App.Sidebar != nil {
				br.App.Sidebar.ShowIndices(show)
			}
		})
	}

	// Register some default shortcuts
	br.Input.Register("Escape", func() {
		glib.IdleAdd(func() {
			br.App.Sidebar.SearchEntry.SetText("")
			br.App.ChatView.FocusEntry()
		})
	})

	// Ctrl+1 to Ctrl+9 to open chats by index
	for i := 1; i <= 9; i++ {
		idx := i - 1
		key := fmt.Sprintf("Control+%d", i)
		br.Input.Register(key, func() {
			glib.IdleAdd(func() {
				br.App.Sidebar.SelectIndex(idx)
			})
		})
	}

	closeChat := func() {
		glib.IdleAdd(func() {
			br.Chat.selectedJID = nil
			br.App.ActiveMainJID = ""
			if br.App.Sidebar != nil {
				br.App.Sidebar.ClearSelection()
			}
			if br.App.ChatView != nil {
				br.App.ChatView.Clear()
				br.App.ChatView.SetHeader("WhatsApp GTK", nil)
			}
		})
	}

	// Close chat when window is hidden/minimized to tray
	br.App.Window.Connect("hide", func() {
		closeChat()
	})

	// Ctrl+0 to return to initial screen
	br.Input.Register("Control+0", func() {
		fmt.Println("Bridge: Ctrl+0 triggered, returning to home screen")
		closeChat()
	})

	searchToggle := func() {
		glib.IdleAdd(func() {
			if br.App.ChatView != nil {
				br.App.ChatView.ToggleSearch()
			}
		})
	}
	br.Input.Register("Control+f", searchToggle)
	br.Input.Register("Control+F", searchToggle)

	// Ctrl+Tab and Ctrl+Shift+Tab for next/prev chat
	br.Input.Register("Control+Tab", func() {
		glib.IdleAdd(func() {
			br.App.Sidebar.SelectOffset(1)
		})
	})
	br.Input.Register("Control+Shift+Tab", func() {
		glib.IdleAdd(func() {
			br.App.Sidebar.SelectOffset(-1)
		})
	})
}

// setupServiceHandlers wires ContactService and MediaService callbacks to the UI.
func (br *Bridge) setupServiceHandlers() {
	br.Contacts.SetOnAvatarSet(func(jid string, tex *gdk.Texture) {
		br.App.ChatView.SetAvatar(jid, tex)
		br.App.Sidebar.SetAvatar(jid, tex)
	})

	br.Media.SetOnMediaDownloaded(func(task DownloadTask, data []byte, path string) {
		glib.IdleAdd(func() {
			selectedJID := br.Chat.SelectedJID()
			if selectedJID != nil && selectedJID.ToNonAD().String() == task.ChatJID {
				if task.MsgType == "audio" {
					br.App.ChatView.UpdateMessageAudio(task.ID, path)
					return
				}
				if task.MsgType == "document" {
					br.App.ChatView.UpdateMessageDocument(task.ID, path)
					return
				}
				if task.MsgType == "sticker" {
					var anim *gdkpixbuf.PixbufAnimation
					var tex *gdk.Texture
					
					anim, _ = gdkpixbuf.NewPixbufAnimationFromFile(path)
					if anim != nil && anim.IsStaticImage() {
						tex = gdk.NewTextureForPixbuf(anim.StaticImage())
						anim = nil
					}
					br.App.ChatView.UpdateMessageSticker(task.ID, anim, tex, path)
					return
				}

				pixbuf, _ := gdkpixbuf.NewPixbufFromFile(path)
				if pixbuf == nil { return }
				tex := gdk.NewTextureForPixbuf(pixbuf)

				br.App.ChatView.UpdateMessageImage(task.ID, tex, path)
			}
		})
	})
}

// handleKeyPressed builds a key combo string and delegates to InputManager.
func (br *Bridge) handleKeyPressed(key string, mods gdk.ModifierType) bool {
	combo := ""
	if mods&gdk.ControlMask != 0 {
		combo += "Control+"
	}
	if mods&gdk.AltMask != 0 {
		combo += "Alt+"
	}
	if mods&gdk.ShiftMask != 0 {
		combo += "Shift+"
	}
	combo += key

	return br.Input.HandleKeyPressed(combo)
}

func (br *Bridge) WireChatView(cv *chat.ChatView) {
	cv.OnSendMessage = func(text, replyToID string) {
		if jid := br.Chat.SelectedJID(); jid != nil {
			br.Chat.HandleSendMessage(*jid, text, replyToID)
		}
	}
	cv.OnPasteImage = func(tex *gdk.Texture) {
		if jid := br.Chat.SelectedJID(); jid != nil {
			br.Chat.HandlePasteImage(*jid, tex)
		}
	}
	cv.OnSendFile = func(path string) {
		if jid := br.Chat.SelectedJID(); jid != nil {
			br.Chat.HandleSendFile(*jid, path)
		}
	}
	cv.OnSendReaction = func(id, emoji string) {
		if jid := br.Chat.SelectedJID(); jid != nil {
			br.Chat.HandleSendReaction(*jid, id, emoji)
		}
	}
	cv.OnLoadOlder = func() {
		if jid := br.Chat.SelectedJID(); jid != nil && !cv.IsSearching {
			br.Render.LoadOlderMessages(jid.ToNonAD().String(), cv, "")
		}
	}
	cv.OnLoadMessageRequest = func(id string) {
		if jid := br.Chat.SelectedJID(); jid != nil && !cv.IsSearching {
			br.Render.LoadOlderMessages(jid.ToNonAD().String(), cv, id)
		}
	}
	cv.OnSearchMessages = func(query string) {
		if jid := br.Chat.SelectedJID(); jid != nil {
			br.Render.RenderMessageSearch(jid.ToNonAD().String(), query)
		}
	}
	cv.OnCancelSearch = func() {
		if jid := br.Chat.SelectedJID(); jid != nil {
			br.Render.CancelMessageSearch(jid.ToNonAD().String())
		}
	}
	cv.OnSearchResultClick = func(id string) {
		if jid := br.Chat.SelectedJID(); jid != nil {
			glib.IdleAdd(func() {
				cv.SearchBar.SetSearchMode(false)
			})
			br.Render.CancelMessageSearchAndJump(jid.ToNonAD().String(), id)
		}
	}
	cv.OnMentionClick = func(jid string) {
		glib.IdleAdd(func() {
			if br.App.Sidebar != nil {
				br.App.Sidebar.SelectChat(jid)
			}
		})
	}
	cv.OnSendPollVote = func(msgID string, senderJID string, isFromMe bool, selectedOptions []string) {
		if jid := br.Chat.SelectedJID(); jid != nil {
			sender, _ := types.ParseJID(senderJID)
			br.Backend.SendPollVote(context.Background(), *jid, msgID, sender, isFromMe, selectedOptions)
		}
	}
	cv.OnPinMessage = func(id string, pin bool, duration uint32) {
		if jid := br.Chat.SelectedJID(); jid != nil {
			br.Chat.HandlePinMessage(*jid, id, pin, duration)
		}
	}

	cv.OnDownloadMedia = br.Chat.HandleDownloadMedia
	cv.OnOpenImage = br.Chat.HandleOpenImage
	cv.OnDetach = br.Chat.HandleDetach
}
