package bridge

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"whats-gtk/internal/backend"
	"whats-gtk/internal/core"
	"whats-gtk/internal/database"
	"whats-gtk/internal/ui"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

type Bridge struct {
	Backend       *backend.Backend
	App           *ui.App
	DB            *database.AppDB
	ctx           context.Context
	
	Contacts      *ContactService
	Media         *MediaService

	Input         *core.InputManager
	Pipeline      *core.MessagePipeline

	selectedJID   *types.JID
	lastSender    string
	sidebarMutex  sync.Mutex
	isSyncing     bool
	lastGroupSync map[string]time.Time
	searchSerial  int
}

func NewBridge(b *backend.Backend, a *ui.App, db *database.AppDB, ctx context.Context) *Bridge {
	br := &Bridge{
		Backend: b, App: a, DB: db, ctx: ctx,
		lastGroupSync: make(map[string]time.Time),
		Input:         core.NewInputManager(),
		Pipeline:      core.NewMessagePipeline(),
	}

	br.registerDefaultHooks()

	br.Contacts = NewContactService(b, db, ctx)
	br.Media = NewMediaService(b, db, ctx)

	br.setupUIHandlers()
	br.setupServiceHandlers()

	return br
}

func (br *Bridge) registerDefaultHooks() {
	// Hook to auto-download stickers
	br.Pipeline.AddHook(func(ctx context.Context, msg *events.Message) error {
		if stkr := msg.Message.GetStickerMessage(); stkr != nil {
			go br.handleDownloadMedia(msg.Info.ID)
		}
		return nil
	})
}

func (br *Bridge) setupUIHandlers() {
	br.App.Sidebar.OnChatSelected = br.handleChatSelected
	br.App.ChatView.OnSendMessage = br.handleSendMessage
	br.App.Sidebar.OnSearch = br.handleSearch
	br.App.ChatView.OnPasteImage = br.handlePasteImage
	br.App.ChatView.OnDownloadMedia = br.handleDownloadMedia
	br.App.ChatView.OnSendReaction = br.handleSendReaction

	br.App.OnKeyPressed = br.handleKeyPressed

	// Register some default shortcuts
	br.Input.Register("Escape", func() {
		glib.IdleAdd(func() {
			br.App.Sidebar.SearchEntry.SetText("")
			br.App.ChatView.FocusEntry()
		})
	})
}

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

func (br *Bridge) handleSendReaction(id, emoji string) {
	if br.selectedJID == nil { return }
	msg, err := br.DB.GetMessage(id); if err != nil { return }
	
	chatJID := *br.selectedJID

	go func() {
		_, err := br.Backend.SendReaction(br.ctx, chatJID, id, msg.IsFromMe, emoji)
		if err == nil {
			br.handleReactionInternal(chatJID, br.Backend.Device.ID.ToNonAD(), emoji, id, time.Now())
		} else {
			fmt.Printf("Bridge: SendReaction failed: %v\n", err)
		}
	}()
}

func (br *Bridge) handlePasteImage(tex *gdk.Texture) {
	if br.selectedJID == nil { return }
	targetJID := *br.selectedJID
	now := time.Now().Format("15:04")

	glib.IdleAdd(func() {
		if br.selectedJID != nil && br.selectedJID.ToNonAD().String() == targetJID.ToNonAD().String() {
			br.App.ChatView.AddImage("temp_img", "", "", tex, nil, true, false, "pending", now, nil, "", "", "", int(tex.Width()), int(tex.Height()))
			br.App.ChatView.ScrollToBottom()
		}
	})

	go func() {
		// Convert texture to bytes via pixbuf
		pixbuf := gdk.PixbufGetFromTexture(tex)
		if pixbuf == nil { return }
		
		tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("paste_%d.jpg", time.Now().UnixNano()))
		err := pixbuf.Savev(tmpPath, "jpeg", nil, nil)
		if err != nil {
			fmt.Printf("Bridge: Failed to save temp image: %v\n", err)
			return
		}
		defer os.Remove(tmpPath)

		data, err := os.ReadFile(tmpPath)
		if err != nil {
			fmt.Printf("Bridge: Failed to read temp image: %v\n", err)
			return
		}

		resp, err := br.Backend.SendImage(br.ctx, targetJID, data, "image/jpeg")
		if err != nil {
			fmt.Printf("Bridge: SendImage failed: %v\n", err)
			return
		}

		glib.IdleAdd(func() {
			if br.selectedJID != nil && br.selectedJID.ToNonAD().String() == targetJID.ToNonAD().String() {
				br.App.ChatView.UpdateMessageStatus("temp_img", "sent")
				if b, exists := br.App.ChatView.MessageRows["temp_img"]; exists {
					br.App.ChatView.MessageRows[resp.ID] = b; delete(br.App.ChatView.MessageRows, "temp_img")
				}
				if r, exists := br.App.ChatView.MessageListRows["temp_img"]; exists {
					br.App.ChatView.MessageListRows[resp.ID] = r; delete(br.App.ChatView.MessageListRows, "temp_img")
				}
			}
		})

		// Save to DB and media folder
		path := filepath.Join("media", resp.ID+".jpg")
		os.WriteFile(path, data, 0644)
		br.DB.SaveMessage(database.Message{
			ID: resp.ID, ChatJID: targetJID.ToNonAD().String(), SenderJID: br.Backend.Device.ID.ToNonAD().String(),
			Content: path, Type: "image", Timestamp: resp.Timestamp, Status: "sent", IsFromMe: true,
			MediaWidth: sql.NullInt64{Int64: int64(tex.Width()), Valid: true},
			MediaHeight: sql.NullInt64{Int64: int64(tex.Height()), Valid: true},
		})
		br.DB.UpdateContactTimestamp(targetJID.ToNonAD().String(), resp.Timestamp)
	}()
}

func (br *Bridge) setupServiceHandlers() {
	br.Contacts.SetOnAvatarSet(func(jid string, tex *gdk.Texture) {
		br.App.ChatView.SetAvatar(jid, tex)
		br.App.Sidebar.SetAvatar(jid, tex)
	})

	br.Media.SetOnMediaDownloaded(func(task DownloadTask, data []byte, path string) {
		glib.IdleAdd(func() {
			if br.selectedJID != nil && br.selectedJID.ToNonAD().String() == task.ChatJID {
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

				br.App.ChatView.UpdateMessageImage(task.ID, tex)
			}
		})
	})
}

func (br *Bridge) handleChatSelected(jidStr string) {
	jid, err := types.ParseJID(jidStr); if err != nil { return }
	jid = br.resolveJID(jid); br.selectedJID = &jid; br.lastSender = "" 
	
	if contact, err := br.DB.GetContact(jid.String()); err == nil {
		headerName := contact.DisplayName()
		if jid.Server == types.GroupServer { headerName = "[G] " + headerName }
		br.App.ChatView.SetHeader(headerName, br.Contacts.GetAvatar(jid.String()))
	} else {
		br.App.ChatView.SetHeader(jid.String(), br.Contacts.GetAvatar(jid.String()))
	}

	br.refreshMessages(jid)
	
	// Mark as read
	go br.Backend.MarkRead(br.ctx, jid, []string{}, types.JID{}, time.Now())

	if strings.HasSuffix(jid.String(), "@lid") && !br.isSyncing {
		go br.Contacts.ResolveLIDMapping(jid.String())
	}

	if jid.Server == types.GroupServer {
		br.syncGroupIfNeeded(jid)
	}

	br.App.ChatView.FocusEntry()
}

func (br *Bridge) syncGroupIfNeeded(jid types.JID) {
	lastSync, exists := br.lastGroupSync[jid.String()]
	if !exists || time.Since(lastSync) > 30*time.Minute {
		go func(groupJID types.JID) {
			info, err := br.Backend.GetGroupInfo(br.ctx, groupJID); if err != nil { return }
			br.lastGroupSync[groupJID.String()] = time.Now()
			for _, p := range info.Participants {
				pn := p.PhoneNumber.ToNonAD().String(); lid := p.LID.ToNonAD().String()
				if pn != "" && lid != "" {
					br.DB.MergeLID(pn, lid)
				} else {
					br.DB.SaveContact(database.Contact{JID: p.JID.ToNonAD().String()})
				}
			}
			if br.selectedJID != nil && br.selectedJID.ToNonAD().String() == groupJID.ToNonAD().String() {
				glib.IdleAdd(func() { br.refreshMessages(groupJID) })
			}
		}(jid)
	}
}

func (br *Bridge) handleSendMessage(text string, replyToID string) {
	if br.selectedJID == nil { return }
	targetJID := *br.selectedJID; now := time.Now().Format("15:04")
	
	// Get Quoted Context if any
	var qID, qSender, qContent string
	if replyToID != "" {
		if qm, err := br.DB.GetMessage(replyToID); err == nil {
			qID = qm.ID
			qSender = qm.SenderJID
			qContent = qm.Content
			if qm.Type != "text" { qContent = "[" + strings.Title(qm.Type) + "]" }
		}
	}

	glib.IdleAdd(func() {
		if br.selectedJID != nil && br.selectedJID.ToNonAD().String() == targetJID.ToNonAD().String() {
			isCont := br.lastSender == br.Backend.Device.ID.ToNonAD().String()
			// For own messages, name is empty but avatar should show if available
			br.App.ChatView.AddMessage("temp", "", "", text, true, isCont, "pending", now, nil, qID, qSender, qContent)
			br.lastSender = br.Backend.Device.ID.ToNonAD().String()
			br.App.ChatView.ScrollToBottom()
		}
	})

	go func() {
		var contextInfo *waProto.ContextInfo
		if replyToID != "" {
			if qm, err := br.DB.GetMessage(replyToID); err == nil {
				contextInfo = &waProto.ContextInfo{
					StanzaID:    proto.String(qm.ID),
					Participant: proto.String(qm.SenderJID),
					QuotedMessage: &waProto.Message{
						Conversation: proto.String(qm.Content),
					},
				}
			}
		}

		resp, err := br.Backend.SendText(br.ctx, targetJID, text, contextInfo); if err != nil { return }
		glib.IdleAdd(func() {
			if br.selectedJID != nil && br.selectedJID.ToNonAD().String() == targetJID.ToNonAD().String() {
				br.App.ChatView.UpdateMessageStatus("temp", "sent")
				if b, exists := br.App.ChatView.MessageRows["temp"]; exists {
					br.App.ChatView.MessageRows[resp.ID] = b; delete(br.App.ChatView.MessageRows, "temp")
				}
				if r, exists := br.App.ChatView.MessageListRows["temp"]; exists {
					br.App.ChatView.MessageListRows[resp.ID] = r; delete(br.App.ChatView.MessageListRows, "temp")
				}
			}
		})
		br.DB.SaveMessage(database.Message{
			ID: resp.ID, ChatJID: targetJID.ToNonAD().String(), SenderJID: br.Backend.Device.ID.ToNonAD().String(), 
			Content: text, Type: "text", Timestamp: resp.Timestamp, Status: "sent", IsFromMe: true,
			QuotedMsgID: sql.NullString{String: qID, Valid: qID != ""},
			QuotedMsgSender: sql.NullString{String: qSender, Valid: qSender != ""},
			QuotedMsgContent: sql.NullString{String: qContent, Valid: qContent != ""},
		})
		br.DB.UpdateContactTimestamp(targetJID.ToNonAD().String(), resp.Timestamp)
	}()
}

func (br *Bridge) handleSearch(t string) {
	br.sidebarMutex.Lock()
	br.searchSerial++
	serial := br.searchSerial
	br.sidebarMutex.Unlock()

	go func() {
		var c []database.Contact
		var err error
		if strings.TrimSpace(t) == "" {
			c, err = br.DB.GetAllContacts(100)
		} else {
			c, err = br.DB.SearchContacts(t, 200)
		}
		
		br.sidebarMutex.Lock()
		if serial != br.searchSerial {
			br.sidebarMutex.Unlock()
			return
		}
		br.sidebarMutex.Unlock()

		if err != nil {
			fmt.Printf("Bridge: Search failed: %v\n", err)
			return
		}
		br.refreshSidebar(c)
	}()
}

func (br *Bridge) refreshMessages(jid types.JID) {
	go func() {
		jids := []string{jid.ToNonAD().String()}
		if contact, err := br.DB.GetContact(jid.ToNonAD().String()); err == nil {
			if contact.LID.Valid && contact.LID.String != "" {
				jids = append(jids, contact.LID.String)
			}
		}

		msgs, err := br.DB.GetMessages(jids, 50)
		if err != nil {
			fmt.Printf("Bridge: GetMessages failed: %v\n", err)
			return
		}
		seen := make(map[string]bool)
		for i := len(msgs) - 1; i >= 0; i-- {
			if !msgs[i].IsFromMe && !seen[msgs[i].SenderJID] {
				br.Contacts.GetAvatar(msgs[i].SenderJID)
				seen[msgs[i].SenderJID] = true
			}
		}
		glib.IdleAdd(func() {
			if br.selectedJID == nil || br.selectedJID.ToNonAD().String() != jid.ToNonAD().String() { return }
			br.App.ChatView.Clear(); br.lastSender = "" 
			for _, m := range msgs {
				tStr := m.Timestamp.Format("15:04"); sName := ""; var av *gdk.Texture; isCont := m.SenderJID == br.lastSender
				if jid.Server == types.GroupServer && !m.IsFromMe {
					if !isCont {
						sName = br.Contacts.ResolveSenderName(m.SenderJID)
						av = br.Contacts.GetAvatar(m.SenderJID)
					}
				}
				br.lastSender = m.SenderJID
				qID := m.QuotedMsgID.String; qSender := m.QuotedMsgSender.String; qContent := m.QuotedMsgContent.String
				qSenderName := qSender
				if qSender != "" {
					qSenderName = br.Contacts.ResolveSenderName(qSender)
				}

				if m.Type == "image" || m.Type == "sticker" || m.Type == "video" {
					var texImg, texThumb *gdk.Texture
					texThumb = br.bytesToTexture(m.Thumbnail)
					
					if _, err := os.Stat(m.Content); err == nil {
						pixbuf, _ := gdkpixbuf.NewPixbufFromFile(m.Content)
						if pixbuf != nil {
							texImg = gdk.NewTextureForPixbuf(pixbuf)
						}
					} else if m.Type == "sticker" {
						// Auto-download missing stickers
						go br.handleDownloadMedia(m.ID)
					}
					
					mW := int(m.MediaWidth.Int64); mH := int(m.MediaHeight.Int64)

					if m.Type == "image" {
						br.App.ChatView.AddImage(m.ID, m.SenderJID, sName, texImg, texThumb, m.IsFromMe, isCont, m.Status, tStr, av, qID, qSenderName, qContent, mW, mH)
					} else if m.Type == "sticker" {
						br.App.ChatView.AddSticker(m.ID, m.SenderJID, sName, texImg, texThumb, m.IsFromMe, isCont, m.Status, tStr, av, qID, qSenderName, qContent, mW, mH)
					} else if m.Type == "video" {
						br.App.ChatView.AddVideo(m.ID, m.SenderJID, sName, texThumb, m.IsFromMe, isCont, m.Status, tStr, av, qID, qSenderName, qContent, mW, mH)
					}
				} else if m.Type == "audio" {
					br.App.ChatView.AddAudio(m.ID, m.SenderJID, sName, m.IsFromMe, isCont, m.Status, tStr, av, qID, qSenderName, qContent)
					if m.Content != "" {
						if _, err := os.Stat(m.Content); err == nil {
							br.App.ChatView.UpdateMessageAudio(m.ID, m.Content)
						}
					}
				} else if m.Type == "document" {
					fileName := "file"
					if m.Content != "" {
						if strings.HasPrefix(m.Content, "[Document: ") {
							fileName = strings.TrimSuffix(strings.TrimPrefix(m.Content, "[Document: "), "]")
						} else if strings.Contains(m.Content, "media/") {
							// Extract original name from saved path: media/ID_FileName.ext
							base := filepath.Base(m.Content)
							if idx := strings.Index(base, "_"); idx != -1 {
								fileName = base[idx+1:]
							}
						}
					}
					texThumb := br.bytesToTexture(m.Thumbnail)
					br.App.ChatView.AddDocument(m.ID, m.SenderJID, sName, fileName, texThumb, m.IsFromMe, isCont, m.Status, tStr, av, qID, qSenderName, qContent)
					if m.Content != "" && !strings.HasPrefix(m.Content, "[Document: ") {
						if _, err := os.Stat(m.Content); err == nil {
							br.App.ChatView.UpdateMessageDocument(m.ID, m.Content)
						}
					}
				} else {
					if m.Content != "" {
						br.App.ChatView.AddMessage(m.ID, m.SenderJID, sName, m.Content, m.IsFromMe, isCont, m.Status, tStr, av, qID, qSenderName, qContent)
					}
				}
				
				// Set reactions
				reacts, _ := br.DB.GetReactions(m.ID)
				if len(reacts) > 0 {
					br.App.ChatView.UpdateMessageReactions(m.ID, br.uniqueReactions(reacts))
				}
			}
			br.App.ChatView.ScrollToBottom()
		})
	}()
}

func (br *Bridge) Start(ctx context.Context) {
	os.MkdirAll("media", 0755)
	br.Backend.SetEventHandler(br.HandleEvent)
	br.Backend.Connect()
	go func() {
		c, err := br.DB.GetAllContacts(100)
		if err == nil && len(c) > 0 {
			br.refreshSidebar(c)
		}
	}()
}

func (br *Bridge) refreshSidebar(contacts []database.Contact) {
	glib.IdleAdd(func() {
		br.App.Sidebar.SetRefreshing(true)
		br.App.Sidebar.ClearChats()
		for _, c := range contacts {
			prefix := ""
			if c.IsGroup.Valid && c.IsGroup.Bool { prefix = "[G] " }
			br.App.Sidebar.AddChat(c.JID, prefix+c.DisplayName())
			
			if tex := br.Contacts.GetAvatarNoFetch(c.JID); tex != nil {
				br.App.Sidebar.SetAvatar(c.JID, tex)
			}
		}
		
		if br.selectedJID != nil {
			br.App.Sidebar.SelectChat(br.selectedJID.ToNonAD().String())
		}
		br.App.Sidebar.SetRefreshing(false)
	})
}

func (br *Bridge) HandleEvent(evt backend.AppEvent) {
	switch v := evt.(type) {
	case *backend.HistorySyncEvent:
		br.handleHistorySync(v)
	case *backend.MessageEvent:
		br.handleMessage(v)
	case *backend.ConnectedEvent:
		br.handleConnected()
	case *backend.QREvent:
		br.handleQR(v)
	case *backend.OfflineSyncCompletedEvent:
		br.handleOfflineSyncCompleted()
	case *backend.ReceiptEvent:
		br.handleReceipt(v)
	case *backend.ContactEvent:
		br.handleContact(v)
	case *backend.PushNameEvent:
		br.handlePushName(v)
	case *backend.MediaRetryEvent:
		br.handleMediaRetry(v)
	}
}

func (br *Bridge) handleMediaRetry(v *backend.MediaRetryEvent) {
	fmt.Printf("Bridge: Received Media Retry for %s\n", v.Info.MessageID)
	
	msg, err := br.DB.GetMessage(v.Info.MessageID)
	if err != nil { return }

	retryData, err := whatsmeow.DecryptMediaRetryNotification(v.Info, msg.MediaKey)
	if err != nil {
		fmt.Printf("Bridge: Failed to decrypt Media Retry: %v\n", err)
		return
	}

	if retryData.GetResult() != waProto.MediaRetryNotification_SUCCESS {
		fmt.Printf("Bridge: Media Retry failed with result %s\n", retryData.GetResult())
		return
	}

	msg.MediaDirectPath = sql.NullString{String: retryData.GetDirectPath(), Valid: retryData.GetDirectPath() != ""}
	
	br.DB.SaveMessage(*msg)

	// Re-trigger download
	br.handleDownloadMedia(v.Info.MessageID)
}

func (br *Bridge) handleHistorySync(v *backend.HistorySyncEvent) {
	br.isSyncing = true
	go func() {
		for _, conv := range v.Data.Data.GetConversations() {
			chatJID, _ := types.ParseJID(conv.GetID()); chatJID = chatJID.ToNonAD()
			br.DB.SaveContact(database.Contact{JID: chatJID.String(), IsGroup: sql.NullBool{Bool: chatJID.Server == types.GroupServer, Valid: true}})
			for _, hMsg := range conv.GetMessages() {
				pMsg, err := br.Backend.Client.ParseWebMessage(chatJID, hMsg.GetMessage())
				if err == nil {
					br.persistMessage(pMsg)
					if pMsg.Message.GetReactionMessage() != nil {
						br.handleReactionInternal(pMsg.Info.Chat, pMsg.Info.Sender, pMsg.Message.GetReactionMessage().GetText(), pMsg.Message.GetReactionMessage().GetKey().GetID(), pMsg.Info.Timestamp)
					}
				}
			}
		}
		br.isSyncing = false; c, _ := br.DB.GetAllContacts(100); br.refreshSidebar(c)
	}()
}

func (br *Bridge) handleMessage(v *backend.MessageEvent) {
	msg := v.Info
	if react := msg.Message.GetReactionMessage(); react != nil {
		br.handleReactionInternal(msg.Info.Chat, msg.Info.Sender, react.GetText(), react.GetKey().GetID(), msg.Info.Timestamp)
		return
	}

	br.persistMessage(msg)
	
	// Process message through hooks
	br.Pipeline.Process(msg)
	
	resolvedChat := br.resolveJID(msg.Info.Chat)
	if !br.isSyncing || (br.selectedJID != nil && resolvedChat.ToNonAD().String() == br.selectedJID.ToNonAD().String()) {
		jid := resolvedChat.ToNonAD().String()

		// Automatically mark as read if this chat is currently selected
		if br.selectedJID != nil && jid == br.selectedJID.ToNonAD().String() {
			go br.Backend.MarkRead(br.ctx, msg.Info.Chat, []string{msg.Info.ID}, msg.Info.Sender, time.Now())
		}

		glib.IdleAdd(func() {
			br.App.Sidebar.MoveChatToTop(jid)
			if br.selectedJID != nil && resolvedChat.ToNonAD().String() == br.selectedJID.ToNonAD().String() {
				tStr := msg.Info.Timestamp.Format("15:04"); sName := ""; var av *gdk.Texture; isG := msg.Info.Chat.Server == types.GroupServer
				
				resolvedSender := br.resolveJID(msg.Info.Sender)
				sJID := resolvedSender.ToNonAD().String()
				isCont := sJID == br.lastSender
				
				if isG && !msg.Info.IsFromMe {
					if !isCont {
						sName = br.Contacts.ResolveSenderName(sJID)
						av = br.Contacts.GetAvatar(sJID)
					}
				}
				br.lastSender = sJID
				
				var qID, qSender, qContent string
				var qSenderName string
				if ci := br.extractContextInfo(msg); ci != nil && ci.GetStanzaID() != "" {
					qID = ci.GetStanzaID()
					qSender = ci.GetParticipant()
					if qSender != "" {
						qSenderName = br.Contacts.ResolveSenderName(qSender)
					}
					if qm := ci.GetQuotedMessage(); qm != nil {
						if qm.GetConversation() != "" {
							qContent = qm.GetConversation()
						} else if qm.GetExtendedTextMessage() != nil {
							qContent = qm.GetExtendedTextMessage().GetText()
						} else {
							qContent = "[Quoted Media]"
						}
					}
				}
				
				var mW, mH int
				if img := msg.Message.GetImageMessage(); img != nil {
					mW = int(img.GetWidth()); mH = int(img.GetHeight())
					texThumb := br.bytesToTexture(img.GetJPEGThumbnail())
					br.App.ChatView.AddImage(msg.Info.ID, sJID, sName, nil, texThumb, msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent, mW, mH)
				} else if stkr := msg.Message.GetStickerMessage(); stkr != nil {
					mW = int(stkr.GetWidth()); mH = int(stkr.GetHeight())
					texThumb := br.bytesToTexture(stkr.GetPngThumbnail())
					br.App.ChatView.AddSticker(msg.Info.ID, sJID, sName, nil, texThumb, msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent, mW, mH)
				} else if vid := msg.Message.GetVideoMessage(); vid != nil {
					mW = int(vid.GetWidth()); mH = int(vid.GetHeight())
					texThumb := br.bytesToTexture(vid.GetJPEGThumbnail())
					br.App.ChatView.AddVideo(msg.Info.ID, sJID, sName, texThumb, msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent, mW, mH)
				} else if aud := msg.Message.GetAudioMessage(); aud != nil {
					br.App.ChatView.AddAudio(msg.Info.ID, sJID, sName, msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent)
				} else if doc := msg.Message.GetDocumentMessage(); doc != nil {
					texThumb := br.bytesToTexture(doc.GetJPEGThumbnail())
					br.App.ChatView.AddDocument(msg.Info.ID, sJID, sName, doc.GetFileName(), texThumb, msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent)
				} else if poll := msg.Message.GetPollCreationMessage(); poll != nil {
					var opts []string
					for _, o := range poll.GetOptions() {
						opts = append(opts, o.GetOptionName())
					}
					br.App.ChatView.AddPoll(msg.Info.ID, sJID, sName, poll.GetName(), opts, msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent)
				} else {
					content := br.extractContent(msg)
					if content != "" {
						br.App.ChatView.AddMessage(msg.Info.ID, sJID, sName, content, msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent)
					}
				}
				br.App.ChatView.ScrollToBottom()
			}
		})
	}
}

func (br *Bridge) handleReactionInternal(chat, sender types.JID, text, targetID string, timestamp time.Time) {
	react := database.Reaction{
		MessageID: targetID,
		SenderJID: sender.ToNonAD().String(),
		Reaction:  text,
		Timestamp: timestamp,
	}
	br.DB.SaveReaction(react)
	
	if br.selectedJID != nil && chat.ToNonAD().String() == br.selectedJID.ToNonAD().String() {
		reactions, _ := br.DB.GetReactions(targetID)
		br.App.ChatView.UpdateMessageReactions(targetID, br.uniqueReactions(reactions))
	}
}

func (br *Bridge) uniqueReactions(reactions []database.Reaction) []string {
	unique := make(map[string]bool)
	var result []string
	for _, r := range reactions {
		if !unique[r.Reaction] {
			unique[r.Reaction] = true
			result = append(result, r.Reaction)
		}
	}
	return result
}

func (br *Bridge) handleDownloadMedia(id string) {
	fmt.Printf("Bridge: handleDownloadMedia called for %s\n", id)
	msg, err := br.DB.GetMessage(id)
	if err != nil { 
		fmt.Printf("Bridge: Message %s not found in DB\n", id)
		return 
	}

	if msg.MediaURL.String == "" && msg.MediaDirectPath.String == "" {
		fmt.Printf("Bridge: Message %s has no media URLs\n", id)
		return 
	}

	metadata := &MediaMetadata{
		URL: msg.MediaURL.String, DirectPath: msg.MediaDirectPath.String,
		MediaKey: msg.MediaKey, Mimetype: msg.MediaMimetype.String,
		FileEncSHA256: msg.MediaEncSHA256, FileSHA256: msg.MediaSHA256,
		FileLength: uint64(msg.MediaLength.Int64),
	}

	fmt.Printf("Bridge: Sending DownloadTask for %s (type %s)\n", id, msg.Type)
	br.Media.Download(DownloadTask{
		ID: id, ChatJID: msg.ChatJID, SenderJID: msg.SenderJID, MsgType: msg.Type, Metadata: metadata,
	})
}
func (br *Bridge) bytesToTexture(data []byte) *gdk.Texture {
	if len(data) == 0 { return nil }
	loader := gdkpixbuf.NewPixbufLoader()
	loader.Write(data)
	loader.Close()
	pix := loader.Pixbuf()
	if pix == nil { return nil }
	return gdk.NewTextureForPixbuf(pix)
}

func (br *Bridge) handleQR(v *backend.QREvent) {
	png, err := qrcode.Encode(v.Code, qrcode.Medium, 256)
	if err != nil {
		fmt.Printf("Bridge: Failed to generate QR code: %v\n", err)
		return
	}

	loader := gdkpixbuf.NewPixbufLoader()
	loader.Write(png)
	loader.Close()
	pix := loader.Pixbuf()
	if pix == nil {
		return
	}
	tex := gdk.NewTextureForPixbuf(pix)

	glib.IdleAdd(func() {
		br.App.ShowQRCode(tex)
	})
}

func (br *Bridge) handleConnected() {
	glib.IdleAdd(func() {
		br.App.HideQRCode()
	})
	go func() {
		br.Contacts.Sync(br.ctx)
		c, _ := br.DB.GetAllContacts(200)
		br.refreshSidebar(c)
	}()
}

func (br *Bridge) handleOfflineSyncCompleted() {
	br.isSyncing = false
	go func() {
		c, _ := br.DB.GetAllContacts(100)
		br.refreshSidebar(c)
	}()
}

func (br *Bridge) handleReceipt(v *backend.ReceiptEvent) {
	chatJID := v.Info.Chat.ToNonAD().String(); status := "sent"
	if v.Info.Type == types.ReceiptTypeDelivered { status = "delivered" }
	if v.Info.Type == types.ReceiptTypeRead || v.Info.Type == types.ReceiptTypeReadSelf { status = "read" }
	for _, id := range v.Info.MessageIDs {
		br.DB.UpdateMessageStatus(id, chatJID, status)
		if br.selectedJID != nil && br.selectedJID.ToNonAD().String() == chatJID {
			br.App.ChatView.UpdateMessageStatus(id, status)
		}
	}
}

func (br *Bridge) handleContact(v *backend.ContactEvent) {
	if v.Info.Action != nil {
		jid := v.Info.JID.ToNonAD().String(); pnJID := v.Info.Action.GetPnJID()
		if pnJID != "" && strings.HasSuffix(jid, "@lid") { br.DB.MergeLID(pnJID+"@s.whatsapp.net", jid) }
		br.DB.SaveContact(database.Contact{JID: jid, SavedName: sql.NullString{String: v.Info.Action.GetFullName(), Valid: v.Info.Action.GetFullName() != ""}, PushName: sql.NullString{String: v.Info.Action.GetFirstName(), Valid: v.Info.Action.GetFirstName() != ""}})
	}
}

func (br *Bridge) handlePushName(v *backend.PushNameEvent) {
	br.DB.SaveContact(database.Contact{JID: v.Info.JID.ToNonAD().String(), PushName: sql.NullString{String: v.Info.NewPushName, Valid: v.Info.NewPushName != ""}})
}

func (br *Bridge) persistMediaMessage(msg *events.Message, msgType, path string) {
	chatJID := msg.Info.Chat.ToNonAD().String(); senderJID := msg.Info.Sender.ToNonAD().String()
	var thumb []byte
	var width, height int64
	if img := msg.Message.GetImageMessage(); img != nil { 
		thumb = img.GetJPEGThumbnail()
		width = int64(img.GetWidth())
		height = int64(img.GetHeight())
	} else if stkr := msg.Message.GetStickerMessage(); stkr != nil { 
		thumb = stkr.GetPngThumbnail() 
		width = int64(stkr.GetWidth())
		height = int64(stkr.GetHeight())
	}
	br.DB.SaveMessage(database.Message{
		ID: msg.Info.ID, ChatJID: chatJID, SenderJID: senderJID, Content: path, Type: msgType, 
		Timestamp: msg.Info.Timestamp, IsFromMe: msg.Info.IsFromMe, Thumbnail: thumb,
		MediaWidth: sql.NullInt64{Int64: width, Valid: width > 0},
		MediaHeight: sql.NullInt64{Int64: height, Valid: height > 0},
	})
	br.DB.UpdateContactTimestamp(chatJID, msg.Info.Timestamp)
}

func (br *Bridge) resolveJID(jid types.JID) types.JID {
	jStr := jid.ToNonAD().String()
	if strings.HasSuffix(jStr, "@lid") {
		if c, err := br.DB.GetContact(jStr); err == nil && c.LID.Valid && c.LID.String != "" {
			// This is actually tricky because MergeLID stores PN as JID and LID as LID.
			// Let's check if we have a contact where LID is this JID.
			if target, err := br.DB.GetContactByLID(jStr); err == nil {
				if parsed, err := types.ParseJID(target.JID); err == nil {
					return parsed.ToNonAD()
				}
			}
		}
	}
	return jid.ToNonAD()
}

func (br *Bridge) persistMessage(msg *events.Message) {
	if msg.Message.GetReactionMessage() != nil || 
	   msg.Message.GetProtocolMessage() != nil ||
	   msg.Message.GetSenderKeyDistributionMessage() != nil {
		return
	}

	msgType := "text"; content := br.extractContent(msg); var thumb []byte
	var metadata database.Message

	if img := msg.Message.GetImageMessage(); img != nil {
		msgType = "image"; thumb = img.GetJPEGThumbnail()
		metadata.MediaURL = sql.NullString{String: img.GetURL(), Valid: img.GetURL() != ""}
		metadata.MediaDirectPath = sql.NullString{String: img.GetDirectPath(), Valid: img.GetDirectPath() != ""}
		metadata.MediaKey = img.GetMediaKey()
		metadata.MediaMimetype = sql.NullString{String: img.GetMimetype(), Valid: img.GetMimetype() != ""}
		metadata.MediaEncSHA256 = img.GetFileEncSHA256()
		metadata.MediaSHA256 = img.GetFileSHA256()
		metadata.MediaLength = sql.NullInt64{Int64: int64(img.GetFileLength()), Valid: true}
		metadata.MediaWidth = sql.NullInt64{Int64: int64(img.GetWidth()), Valid: true}
		metadata.MediaHeight = sql.NullInt64{Int64: int64(img.GetHeight()), Valid: true}
	} else if stkr := msg.Message.GetStickerMessage(); stkr != nil {
		msgType = "sticker"; thumb = stkr.GetPngThumbnail()
		metadata.MediaURL = sql.NullString{String: stkr.GetURL(), Valid: stkr.GetURL() != ""}
		metadata.MediaDirectPath = sql.NullString{String: stkr.GetDirectPath(), Valid: stkr.GetDirectPath() != ""}
		metadata.MediaKey = stkr.GetMediaKey()
		metadata.MediaMimetype = sql.NullString{String: stkr.GetMimetype(), Valid: stkr.GetMimetype() != ""}
		metadata.MediaEncSHA256 = stkr.GetFileEncSHA256()
		metadata.MediaSHA256 = stkr.GetFileSHA256()
		metadata.MediaLength = sql.NullInt64{Int64: int64(stkr.GetFileLength()), Valid: true}
		metadata.MediaWidth = sql.NullInt64{Int64: int64(stkr.GetWidth()), Valid: true}
		metadata.MediaHeight = sql.NullInt64{Int64: int64(stkr.GetHeight()), Valid: true}
	} else if vid := msg.Message.GetVideoMessage(); vid != nil {
		msgType = "video"; thumb = vid.GetJPEGThumbnail()
		metadata.MediaURL = sql.NullString{String: vid.GetURL(), Valid: vid.GetURL() != ""}
		metadata.MediaDirectPath = sql.NullString{String: vid.GetDirectPath(), Valid: vid.GetDirectPath() != ""}
		metadata.MediaKey = vid.GetMediaKey()
		metadata.MediaMimetype = sql.NullString{String: vid.GetMimetype(), Valid: vid.GetMimetype() != ""}
		metadata.MediaEncSHA256 = vid.GetFileEncSHA256()
		metadata.MediaSHA256 = vid.GetFileSHA256()
		metadata.MediaLength = sql.NullInt64{Int64: int64(vid.GetFileLength()), Valid: true}
		metadata.MediaWidth = sql.NullInt64{Int64: int64(vid.GetWidth()), Valid: true}
		metadata.MediaHeight = sql.NullInt64{Int64: int64(vid.GetHeight()), Valid: true}
	} else if doc := msg.Message.GetDocumentMessage(); doc != nil {
		msgType = "document"; thumb = doc.GetJPEGThumbnail()
		metadata.MediaURL = sql.NullString{String: doc.GetURL(), Valid: doc.GetURL() != ""}
		metadata.MediaDirectPath = sql.NullString{String: doc.GetDirectPath(), Valid: doc.GetDirectPath() != ""}
		metadata.MediaKey = doc.GetMediaKey()
		metadata.MediaMimetype = sql.NullString{String: doc.GetMimetype(), Valid: doc.GetMimetype() != ""}
		metadata.MediaEncSHA256 = doc.GetFileEncSHA256()
		metadata.MediaSHA256 = doc.GetFileSHA256()
		metadata.MediaLength = sql.NullInt64{Int64: int64(doc.GetFileLength()), Valid: true}
	} else if aud := msg.Message.GetAudioMessage(); aud != nil {
		msgType = "audio"
		metadata.MediaURL = sql.NullString{String: aud.GetURL(), Valid: aud.GetURL() != ""}
		metadata.MediaDirectPath = sql.NullString{String: aud.GetDirectPath(), Valid: aud.GetDirectPath() != ""}
		metadata.MediaKey = aud.GetMediaKey()
		metadata.MediaMimetype = sql.NullString{String: aud.GetMimetype(), Valid: aud.GetMimetype() != ""}
		metadata.MediaEncSHA256 = aud.GetFileEncSHA256()
		metadata.MediaSHA256 = aud.GetFileSHA256()
		metadata.MediaLength = sql.NullInt64{Int64: int64(aud.GetFileLength()), Valid: true}
	}

	// Check if file already exists on disk
	if msgType != "text" {
		ext := ".jpg"
		switch msgType {
		case "sticker": ext = ".webp"
		case "video": ext = ".mp4"
		case "audio": ext = ".ogg"
		case "document": ext = ".bin"
		}
		path := filepath.Join("media", msg.Info.ID+ext)
		if _, err := os.Stat(path); err == nil {
			content = path
		}
	}

	// Only save if it has content or is a known media type
	if content == "" && msgType == "text" {
		return
	}

	if (msgType == "image" || msgType == "sticker" || msgType == "video") && len(thumb) == 0 {
		fmt.Printf("Bridge: Media %s arrived without thumbnail (ID: %s)\n", msgType, msg.Info.ID)
	}

	chatJID := br.resolveJID(msg.Info.Chat).String()
	senderJID := br.resolveJID(msg.Info.Sender).String()
	
	if strings.HasSuffix(msg.Info.Sender.String(), "@lid") { br.Contacts.ResolveLIDMapping(msg.Info.Sender.String()) }
	
	metadata.ID = msg.Info.ID; metadata.ChatJID = chatJID; metadata.SenderJID = senderJID
	metadata.Content = content; metadata.Type = msgType; metadata.Timestamp = msg.Info.Timestamp
	metadata.IsFromMe = msg.Info.IsFromMe; metadata.Thumbnail = thumb

	// Extract Quoted Message Context
	if ci := br.extractContextInfo(msg); ci != nil && ci.GetStanzaID() != "" {
		metadata.QuotedMsgID = sql.NullString{String: ci.GetStanzaID(), Valid: true}
		metadata.QuotedMsgSender = sql.NullString{String: ci.GetParticipant(), Valid: ci.GetParticipant() != ""}
		
		// Extract quoted content (this is simplified, might need more types)
		quotedContent := ""
		if qm := ci.GetQuotedMessage(); qm != nil {
			if qm.GetConversation() != "" {
				quotedContent = qm.GetConversation()
			} else if qm.GetExtendedTextMessage() != nil {
				quotedContent = qm.GetExtendedTextMessage().GetText()
			} else if qm.GetImageMessage() != nil {
				quotedContent = "[Image]"
				if qm.GetImageMessage().GetCaption() != "" { quotedContent += ": " + qm.GetImageMessage().GetCaption() }
			} else if qm.GetVideoMessage() != nil {
				quotedContent = "[Video]"
				if qm.GetVideoMessage().GetCaption() != "" { quotedContent += ": " + qm.GetVideoMessage().GetCaption() }
			} else if qm.GetAudioMessage() != nil {
				quotedContent = "[Audio]"
			} else if qm.GetStickerMessage() != nil {
				quotedContent = "[Sticker]"
			} else if qm.GetDocumentMessage() != nil {
				quotedContent = "[Document]"
				if qm.GetDocumentMessage().GetFileName() != "" { quotedContent += ": " + qm.GetDocumentMessage().GetFileName() }
			}
		}
		metadata.QuotedMsgContent = sql.NullString{String: quotedContent, Valid: quotedContent != ""}
	}
	
	br.DB.SaveMessage(metadata)
	br.DB.UpdateContactTimestamp(chatJID, msg.Info.Timestamp)
	
	if !msg.Info.IsFromMe {
		pushName := msg.Info.PushName; fullName := ""
		contactInfo, err := br.Backend.Client.Store.Contacts.GetContact(br.ctx, msg.Info.Sender)
		if err == nil && contactInfo.Found { if pushName == "" { pushName = contactInfo.PushName }; fullName = contactInfo.FullName }
		if pushName != "" || fullName != "" { br.DB.SaveContact(database.Contact{JID: senderJID, SavedName: sql.NullString{String: fullName, Valid: fullName != ""}, PushName: sql.NullString{String: pushName, Valid: pushName != ""}}) }
	}
}

func (br *Bridge) extractContextInfo(msg *events.Message) *waProto.ContextInfo {
	if etm := msg.Message.GetExtendedTextMessage(); etm != nil {
		return etm.GetContextInfo()
	}
	if img := msg.Message.GetImageMessage(); img != nil {
		return img.GetContextInfo()
	}
	if vid := msg.Message.GetVideoMessage(); vid != nil {
		return vid.GetContextInfo()
	}
	if aud := msg.Message.GetAudioMessage(); aud != nil {
		return aud.GetContextInfo()
	}
	if doc := msg.Message.GetDocumentMessage(); doc != nil {
		return doc.GetContextInfo()
	}
	if stkr := msg.Message.GetStickerMessage(); stkr != nil {
		return stkr.GetContextInfo()
	}
	return nil
}

func (br *Bridge) extractContent(msg *events.Message) string {
	if msg.Message.GetConversation() != "" {
		return msg.Message.GetConversation()
	}
	if msg.Message.GetExtendedTextMessage().GetText() != "" {
		return msg.Message.GetExtendedTextMessage().GetText()
	}
	if msg.Message.GetImageMessage().GetCaption() != "" {
		return msg.Message.GetImageMessage().GetCaption()
	}
	if msg.Message.GetVideoMessage().GetCaption() != "" {
		return msg.Message.GetVideoMessage().GetCaption()
	}
	if msg.Message.GetDocumentMessage().GetCaption() != "" {
		return msg.Message.GetDocumentMessage().GetCaption()
	}

	// Placeholder for unimplemented types to avoid empty bubbles
	if msg.Message.GetAudioMessage() != nil {
		return "[Audio Message]"
	}
	if msg.Message.GetDocumentMessage() != nil {
		return fmt.Sprintf("[Document: %s]", msg.Message.GetDocumentMessage().GetFileName())
	}
	if msg.Message.GetPollCreationMessage() != nil {
		return fmt.Sprintf("[Poll: %s]", msg.Message.GetPollCreationMessage().GetName())
	}
	if msg.Message.GetContactMessage() != nil {
		return fmt.Sprintf("[Contact: %s]", msg.Message.GetContactMessage().GetDisplayName())
	}
	if msg.Message.GetLocationMessage() != nil {
		return "[Location]"
	}

	return ""
}
