package bridge

import (
	"context"
	"fmt"
	"os"
	"time"
	"whats-gtk/internal/backend"
	"whats-gtk/internal/core"
	"whats-gtk/internal/database"
	"whats-gtk/internal/events"
	"whats-gtk/internal/notifications"
	"whats-gtk/internal/paths"
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
// MessageService, ContactService, MediaService, InputManager,
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
	Contacts *ContactService
	Media    *MediaService
	Input    *core.InputManager
	Pipeline *core.MessagePipeline
	Notifier *notifications.Notifier
}

// NewBridge creates all sub-services and wires them together.
func NewBridge(b *backend.Backend, a *ui.App, db *database.AppDB, ctx context.Context, bus *events.EventBus) *Bridge {
	pipeline := core.NewMessagePipeline()
	input := core.NewInputManager()
	contacts := NewContactService(b, db, ctx)
	media := NewMediaService(b, db, ctx)
	msgs := NewMessageService(db, b, contacts, a, ctx)
	var myJID string
	if b != nil && b.Client != nil && b.Client.Store != nil && b.Client.Store.ID != nil {
		myJID = b.Client.Store.ID.ToNonAD().String()
	}
	mapper := NewViewMapper(contacts, db, myJID)
	chat := NewChatController(b, a, db, msgs, contacts, media, ctx, bus, mapper)
	evts := NewEventHandler(b, a, db, msgs, chat, contacts, media, pipeline, ctx)
	notifier := notifications.NewNotifier(func(chatJID string) {
		glib.IdleAdd(func() {
			if a != nil && a.Window != nil {
				a.Window.Present()
			}
			if a != nil && a.Sidebar != nil {
				a.Sidebar.SelectChat(chatJID)
			}
			if chat != nil {
				chat.HandleChatSelected(chatJID)
			}
		})
	})

	// Wire back-references (these can't be set in constructors due to circular init)
	msgs.GetSelectedJID = chat.SelectedJID
	a.ResolveJIDFunc = msgs.ResolveJIDString

	br := &Bridge{
		Backend:  b,
		App:      a,
		DB:       db,
		EventBus: bus,
		ctx:      ctx,
		Events:   evts,
		Chat:     chat,
		Messages: msgs,
		Contacts: contacts,
		Media:    media,
		Input:    input,
		Pipeline: pipeline,
		Notifier: notifier,
	}

	br.registerDefaultHooks()
	br.setupUIHandlers()
	br.setupServiceHandlers()

	return br
}

// Start is the application entry point: creates the media directory,
// sets the event handler, connects the backend, and loads the initial sidebar.
func (br *Bridge) Start(ctx context.Context) {
	_ = os.MkdirAll(paths.MediaDir(), 0755)
	br.Backend.SetEventHandler(br.Events.HandleEvent)
	br.Backend.Connect()
	go func() {
		// Initial sync
		br.RefreshProfilesUI()
		br.Chat.RefreshSidebarUI()
	}()
}

// Shutdown gracefully finishes pending database sync operations and disconnects.
func (br *Bridge) Shutdown() {
	if br.Chat != nil {
		br.Chat.StopAllTyping()
	}
	if br.Events != nil {
		br.Events.WaitSync(3 * time.Second)
	}
	if br.Backend != nil {
		br.Backend.Disconnect()
	}
}

func (br *Bridge) RefreshProfilesUI() {
	profiles, _ := br.DB.GetProfiles()
	activeID := br.Chat.GetActiveProfileID()
	glib.IdleAdd(func() {
		if br.App.Sidebar != nil {
			br.App.Sidebar.SetProfiles(profiles, activeID)
		}
	})
}

// registerDefaultHooks registers pipeline hooks for auto-downloading stickers,
// dispatching events, and sending desktop notifications.
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

	// Hook to send desktop notifications when app window is inactive or in background
	br.Pipeline.AddHook(func(ctx context.Context, msg *meowEvents.Message) error {
		if msg.Info.IsFromMe || br.Events.IsSyncing() || br.Notifier == nil {
			return nil
		}
		chatJID := br.Messages.ResolveJID(msg.Info.Chat).ToNonAD().String()
		selJID := br.Chat.SelectedJID()
		isCurrentChat := selJID != nil && selJID.ToNonAD().String() == chatJID
		if br.App.Window.IsActive() && isCurrentChat {
			return nil
		}
		if br.App.DetachedWindows != nil {
			if dWin, ok := br.App.DetachedWindows[chatJID]; ok && dWin != nil && dWin.IsActive() {
				return nil
			}
		}

		// Archived chats must NOT send notifications
		if contact, err := br.DB.GetContact(chatJID); err == nil && contact != nil && contact.IsArchived {
			return nil
		}
		rawChatJID := msg.Info.Chat.ToNonAD().String()
		if rawChatJID != chatJID {
			if contact, err := br.DB.GetContact(rawChatJID); err == nil && contact != nil && contact.IsArchived {
				return nil
			}
		}

		senderJID := br.Messages.ResolveJID(msg.Info.Sender).ToNonAD().String()
		isGroup := msg.Info.Chat.Server == types.GroupServer

		title := ""
		bodyPrefix := ""

		if isGroup {
			groupName := br.Contacts.GetCleanName(chatJID)
			if groupName == "" || groupName == chatJID {
				if c, err := br.DB.GetContact(chatJID); err == nil && c.DisplayName() != "" {
					groupName = c.DisplayName()
				}
			}
			if groupName == "" {
				groupName = "Grupo"
			}
			senderName := br.Contacts.GetCleanName(senderJID)
			if senderName == "" || senderName == senderJID {
				if msg.Info.PushName != "" {
					senderName = msg.Info.PushName
				}
			}
			title = groupName
			bodyPrefix = senderName + ": "
		} else {
			contactName := br.Contacts.GetCleanName(chatJID)
			if contactName == "" || contactName == chatJID {
				if msg.Info.PushName != "" {
					contactName = msg.Info.PushName
				}
			}
			title = contactName
		}

		msgText := ""
		if msg.Message.GetConversation() != "" {
			msgText = msg.Message.GetConversation()
		} else if ext := msg.Message.GetExtendedTextMessage(); ext != nil {
			msgText = ext.GetText()
		} else if img := msg.Message.GetImageMessage(); img != nil {
			if img.GetCaption() != "" {
				msgText = "📷 " + img.GetCaption()
			} else {
				msgText = "📷 Foto"
			}
		} else if vid := msg.Message.GetVideoMessage(); vid != nil {
			if vid.GetCaption() != "" {
				msgText = "🎥 " + vid.GetCaption()
			} else {
				msgText = "🎥 Vídeo"
			}
		} else if msg.Message.GetAudioMessage() != nil {
			msgText = "🎵 Áudio"
		} else if doc := msg.Message.GetDocumentMessage(); doc != nil {
			if doc.GetFileName() != "" {
				msgText = "📄 " + doc.GetFileName()
			} else {
				msgText = "📄 Documento"
			}
		} else if msg.Message.GetStickerMessage() != nil {
			msgText = "🏷️ Figurinha"
		} else if pm := br.Messages.GetPollCreationMessage(msg.Message); pm != nil {
			msgText = "📊 Enquete: " + pm.GetName()
		} else {
			msgText = "Nova mensagem"
		}

		body := bodyPrefix + msgText
		_ = br.Notifier.Notify(chatJID, title, body, "dialog-information")
		return nil
	})
}

// setupUIHandlers wires all UI callbacks to the ChatController and InputManager.
func (br *Bridge) setupUIHandlers() {
	br.App.Sidebar.OnChatSelected = br.Chat.HandleChatSelected
	br.App.Sidebar.OnSearch = br.Chat.HandleSearch

	br.App.Sidebar.OnProfileSelected = func(profileID int64) {
		br.Chat.SetActiveProfileID(profileID)
	}
	br.App.Sidebar.OnManageProfiles = func() {
		ui.ShowProfileManagerDialog(&br.App.Window.Window, br.DB, func() {
			br.RefreshProfilesUI()
		}, func(profileID int64) {
			br.Chat.SetActiveProfileID(profileID)
			br.RefreshProfilesUI()
		})
	}

	br.RefreshProfilesUI()

	if br.App.InfoView != nil {
		br.App.InfoView.OnArchiveToggled = func(archived bool) {
			selJID := br.Chat.SelectedJID()
			if selJID == nil {
				return
			}
			jid := *selJID
			go func() {
				if br.Backend != nil {
					_ = br.Backend.ArchiveChat(context.Background(), jid, archived)
				}
				_ = br.DB.SetContactArchived(jid.ToNonAD().String(), archived)
				br.Chat.RefreshSidebarUI()
				glib.IdleAdd(func() {
					if br.App.InfoView != nil {
						br.App.InfoView.SetArchived(archived)
					}
				})
			}()
		}
	}

	br.WireChatView(br.App.ChatView)
	br.App.Window.Connect("notify::is-active", br.Chat.HandleWindowActive)

	br.App.OnKeyPressed = br.handleKeyPressed
	commitCycle := func() bool {
		if br.App.Sidebar != nil && br.App.Sidebar.IsCycling() {
			glib.IdleAdd(func() {
				br.App.Sidebar.CommitCycle()
			})
			return true
		}
		return false
	}
	cancelCycle := func() bool {
		if br.App.Sidebar != nil && br.App.Sidebar.IsCycling() {
			glib.IdleAdd(func() {
				br.App.Sidebar.CancelCycle()
			})
			return true
		}
		return false
	}

	br.App.OnNextChatRequested = func() {
		glib.IdleAdd(func() {
			br.App.Sidebar.CycleOffset(1)
		})
	}
	br.App.OnPreviousChatRequested = func() {
		glib.IdleAdd(func() {
			br.App.Sidebar.CycleOffset(-1)
		})
	}
	br.App.OnCtrlReleased = func() {
		commitCycle()
	}
	br.App.OnCommitCycle = commitCycle
	br.App.OnCancelCycle = cancelCycle
	br.App.OnSearchRequested = func() {
		var searchDialog *ui.SearchDialog
		onSearch := func(query string) {
			if query == "" {
				glib.IdleAdd(func() { searchDialog.Populate(nil, nil) })
				return
			}

			glib.IdleAdd(func() { searchDialog.SetLoading(true) })

			go func() {
				activeProfileID := br.Chat.GetActiveProfileID()
				msgs, err := br.DB.SearchMessages(activeProfileID, query, 50)
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
						br.Chat.RefreshMessagesAround(targetJID, msg.ID)
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

	// Ctrl+1 to Ctrl+9 to open chats by index
	for i := 1; i <= 9; i++ {
		idx := i - 1
		action := func() {
			glib.IdleAdd(func() {
				br.App.Sidebar.SelectIndex(idx)
			})
		}
		br.Input.Register(fmt.Sprintf("Control+%d", i), action)
		br.Input.Register(fmt.Sprintf("Control+KP_%d", i), action)
	}

	// Alt+1 to Alt+9 to switch active profile by index
	for i := 1; i <= 9; i++ {
		idx := i - 1
		action := func() {
			glib.IdleAdd(func() {
				if br.App.Sidebar != nil {
					br.App.Sidebar.SelectProfileIndex(idx)
				}
			})
		}
		br.Input.Register(fmt.Sprintf("Alt+%d", i), action)
		br.Input.Register(fmt.Sprintf("Alt+KP_%d", i), action)
	}

	closeChat := func() {
		glib.IdleAdd(func() {
			if br.Chat.SelectedJID() != nil {
				br.Chat.StopTyping(*br.Chat.SelectedJID())
			}
			br.Chat.selectedJID = nil
			br.App.ActiveMainJID = ""
			if br.App.Sidebar != nil {
				br.App.Sidebar.ClearSelection()
			}
			if br.App.ChatView != nil {
				br.App.ChatView.ClearInput()
				br.App.ChatView.SetNoConversation()
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

	newConversation := func() {
		glib.IdleAdd(func() {
			ui.ShowNewConversationDialog(&br.App.Window.Window, func(phone, message string, done func(err error)) {
				go func() {
					err := br.Chat.StartNewConversation(phone, message)
					done(err)
				}()
			})
		})
	}
	br.App.OnNewChatRequested = newConversation
	br.App.Sidebar.OnNewChat = newConversation
	br.Input.Register("Control+n", newConversation)
	br.Input.Register("Control+N", newConversation)

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
	nextChat := func() {
		glib.IdleAdd(func() {
			br.App.Sidebar.CycleOffset(1)
		})
	}
	prevChat := func() {
		glib.IdleAdd(func() {
			br.App.Sidebar.CycleOffset(-1)
		})
	}
	br.Input.Register("Control+Tab", nextChat)
	br.Input.Register("Control+ISO_Left_Tab", nextChat)
	br.Input.Register("Control+Shift+Tab", prevChat)
	br.Input.Register("Control+Shift+ISO_Left_Tab", prevChat)

	// Ctrl+Up and Ctrl+Down for keyboard message selection and reply
	br.Input.Register("Control+Up", func() {
		glib.IdleAdd(func() {
			if br.App.ChatView != nil {
				br.App.ChatView.MessageList.SelectMessageUp()
			}
		})
	})
	br.Input.Register("Control+Down", func() {
		glib.IdleAdd(func() {
			if br.App.ChatView != nil {
				br.App.ChatView.MessageList.SelectMessageDown()
			}
		})
	})
	br.Input.Register("Control+Left", func() {
		glib.IdleAdd(func() {
			if br.App.ChatView != nil {
				br.App.ChatView.MessageList.SelectMessageBlockUp()
			}
		})
	})
	br.Input.Register("Control+Right", func() {
		glib.IdleAdd(func() {
			if br.App.ChatView != nil {
				br.App.ChatView.MessageList.SelectMessageBlockDown()
			}
		})
	})
	br.Input.Register("Control+Shift+Up", func() {
		glib.IdleAdd(func() {
			if br.App.ChatView != nil && br.App.ChatView.MessageList.HasKeyboardSelection() {
				br.App.ChatView.MessageList.SelectReferencedMessage()
			}
		})
	})
	br.Input.Register("Control+Shift+Down", func() {
		glib.IdleAdd(func() {
			if br.App.ChatView != nil && br.App.ChatView.MessageList.HasKeyboardSelection() {
				br.App.ChatView.MessageList.SelectReferencedMessageReturn()
			}
		})
	})
	br.Input.Register("Return", func() {
		glib.IdleAdd(func() {
			if br.App.ChatView != nil && br.App.ChatView.MessageList.HasKeyboardSelection() {
				br.App.ChatView.MessageList.ConfirmKeyboardReply()
				br.App.ChatView.FocusEntry()
			}
		})
	})
	br.Input.Register("Escape", func() {
		glib.IdleAdd(func() {
			if br.App.Sidebar != nil && br.App.Sidebar.CancelCycle() {
				return
			}
			if br.App.ChatView != nil {
				if br.App.ChatView.InputBar.ReplyToID != "" {
					br.App.ChatView.InputBar.CancelReply()
					br.App.ChatView.FocusEntry()
					return
				}
				if br.App.ChatView.MessageList.HasKeyboardSelection() {
					br.App.ChatView.MessageList.ClearKeyboardSelection()
					br.App.ChatView.FocusEntry()
					return
				}
			}
			if br.App.Sidebar != nil {
				br.App.Sidebar.SearchEntry.SetText("")
			}
			if br.App.ChatView != nil {
				br.App.ChatView.FocusEntry()
			}
		})
	})
}

// setupServiceHandlers wires ContactService and MediaService callbacks to the UI.
func (br *Bridge) setupServiceHandlers() {
	br.Contacts.SetOnAvatarSet(func(jid string, tex *gdk.Texture) {
		glib.IdleAdd(func() {
			if br.App.ChatView != nil {
				br.App.ChatView.SetAvatar(jid, tex)
				if br.App.ActiveMainJID == jid || br.App.IsSameJID(br.App.ActiveMainJID, jid) {
					if br.App.ChatView.TopBar != nil && br.App.ChatView.TopBar.Avatar != nil {
						br.App.ChatView.TopBar.Avatar.SetCustomImage(tex)
					}
					if br.App.InfoView != nil && br.App.InfoView.Avatar != nil {
						br.App.InfoView.Avatar.SetCustomImage(tex)
					}
				}
			}
			if br.App.Sidebar != nil {
				br.App.Sidebar.SetAvatar(jid, tex)
			}
		})
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
	if key == "ISO_Left_Tab" {
		key = "Tab"
	}
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
	cv.OnNextChat = func() {
		glib.IdleAdd(func() {
			br.App.Sidebar.CycleOffset(1)
		})
	}
	cv.OnPreviousChat = func() {
		glib.IdleAdd(func() {
			br.App.Sidebar.CycleOffset(-1)
		})
	}
	cv.OnCtrlReleased = func() {
		if br.App.Sidebar != nil && br.App.Sidebar.IsCycling() {
			glib.IdleAdd(func() {
				br.App.Sidebar.CommitCycle()
			})
		}
	}
	cv.OnCommitCycle = func() bool {
		if br.App.Sidebar != nil && br.App.Sidebar.IsCycling() {
			glib.IdleAdd(func() {
				br.App.Sidebar.CommitCycle()
			})
			return true
		}
		return false
	}
	cv.OnCancelCycle = func() bool {
		if br.App.Sidebar != nil && br.App.Sidebar.IsCycling() {
			glib.IdleAdd(func() {
				br.App.Sidebar.CancelCycle()
			})
			return true
		}
		return false
	}
	cv.OnTyping = func() {
		if jid := br.Chat.SelectedJID(); jid != nil {
			br.Chat.StartTyping(*jid)
		}
	}
	cv.OnStopTyping = func() {
		if jid := br.Chat.SelectedJID(); jid != nil {
			br.Chat.StopTyping(*jid)
		}
	}
	cv.OnReconnect = func() {
		go func() {
			glib.IdleAdd(func() {
				cv.SetConnectionStatus("Connecting to WhatsApp...", false)
			})
			_ = br.Backend.Connect()
		}()
	}
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
	cv.OnSendSticker = func(item database.StickerItem) {
		if jid := br.Chat.SelectedJID(); jid != nil {
			br.Chat.HandleSendSticker(*jid, item)
		}
	}
	cv.OnSendStickerFile = func(path string) {
		if jid := br.Chat.SelectedJID(); jid != nil {
			br.Chat.HandleSendStickerFile(*jid, path)
		}
	}
	cv.OnToggleFavoriteSticker = func(item database.StickerItem, isFav bool) {
		br.Chat.HandleToggleFavoriteSticker(item, isFav)
	}
	cv.OnDeleteStickerHistory = func(id string) {
		_ = br.DB.DeleteStickerHistory(id)
	}
	cv.OnSyncFavorites = func() {
		if br.Backend != nil {
			go func() {
				_ = br.Backend.FetchFavoriteStickers(br.ctx)
			}()
		}
	}
	cv.IsStickerFavorite = func(id, path string) bool {
		fav, _ := br.DB.IsStickerFavorite(id)
		if !fav && path != "" {
			fav, _ = br.DB.IsStickerFavorite(path)
		}
		return fav
	}
	cv.LoadFavorites = func() []database.StickerItem {
		items, _ := br.DB.GetFavoriteStickers(100)
		return items
	}
	cv.LoadHistory = func() []database.StickerItem {
		items, _ := br.DB.GetStickerHistory(60)
		return items
	}
	cv.OnSendReaction = func(id, emoji string) {
		if jid := br.Chat.SelectedJID(); jid != nil {
			br.Chat.HandleSendReaction(*jid, id, emoji)
		}
	}
	cv.OnLoadOlder = func() {
		if jid := br.Chat.SelectedJID(); jid != nil && !cv.IsSearching {
			br.Chat.LoadOlderMessages(jid.ToNonAD().String(), "")
		}
	}
	cv.OnLoadMessageRequest = func(id string) {
		if jid := br.Chat.SelectedJID(); jid != nil && !cv.IsSearching {
			br.Chat.LoadOlderMessages(jid.ToNonAD().String(), id)
		}
	}
	cv.OnSearchMessages = func(query string) {
		if jid := br.Chat.SelectedJID(); jid != nil {
			br.Chat.RenderMessageSearch(jid.ToNonAD().String(), query)
		}
	}
	cv.OnCancelSearch = func() {
		if jid := br.Chat.SelectedJID(); jid != nil {
			br.Chat.CancelMessageSearch(jid.ToNonAD().String())
		}
	}
	cv.OnSearchResultClick = func(id string) {
		if jid := br.Chat.SelectedJID(); jid != nil {
			glib.IdleAdd(func() {
					cv.SearchBar.Close()
			})
			br.Chat.CancelMessageSearchAndJump(jid.ToNonAD().String(), id)
		}
	}
	cv.OnMentionClick = func(jid string) {
		glib.IdleAdd(func() {
			if br.App.Sidebar != nil {
				br.App.Sidebar.SelectChat(jid)
				br.Chat.HandleChatSelected(jid)
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

	cv.OnForwardMessages = func(msgIDs []string) {
		contacts := br.Chat.GetForwardContactItems()
		glib.IdleAdd(func() {
			chat.ShowForwardDialog(&br.App.Window.Window, contacts, func(targetJIDs []string) {
				br.Chat.HandleForwardMessages(targetJIDs, msgIDs)
			})
		})
	}
}
