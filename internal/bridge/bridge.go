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
	"whats-gtk/internal/database"
	"whats-gtk/internal/ui"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type Bridge struct {
	Backend       *backend.Backend
	App           *ui.App
	DB            *database.AppDB
	ctx           context.Context
	
	Contacts      *ContactService
	Media         *MediaService

	selectedJID   *types.JID
	lastSender    string
	sidebarMutex  sync.Mutex
	isSyncing     bool
	lastGroupSync map[string]time.Time
}

func NewBridge(b *backend.Backend, a *ui.App, db *database.AppDB, ctx context.Context) *Bridge {
	br := &Bridge{
		Backend: b, App: a, DB: db, ctx: ctx,
		lastGroupSync: make(map[string]time.Time),
	}

	br.Contacts = NewContactService(b, db, ctx)
	br.Media = NewMediaService(b, db, ctx)

	br.setupUIHandlers()
	br.setupServiceHandlers()

	return br
}

func (br *Bridge) setupUIHandlers() {
	br.App.Sidebar.OnChatSelected = br.handleChatSelected
	br.App.ChatView.OnSendMessage = br.handleSendMessage
	br.App.Sidebar.OnSearch = br.handleSearch
	br.App.ChatView.OnPasteImage = br.handlePasteImage
	br.App.ChatView.OnDownloadMedia = br.handleDownloadMedia
	br.App.ChatView.OnSendReaction = br.handleSendReaction
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

func (br *Bridge) handlePasteImage(pixbuf *gdk.Pixbuf) {
	if br.selectedJID == nil { return }
	targetJID := *br.selectedJID
	now := time.Now().Format("15:04")

	glib.IdleAdd(func() {
		if br.selectedJID != nil && br.selectedJID.ToNonAD().String() == targetJID.ToNonAD().String() {
			br.App.ChatView.AddImage("temp_img", "", "", pixbuf, nil, true, false, "pending", now, nil)
			br.App.ChatView.ScrollToBottom()
		}
	})

	go func() {
		// Convert pixbuf to jpeg via temp file
		tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("paste_%d.jpg", time.Now().UnixNano()))
		err := pixbuf.SaveJPEG(tmpPath, 80)
		if err != nil {
			fmt.Printf("Bridge: Failed to save temp image: %v\n", err)
			return
		}
		defer os.Remove(tmpPath)

		data, err := os.ReadFile(tmpPath)
		if err != nil { return }

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
			}
		})

		// Save to DB and media folder
		path := filepath.Join("media", resp.ID+".jpg")
		os.WriteFile(path, data, 0644)
		br.DB.SaveMessage(database.Message{
			ID: resp.ID, ChatJID: targetJID.ToNonAD().String(), SenderJID: br.Backend.Device.ID.ToNonAD().String(),
			Content: path, Type: "image", Timestamp: resp.Timestamp, Status: "sent", IsFromMe: true,
		})
		br.DB.UpdateContactTimestamp(targetJID.ToNonAD().String(), resp.Timestamp)
	}()
}

func (br *Bridge) setupServiceHandlers() {
	br.Contacts.SetOnAvatarSet(func(jid string, pixbuf *gdk.Pixbuf) {
		br.App.ChatView.SetAvatar(jid, pixbuf)
	})

	br.Media.SetOnMediaDownloaded(func(task DownloadTask, data []byte, path string) {
		glib.IdleAdd(func() {
			if data == nil { data, _ = os.ReadFile(path) }
			if data == nil { return }
			
			loader, lErr := gdk.PixbufLoaderNew(); if lErr != nil { return }
			if _, err := loader.Write(data); err != nil { loader.Close(); return }
			loader.Close(); pixbuf, _ := loader.GetPixbuf()
			
			if br.selectedJID != nil && br.selectedJID.ToNonAD().String() == task.ChatJID {
				br.App.ChatView.UpdateMessageImage(task.ID, pixbuf)
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
		br.App.ChatView.SetHeader(headerName, nil)
	} else {
		br.App.ChatView.SetHeader(jid.String(), nil)
	}

	br.refreshMessages(jid)

	if strings.HasSuffix(jid.String(), "@lid") && !br.isSyncing {
		go br.Contacts.ResolveLIDMapping(jid.String())
	}

	if jid.Server == types.GroupServer {
		br.syncGroupIfNeeded(jid)
	}
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

func (br *Bridge) handleSendMessage(text string) {
	if br.selectedJID == nil { return }
	targetJID := *br.selectedJID; now := time.Now().Format("15:04")
	
	glib.IdleAdd(func() {
		if br.selectedJID != nil && br.selectedJID.ToNonAD().String() == targetJID.ToNonAD().String() {
			isCont := br.lastSender == br.Backend.Device.ID.ToNonAD().String()
			br.App.ChatView.AddMessage("temp", "", "", text, true, isCont, "pending", now, nil)
			br.lastSender = br.Backend.Device.ID.ToNonAD().String()
			br.App.ChatView.ScrollToBottom()
		}
	})

	go func() {
		resp, err := br.Backend.SendText(br.ctx, targetJID, text); if err != nil { return }
		glib.IdleAdd(func() {
			if br.selectedJID != nil && br.selectedJID.ToNonAD().String() == targetJID.ToNonAD().String() {
				br.App.ChatView.UpdateMessageStatus("temp", "sent")
				if b, exists := br.App.ChatView.MessageRows["temp"]; exists {
					br.App.ChatView.MessageRows[resp.ID] = b; delete(br.App.ChatView.MessageRows, "temp")
				}
			}
		})
		br.DB.SaveMessage(database.Message{ID: resp.ID, ChatJID: targetJID.ToNonAD().String(), SenderJID: br.Backend.Device.ID.ToNonAD().String(), Content: text, Type: "text", Timestamp: resp.Timestamp, Status: "sent", IsFromMe: true})
		br.DB.UpdateContactTimestamp(targetJID.ToNonAD().String(), resp.Timestamp)
	}()
}

func (br *Bridge) handleSearch(t string) {
	go func() {
		var c []database.Contact
		if t == "" {
			c, _ = br.DB.GetAllContacts(100)
		} else {
			c, _ = br.DB.SearchContacts(t, 100)
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
				tStr := m.Timestamp.Format("15:04"); sName := ""; var av *gdk.Pixbuf; isCont := m.SenderJID == br.lastSender
				if jid.Server == types.GroupServer && !m.IsFromMe {
					if !isCont {
						sName = br.Contacts.ResolveSenderName(m.SenderJID)
						av = br.Contacts.GetAvatar(m.SenderJID)
					}
				}
				br.lastSender = m.SenderJID
				if m.Type == "image" || m.Type == "sticker" {
					var pbImg, pbThumb *gdk.Pixbuf
					pbThumb = br.bytesToPixbuf(m.Thumbnail)
					
					if _, err := os.Stat(m.Content); err == nil {
						data, _ := os.ReadFile(m.Content)
						loader, lErr := gdk.PixbufLoaderNew()
						if lErr == nil {
							if _, err := loader.Write(data); err == nil {
								loader.Close(); pbImg, _ = loader.GetPixbuf()
							} else { loader.Close() }
						}
					}
					
					if m.Type == "image" {
						br.App.ChatView.AddImage(m.ID, m.SenderJID, sName, pbImg, pbThumb, m.IsFromMe, isCont, m.Status, tStr, av)
					} else {
						br.App.ChatView.AddSticker(m.ID, m.SenderJID, sName, pbImg, pbThumb, m.IsFromMe, isCont, m.Status, tStr, av)
					}
				} else {
					br.App.ChatView.AddMessage(m.ID, m.SenderJID, sName, m.Content, m.IsFromMe, isCont, m.Status, tStr, av)
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
	br.sidebarMutex.Lock(); defer br.sidebarMutex.Unlock()
	glib.IdleAdd(func() {
		br.App.Sidebar.ClearChats()
		for _, c := range contacts {
			prefix := ""
			if c.IsGroup { prefix = "[G] " }
			br.App.Sidebar.AddChat(c.JID, prefix+c.DisplayName())
		}
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
	case *backend.OfflineSyncCompletedEvent:
		br.handleOfflineSyncCompleted()
	case *backend.ReceiptEvent:
		br.handleReceipt(v)
	case *backend.ContactEvent:
		br.handleContact(v)
	case *backend.PushNameEvent:
		br.handlePushName(v)
	}
}

func (br *Bridge) handleHistorySync(v *backend.HistorySyncEvent) {
	br.isSyncing = true
	go func() {
		for _, conv := range v.Data.Data.GetConversations() {
			chatJID, _ := types.ParseJID(conv.GetID()); chatJID = chatJID.ToNonAD()
			br.DB.SaveContact(database.Contact{JID: chatJID.String(), IsGroup: chatJID.Server == types.GroupServer})
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
	
	resolvedChat := br.resolveJID(msg.Info.Chat)
	if !br.isSyncing || (br.selectedJID != nil && resolvedChat.ToNonAD().String() == br.selectedJID.ToNonAD().String()) {
		jid := resolvedChat.ToNonAD().String()
		glib.IdleAdd(func() {
			br.App.Sidebar.MoveChatToTop(jid)
			if br.selectedJID != nil && resolvedChat.ToNonAD().String() == br.selectedJID.ToNonAD().String() {
				tStr := msg.Info.Timestamp.Format("15:04"); sName := ""; var av *gdk.Pixbuf; isG := msg.Info.Chat.Server == types.GroupServer
				
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
				
				if msg.Message.GetImageMessage() != nil || msg.Message.GetStickerMessage() != nil {
					var thumb []byte
					if img := msg.Message.GetImageMessage(); img != nil { thumb = img.GetJPEGThumbnail()
					} else { thumb = msg.Message.GetStickerMessage().GetPngThumbnail() }
					
					pbThumb := br.bytesToPixbuf(thumb)
					if msg.Message.GetImageMessage() != nil {
						br.App.ChatView.AddImage(msg.Info.ID, sJID, sName, nil, pbThumb, msg.Info.IsFromMe, isCont, "", tStr, av)
					} else {
						br.App.ChatView.AddSticker(msg.Info.ID, sJID, sName, nil, pbThumb, msg.Info.IsFromMe, isCont, "", tStr, av)
					}
				} else {
					content := br.extractContent(msg)
					if content != "" { br.App.ChatView.AddMessage(msg.Info.ID, sJID, sName, content, msg.Info.IsFromMe, isCont, "", tStr, av) }
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
	msg, err := br.DB.GetMessage(id)
	if err != nil { return }
	
	if msg.MediaURL.String == "" && msg.MediaDirectPath.String == "" { return }

	metadata := &MediaMetadata{
		URL: msg.MediaURL.String, DirectPath: msg.MediaDirectPath.String,
		MediaKey: msg.MediaKey, Mimetype: msg.MediaMimetype.String,
		FileEncSHA256: msg.MediaEncSHA256, FileSHA256: msg.MediaSHA256,
		FileLength: uint64(msg.MediaLength.Int64),
	}

	br.Media.Download(DownloadTask{
		ID: id, ChatJID: msg.ChatJID, SenderJID: msg.SenderJID, MsgType: msg.Type, Metadata: metadata,
	})
}

func (br *Bridge) bytesToPixbuf(data []byte) *gdk.Pixbuf {
	if len(data) == 0 { return nil }
	loader, err := gdk.PixbufLoaderNew()
	if err != nil { return nil }
	defer loader.Close()
	_, err = loader.Write(data)
	if err != nil { return nil }
	pix, _ := loader.GetPixbuf()
	return pix
}

func (br *Bridge) handleConnected() {
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
		br.DB.SaveContact(database.Contact{JID: jid, SavedName: sql.NullString{String: v.Info.Action.GetFullName(), Valid: v.Info.Action.GetFullName() != ""}, PushName: v.Info.Action.GetFirstName()})
	}
}

func (br *Bridge) handlePushName(v *backend.PushNameEvent) {
	br.DB.SaveContact(database.Contact{JID: v.Info.JID.ToNonAD().String(), PushName: v.Info.NewPushName})
}

func (br *Bridge) persistMediaMessage(msg *events.Message, msgType, path string) {
	chatJID := msg.Info.Chat.ToNonAD().String(); senderJID := msg.Info.Sender.ToNonAD().String()
	var thumb []byte
	if img := msg.Message.GetImageMessage(); img != nil { thumb = img.GetJPEGThumbnail()
	} else if stkr := msg.Message.GetStickerMessage(); stkr != nil { thumb = stkr.GetPngThumbnail() }
	br.DB.SaveMessage(database.Message{ID: msg.Info.ID, ChatJID: chatJID, SenderJID: senderJID, Content: path, Type: msgType, Timestamp: msg.Info.Timestamp, IsFromMe: msg.Info.IsFromMe, Thumbnail: thumb})
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
	msgType := "text"; content := br.extractContent(msg); var thumb []byte
	var metadata database.Message

	// ... (media extraction remains same)
	if img := msg.Message.GetImageMessage(); img != nil {
		msgType = "image"; thumb = img.GetJPEGThumbnail()
		metadata.MediaURL = sql.NullString{String: img.GetURL(), Valid: img.GetURL() != ""}
		metadata.MediaDirectPath = sql.NullString{String: img.GetDirectPath(), Valid: img.GetDirectPath() != ""}
		metadata.MediaKey = img.GetMediaKey()
		metadata.MediaMimetype = sql.NullString{String: img.GetMimetype(), Valid: img.GetMimetype() != ""}
		metadata.MediaEncSHA256 = img.GetFileEncSHA256()
		metadata.MediaSHA256 = img.GetFileSHA256()
		metadata.MediaLength = sql.NullInt64{Int64: int64(img.GetFileLength()), Valid: true}
	} else if stkr := msg.Message.GetStickerMessage(); stkr != nil {
		msgType = "sticker"; thumb = stkr.GetPngThumbnail()
		metadata.MediaURL = sql.NullString{String: stkr.GetURL(), Valid: stkr.GetURL() != ""}
		metadata.MediaDirectPath = sql.NullString{String: stkr.GetDirectPath(), Valid: stkr.GetDirectPath() != ""}
		metadata.MediaKey = stkr.GetMediaKey()
		metadata.MediaMimetype = sql.NullString{String: stkr.GetMimetype(), Valid: stkr.GetMimetype() != ""}
		metadata.MediaEncSHA256 = stkr.GetFileEncSHA256()
		metadata.MediaSHA256 = stkr.GetFileSHA256()
		metadata.MediaLength = sql.NullInt64{Int64: int64(stkr.GetFileLength()), Valid: true}
	} else if vid := msg.Message.GetVideoMessage(); vid != nil {
		msgType = "video"; thumb = vid.GetJPEGThumbnail()
		metadata.MediaURL = sql.NullString{String: vid.GetURL(), Valid: vid.GetURL() != ""}
		metadata.MediaDirectPath = sql.NullString{String: vid.GetDirectPath(), Valid: vid.GetDirectPath() != ""}
		metadata.MediaKey = vid.GetMediaKey()
		metadata.MediaMimetype = sql.NullString{String: vid.GetMimetype(), Valid: vid.GetMimetype() != ""}
		metadata.MediaEncSHA256 = vid.GetFileEncSHA256()
		metadata.MediaSHA256 = vid.GetFileSHA256()
		metadata.MediaLength = sql.NullInt64{Int64: int64(vid.GetFileLength()), Valid: true}
	} else if doc := msg.Message.GetDocumentMessage(); doc != nil {
		msgType = "document"; thumb = doc.GetJPEGThumbnail()
		metadata.MediaURL = sql.NullString{String: doc.GetURL(), Valid: doc.GetURL() != ""}
		metadata.MediaDirectPath = sql.NullString{String: doc.GetDirectPath(), Valid: doc.GetDirectPath() != ""}
		metadata.MediaKey = doc.GetMediaKey()
		metadata.MediaMimetype = sql.NullString{String: doc.GetMimetype(), Valid: doc.GetMimetype() != ""}
		metadata.MediaEncSHA256 = doc.GetFileEncSHA256()
		metadata.MediaSHA256 = doc.GetFileSHA256()
		metadata.MediaLength = sql.NullInt64{Int64: int64(doc.GetFileLength()), Valid: true}
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
	
	br.DB.SaveMessage(metadata)
	br.DB.UpdateContactTimestamp(chatJID, msg.Info.Timestamp)
	
	if !msg.Info.IsFromMe {
		pushName := msg.Info.PushName; fullName := ""
		contactInfo, err := br.Backend.Client.Store.Contacts.GetContact(br.ctx, msg.Info.Sender)
		if err == nil && contactInfo.Found { if pushName == "" { pushName = contactInfo.PushName }; fullName = contactInfo.FullName }
		if pushName != "" || fullName != "" { br.DB.SaveContact(database.Contact{JID: senderJID, SavedName: sql.NullString{String: fullName, Valid: fullName != ""}, PushName: pushName}) }
	}
}

func (br *Bridge) extractContent(msg *events.Message) string {
	if msg.Message.GetConversation() != "" { return msg.Message.GetConversation() }
	if msg.Message.GetExtendedTextMessage().GetText() != "" { return msg.Message.GetExtendedTextMessage().GetText() }
	return ""
}
