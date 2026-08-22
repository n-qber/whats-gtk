package bridge

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"whats-gtk/internal/backend"
	"whats-gtk/internal/database"
	"whats-gtk/internal/events"
	"whats-gtk/internal/ui"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"go.mau.fi/whatsmeow"
	meowEvents "go.mau.fi/whatsmeow/types/events"
	"go.mau.fi/whatsmeow/types"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
	"whats-gtk/internal/ui/chat"
)

// ChatController handles user-initiated actions: chat selection, message sending,
// reactions, pins, and window events. It delegates search, media, and group sync to helper files.
type ChatController struct {
	Backend  *backend.Backend
	App      *ui.App
	DB       *database.AppDB
	Messages *MessageService
	Contacts *ContactService
	Media    *MediaService
	Mapper   *ViewMapper
	EventBus *events.EventBus
	ctx      context.Context

	OldestMessageTimes map[string]time.Time

	selectedJID     *types.JID
	lastSender      string
	lastDateStr     string
	activeProfileID int64
	sidebarMutex    sync.Mutex
	searchSerial    int
	lastGroupSync   map[string]time.Time
	cachedGroupInfo map[string]*types.GroupInfo
	groupMutex      sync.RWMutex
}

// NewChatController creates a new ChatController.
func NewChatController(b *backend.Backend, app *ui.App, db *database.AppDB, msgs *MessageService, contacts *ContactService, media *MediaService, ctx context.Context, bus *events.EventBus, mapper *ViewMapper) *ChatController {
	activeProfileID, _ := db.GetActiveProfileID()
	cc := &ChatController{
		Backend:            b,
		App:                app,
		DB:                 db,
		Messages:           msgs,
		Contacts:           contacts,
		Media:              media,
		Mapper:             mapper,
		EventBus:           bus,
		ctx:                ctx,
		activeProfileID:    activeProfileID,
		lastGroupSync:      make(map[string]time.Time),
		cachedGroupInfo:    make(map[string]*types.GroupInfo),
		OldestMessageTimes: make(map[string]time.Time),
	}
	cc.setupSubscriptions()
	return cc
}

func (cc *ChatController) setupSubscriptions() {
	events.SubscribeTyped(cc.EventBus, events.EventMessageReceived, func(payload events.LiveMessagePayload) {
		if msg, ok := payload.Msg.(*meowEvents.Message); ok {
			cc.HandleLiveMessage(msg, payload.IsSyncing)
		}
	})
}

func (cc *ChatController) HandleLiveMessage(msg *meowEvents.Message, isSyncing bool) {
	resolvedChat := cc.Messages.ResolveJID(msg.Info.Chat)
	selectedJID := cc.selectedJID
	if !isSyncing || (selectedJID != nil && resolvedChat.ToNonAD().String() == selectedJID.ToNonAD().String()) {
		jidStr := resolvedChat.ToNonAD().String()

		if selectedJID != nil && jidStr == selectedJID.ToNonAD().String() {
			if cc.App.Window.IsActive() {
				go cc.Backend.MarkRead(cc.ctx, msg.Info.Chat, []string{msg.Info.ID}, msg.Info.Sender, time.Now())
			}
		}

		cc.EventBus.Publish(events.Event{
			Type: events.EventChatUpdated,
			Data: jidStr,
		})

		dbMsg, err := cc.DB.GetMessage(msg.Info.ID)
		if err == nil {
			tStr := msg.Info.Timestamp.Format("15:04")
			sName := ""
			var av *gdk.Texture
			isCont := false

			resolvedSender := cc.Messages.ResolveJID(msg.Info.Sender)
			sJID := resolvedSender.ToNonAD().String()
			isCont = sJID == cc.lastSender

			if msg.Info.Chat.Server == types.GroupServer && !msg.Info.IsFromMe {
				if !isCont {
					sName = cc.Contacts.ResolveSenderName(sJID)
					av = cc.Contacts.GetAvatar(sJID)
				}
			}
			cc.lastSender = sJID

			uiMsg := cc.Mapper.MapMessage(*dbMsg, sName, tStr, av, isCont)
			dateStr := cc.Mapper.FormatMessageDate(msg.Info.Timestamp)
			if dateStr != cc.lastDateStr {
				uiMsg.DateSeparator = dateStr
				cc.lastDateStr = dateStr
			}

			cc.EventBus.Publish(events.Event{
				Type: events.EventLiveMessageReceived,
				Data: events.LiveUIMessagePayload{
					JID:       jidStr,
					Message:   uiMsg,
					IsSyncing: isSyncing,
				},
			})
		}
	}
}

func (cc *ChatController) SelectedJID() *types.JID { return cc.selectedJID }
func (cc *ChatController) LastSender() string     { return cc.lastSender }

func (cc *ChatController) HandleWindowActive() {
	if cc.App.Window.IsActive() {
		jid := cc.SelectedJID()
		if jid != nil && cc.Backend != nil && cc.Backend.Client != nil {
			go cc.Backend.MarkRead(cc.ctx, *jid, []string{}, types.JID{}, time.Now())
		}
	}
}

func (cc *ChatController) SetLastSender(s string)  { cc.lastSender = s }
func (cc *ChatController) LastDateStr() string     { return cc.lastDateStr }
func (cc *ChatController) SetLastDateStr(s string) { cc.lastDateStr = s }

func (cc *ChatController) HandleChatSelected(jidStr string) {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return
	}
	jid = cc.Messages.ResolveJID(jid)

	if cc.selectedJID != nil && cc.selectedJID.ToNonAD().String() == jid.ToNonAD().String() {
		glib.IdleAdd(func() {
			if cc.App.ChatView != nil {
				cc.App.ChatView.FocusEntry()
			}
		})
		return
	}

	cc.selectedJID = &jid
	cc.lastSender = ""
	cc.App.ActiveMainJID = jid.ToNonAD().String()

	if contact, err := cc.DB.GetContact(jid.String()); err == nil {
		headerName := contact.DisplayName()
		if jid.Server == types.GroupServer {
			headerName = "[G] " + headerName
		}
		cc.App.ChatView.SetHeader(headerName, cc.Contacts.GetAvatar(jid.String()))
		cc.App.InfoView.SetInfo(headerName, jid.String(), cc.Contacts.GetAvatar(jid.String()))
	} else {
		cc.App.ChatView.SetHeader(jid.String(), cc.Contacts.GetAvatar(jid.String()))
		cc.App.InfoView.SetInfo(jid.String(), jid.String(), cc.Contacts.GetAvatar(jid.String()))
	}
	cc.App.InfoFlap.SetRevealFlap(false)

	cc.DB.ClearUnreadCount(jid.String())
	if contact, err := cc.DB.GetContact(jid.String()); err == nil {
		cc.App.Sidebar.UpdateChatRow(jid.String(), contact.DisplayName(), contact.IsGroup.Valid && contact.IsGroup.Bool, 0, contact.IsPinned)
	}

	if cc.App.ChatView != nil {
		cc.App.ChatView.Clear()
	}

	cc.RefreshMessages(jid)

	if cc.Backend != nil && cc.Backend.Client != nil {
		if cc.App.Window.IsActive() {
			go cc.Backend.MarkRead(cc.ctx, jid, []string{}, types.JID{}, time.Now())
		}
	}

	if jid.Server == types.GroupServer {
		cc.SyncGroupIfNeeded(jid)
	}

	glib.IdleAdd(func() {
		if cc.App.ChatView != nil {
			cc.App.ChatView.FocusEntry()
		}
	})
}

func (cc *ChatController) HandleSendMessage(targetJID types.JID, text string, replyToID string) {
	now := time.Now().Format("15:04")
	msgID := cc.Backend.GenerateMessageID()

	var qID, qSender, qContent string
	if replyToID != "" {
		if qm, err := cc.DB.GetMessage(replyToID); err == nil {
			qID = qm.ID
			qSender = qm.SenderJID
			qContent = qm.Content
			if qm.Type != "text" {
				qContent = "[" + strings.Title(qm.Type) + "]"
			}
		}
	}

	glib.IdleAdd(func() {
		if cv := cc.App.GetChatViewForJID(targetJID.ToNonAD().String()); cv != nil {
			isCont := cc.lastSender == cc.Backend.Device.ID.ToNonAD().String()
			cv.AddMessage(msgID, "", "", text, true, isCont, "pending", now, nil, qID, qSender, qContent)
			cc.lastSender = cc.Backend.Device.ID.ToNonAD().String()
			cv.ScrollToBottom()
		}
	})

	cc.DB.SaveMessage(database.Message{
		ID: msgID, ChatJID: targetJID.ToNonAD().String(), SenderJID: cc.Backend.Device.ID.ToNonAD().String(),
		Content: text, Type: "text", Timestamp: time.Now(), Status: "pending", IsFromMe: true,
		QuotedMsgID:      sql.NullString{String: qID, Valid: qID != ""},
		QuotedMsgSender:  sql.NullString{String: qSender, Valid: qSender != ""},
		QuotedMsgContent: sql.NullString{String: qContent, Valid: qContent != ""},
	})

	go func() {
		var contextInfo *waProto.ContextInfo
		if replyToID != "" {
			if qm, err := cc.DB.GetMessage(replyToID); err == nil {
				contextInfo = &waProto.ContextInfo{
					StanzaID:    proto.String(qm.ID),
					Participant: proto.String(qm.SenderJID),
					QuotedMessage: &waProto.Message{
						Conversation: proto.String(qm.Content),
					},
				}
			}
		}

		resp, err := cc.Backend.SendText(cc.ctx, targetJID, text, contextInfo, whatsmeow.SendRequestExtra{ID: types.MessageID(msgID)})
		if err != nil {
			fmt.Printf("Bridge: SendText failed for %s: %v\n", msgID, err)
			cc.DB.UpdateMessageStatus(msgID, targetJID.ToNonAD().String(), "failed")
			glib.IdleAdd(func() {
				if cv := cc.App.GetChatViewForJID(targetJID.ToNonAD().String()); cv != nil {
					cv.UpdateMessageStatus(msgID, "failed")
				}
			})
			return
		}

		cc.DB.UpdateMessageStatus(msgID, targetJID.ToNonAD().String(), "sent")
		cc.DB.UpdateContactTimestamp(targetJID.ToNonAD().String(), resp.Timestamp)
		glib.IdleAdd(func() {
			if cv := cc.App.GetChatViewForJID(targetJID.ToNonAD().String()); cv != nil {
				cv.UpdateMessageStatus(msgID, "sent")
			}
		})
	}()
}

func (cc *ChatController) HandleSendReaction(targetJID types.JID, id, emoji string) {
	msg, err := cc.DB.GetMessage(id)
	if err != nil {
		return
	}

	go func() {
		_, err := cc.Backend.SendReaction(cc.ctx, targetJID, id, msg.IsFromMe, emoji)
		if err == nil {
			cc.Messages.HandleReaction(targetJID, cc.Backend.Device.ID.ToNonAD(), emoji, id, time.Now())
		} else {
			fmt.Printf("Bridge: SendReaction failed: %v\n", err)
		}
	}()
}

func (cc *ChatController) HandlePinMessage(targetJID types.JID, id string, pin bool, duration uint32) {
	msg, err := cc.DB.GetMessage(id)
	if err != nil {
		return
	}

	go func() {
		_, err := cc.Backend.PinMessage(cc.ctx, targetJID, id, msg.IsFromMe, pin, duration)
		if err == nil {
			cc.DB.UpdateMessagePinned(id, targetJID.ToNonAD().String(), pin)
			glib.IdleAdd(func() {
				if cv := cc.App.GetChatViewForJID(targetJID.ToNonAD().String()); cv != nil {
					cv.UpdateMessagePinned(id, pin)
				}
			})
		} else {
			fmt.Printf("Bridge: PinMessage failed: %v\n", err)
		}
	}()
}

func (c *ChatController) HandleDetach() {
	if c.selectedJID == nil {
		return
	}
	jidStr := c.selectedJID.ToNonAD().String()

	if _, ok := c.App.DetachedChats[jidStr]; ok {
		return
	}

	glib.IdleAdd(func() {
		win := adw.NewWindow()
		win.SetTitle("WhatsApp - Detached Chat")
		win.SetDefaultSize(600, 700)

		cv, err := chat.NewChatView()
		if err != nil {
			fmt.Printf("Failed to create detached chat view: %v\n", err)
			return
		}

		c.App.DetachedChats[jidStr] = cv
		targetJID, _ := types.ParseJID(jidStr)

		cv.OnSendMessage = func(text, replyToID string) { c.HandleSendMessage(targetJID, text, replyToID) }
		cv.OnPasteImage = func(tex *gdk.Texture) { c.HandlePasteImage(targetJID, tex) }
		cv.OnSendFile = func(path string) { c.HandleSendFile(targetJID, path) }
		cv.OnSendReaction = func(id, emoji string) { c.HandleSendReaction(targetJID, id, emoji) }
		cv.OnLoadOlder = func() {
			if !cv.IsSearching {
				c.LoadOlderMessages(targetJID.ToNonAD().String(), "")
			}
		}
		cv.OnLoadMessageRequest = func(id string) {
			if !cv.IsSearching {
				c.LoadOlderMessages(targetJID.ToNonAD().String(), id)
			}
		}
		cv.OnSearchMessages = func(query string) { c.HandleSearch(query) }
		cv.OnCancelSearch = func() { c.CancelMessageSearch(targetJID.ToNonAD().String()) }
		cv.OnCancelSearchAndJump = func(id string) { c.CancelMessageSearchAndJump(targetJID.ToNonAD().String(), id) }
		cv.OnSearchResultClick = func(id string) {
			glib.IdleAdd(func() { cv.SearchBar.Close() })
			c.CancelMessageSearchAndJump(targetJID.ToNonAD().String(), id)
		}
		cv.OnMentionClick = func(mjid string) {
			glib.IdleAdd(func() {
				if c.App.Sidebar != nil {
					c.App.Sidebar.SelectChat(mjid)
					c.HandleChatSelected(mjid)
				}
			})
		}
		cv.OnSendPollVote = func(msgID string, senderJID string, isFromMe bool, selectedOptions []string) {
			sender, _ := types.ParseJID(senderJID)
			c.Backend.SendPollVote(context.Background(), targetJID, msgID, sender, isFromMe, selectedOptions)
		}
		cv.OnPinMessage = func(id string, pin bool, duration uint32) { c.HandlePinMessage(targetJID, id, pin, duration) }
		cv.OnDownloadMedia = c.HandleDownloadMedia
		cv.OnOpenImage = c.HandleOpenImage
		cv.OnDetach = c.HandleDetach

		win.Connect("notify::is-active", func() { c.HandleWindowActive() })

		win.SetContent(cv.Box)
		win.Show()
		glib.IdleAdd(func() {
			cv.FocusEntry()
		})

		win.Connect("close-request", func() bool {
			delete(c.App.DetachedChats, jidStr)
			return false
		})

		c.RefreshMessages(targetJID)

		c.App.ChatView.SetNoConversation()
		c.App.ActiveMainJID = ""
		c.selectedJID = nil
	})
}

func (cc *ChatController) RefreshMessages(jid types.JID) {
	go func() {
		jids := []string{jid.ToNonAD().String()}
		if contact, err := cc.DB.GetContact(jid.ToNonAD().String()); err == nil {
			if contact.LID.Valid && contact.LID.String != "" {
				jids = append(jids, contact.LID.String)
			}
		}

		msgs, err := cc.DB.GetMessages(jids, 50)
		if err != nil {
			fmt.Printf("Controller: GetMessages failed: %v\n", err)
			return
		}

		if len(msgs) > 0 {
			cc.OldestMessageTimes[jid.ToNonAD().String()] = msgs[0].Timestamp
		}

		var uiMsgs []events.UIMessage
		var lastSender string
		var localLastDateStr string

		for _, m := range msgs {
			sName := ""
			var av *gdk.Texture
			isCont := m.SenderJID == lastSender

			if jid.Server == types.GroupServer && !m.IsFromMe {
				if !isCont {
					sName = cc.Contacts.ResolveSenderName(m.SenderJID)
					av = cc.Contacts.GetAvatar(m.SenderJID)
				}
			}
			lastSender = m.SenderJID

			dateStr := cc.Mapper.FormatMessageDate(m.Timestamp)
			tStr := m.Timestamp.Format("15:04")

			uiMsg := cc.Mapper.MapMessage(m, sName, tStr, av, isCont)
			if dateStr != localLastDateStr {
				uiMsg.DateSeparator = dateStr
				localLastDateStr = dateStr
			} else {
				uiMsg.DateSeparator = ""
			}
			uiMsgs = append(uiMsgs, uiMsg)
		}

		if len(uiMsgs) > 0 {
			cc.lastDateStr = localLastDateStr
		}

		cc.EventBus.Publish(events.Event{
			Type: events.EventChatMessagesLoaded,
			Data: events.ChatMessagesPayload{
				JID:      jid.ToNonAD().String(),
				Messages: uiMsgs,
				Append:   false,
			},
		})
	}()
}

func (cc *ChatController) GetForwardContactItems() []chat.ForwardContactItem {
	dbContacts, err := cc.DB.GetAllContacts(cc.activeProfileID, 500)
	if err != nil {
		return nil
	}
	var items []chat.ForwardContactItem
	for _, c := range dbContacts {
		if c.IsArchived {
			continue
		}
		isGroup := c.IsGroup.Valid && c.IsGroup.Bool
		parsedJID, _ := types.ParseJID(c.JID)
		avatar := cc.Contacts.GetAvatar(parsedJID.String())

		items = append(items, chat.ForwardContactItem{
			JID:     c.JID,
			Name:    c.DisplayName(),
			IsGroup: isGroup,
			Avatar:  avatar,
		})
	}
	return items
}

func (cc *ChatController) HandleForwardMessages(targetJIDStrs []string, msgIDs []string) {
	go func() {
		for _, targetStr := range targetJIDStrs {
			targetJID, err := types.ParseJID(targetStr)
			if err != nil {
				continue
			}
			targetJID = cc.Messages.ResolveJID(targetJID)
			chatJID := targetJID.ToNonAD().String()

			for _, msgID := range msgIDs {
				dbMsg, err := cc.DB.GetMessage(msgID)
				if err != nil {
					continue
				}

				resp, err := cc.Backend.ForwardMessage(cc.ctx, targetJID, *dbMsg)
				if err != nil {
					fmt.Printf("Bridge: Failed to forward message %s to %s: %v\n", msgID, targetStr, err)
					continue
				}

				newMsg := database.Message{
					ID:              resp.ID,
					ChatJID:         chatJID,
					SenderJID:       cc.Backend.Device.ID.ToNonAD().String(),
					Content:         dbMsg.Content,
					Caption:         dbMsg.Caption,
					Type:            dbMsg.Type,
					Timestamp:       resp.Timestamp,
					Status:          "sent",
					IsFromMe:        true,
					Thumbnail:       dbMsg.Thumbnail,
					MediaURL:        dbMsg.MediaURL,
					MediaDirectPath: dbMsg.MediaDirectPath,
					MediaKey:        dbMsg.MediaKey,
					MediaMimetype:   dbMsg.MediaMimetype,
					MediaEncSHA256:  dbMsg.MediaEncSHA256,
					MediaSHA256:     dbMsg.MediaSHA256,
					MediaLength:     dbMsg.MediaLength,
					MediaWidth:      dbMsg.MediaWidth,
					MediaHeight:     dbMsg.MediaHeight,
					IsForwarded:     true,
				}

				cc.DB.SaveMessage(newMsg)
				cc.DB.UpdateContactTimestamp(chatJID, resp.Timestamp)

				if cc.selectedJID != nil && cc.selectedJID.ToNonAD().String() == chatJID {
					tStr := resp.Timestamp.Format("15:04")
					isCont := cc.lastSender == cc.Backend.Device.ID.ToNonAD().String()
					uiMsg := cc.Mapper.MapMessage(newMsg, "", tStr, nil, isCont)
					cc.lastSender = cc.Backend.Device.ID.ToNonAD().String()

					glib.IdleAdd(func() {
						if cv := cc.App.GetChatViewForJID(chatJID); cv != nil {
							if uiMsg.Type == "text" {
								cv.AddMessage(uiMsg.ID, uiMsg.JID, uiMsg.SenderName, uiMsg.Content, uiMsg.IsFromMe, uiMsg.IsContinuation, uiMsg.Status, uiMsg.TimeString, uiMsg.Avatar, "", "", "")
							} else if uiMsg.Type == "image" {
								var texImg, texThumb *gdk.Texture
								texThumb = bytesToTexture(uiMsg.Thumbnail)
								if uiMsg.DocumentPath != "" {
									if pixbuf, _ := gdkpixbuf.NewPixbufFromFile(uiMsg.DocumentPath); pixbuf != nil {
										texImg = gdk.NewTextureForPixbuf(pixbuf)
									}
								}
								cv.AddImage(uiMsg.ID, uiMsg.JID, uiMsg.SenderName, uiMsg.Content, texImg, texThumb, uiMsg.DocumentPath, uiMsg.IsFromMe, uiMsg.IsContinuation, uiMsg.Status, uiMsg.TimeString, uiMsg.Avatar, "", "", "", uiMsg.MediaWidth, uiMsg.MediaHeight)
							} else {
								cv.AddMessage(uiMsg.ID, uiMsg.JID, uiMsg.SenderName, uiMsg.Content, uiMsg.IsFromMe, uiMsg.IsContinuation, uiMsg.Status, uiMsg.TimeString, uiMsg.Avatar, "", "", "")
							}
							cv.UpdateMessageForwarded(uiMsg.ID, true)
							cv.ScrollToBottom()
						}
					})
				}

				cc.EventBus.Publish(events.Event{
					Type: events.EventChatUpdated,
					Data: chatJID,
				})
			}
		}
	}()
}

