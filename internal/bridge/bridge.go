package bridge

import (
	"context"
	"fmt"
	"os"
	"whats-gtk/internal/backend"
	"whats-gtk/internal/core"
	"whats-gtk/internal/database"
	"whats-gtk/internal/ui"
	"whats-gtk/internal/ui/chat"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"go.mau.fi/whatsmeow/types/events"
)

// Bridge is the thin orchestrator that wires together all sub-services.
// It creates, configures, and connects: EventHandler, ChatController,
// MessageService, Renderer, ContactService, MediaService, InputManager,
// and MessagePipeline.
type Bridge struct {
	Backend  *backend.Backend
	App      *ui.App
	DB       *database.AppDB
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
func NewBridge(b *backend.Backend, a *ui.App, db *database.AppDB, ctx context.Context) *Bridge {
	pipeline := core.NewMessagePipeline()
	input := core.NewInputManager()
	contacts := NewContactService(b, db, ctx)
	media := NewMediaService(b, db, ctx)
	msgs := NewMessageService(db, b, contacts, a, ctx)
	rend := NewRenderer(a, db, contacts, b, msgs, ctx)
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
	br.Pipeline.AddHook(func(ctx context.Context, msg *events.Message) error {
		if stkr := msg.Message.GetStickerMessage(); stkr != nil {
			go br.Chat.HandleDownloadMedia(msg.Info.ID)
		}
		return nil
	})

	// Hook to render incoming messages in the UI
	br.Pipeline.AddHook(func(ctx context.Context, msg *events.Message) error {
		br.Render.RenderLiveMessage(msg, br.Events.IsSyncing())
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

	// Ctrl+0 to return to initial screen
	br.Input.Register("Control+0", func() {
		fmt.Println("Bridge: Ctrl+0 triggered, returning to home screen")
		glib.IdleAdd(func() {
			br.Chat.selectedJID = nil
			if br.App.Sidebar != nil {
				br.App.Sidebar.ClearSelection()
			}
			if br.App.ChatView != nil {
				br.App.ChatView.Clear()
				br.App.ChatView.SetHeader("WhatsApp GTK", nil)
			}
		})
	})

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
	cv.OnPinMessage = func(id string, pin bool, duration uint32) {
		if jid := br.Chat.SelectedJID(); jid != nil {
			br.Chat.HandlePinMessage(*jid, id, pin, duration)
		}
	}

	cv.OnDownloadMedia = br.Chat.HandleDownloadMedia
	cv.OnOpenImage = br.Chat.HandleOpenImage
	cv.OnDetach = br.Chat.HandleDetach
}
