package bridge

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
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
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type DownloadTask struct {
	Message    *events.Message
	SenderName string
	TimeStr    string
	Avatar     *gdk.Pixbuf
	MsgType    string
	IsCont     bool
}

type Bridge struct {
	Backend       *backend.Backend
	App           *ui.App
	DB            *database.AppDB
	ctx           context.Context
	jids          []types.JID
	selectedJID   *types.JID
	lastSender    string
	sidebarMutex  sync.Mutex
	isSyncing     bool
	avatarCache   map[string]*gdk.Pixbuf
	mediaQueue    chan DownloadTask
	avatarQueue   chan string
	lastGroupSync map[string]time.Time
}

func NewBridge(b *backend.Backend, a *ui.App, db *database.AppDB, ctx context.Context) *Bridge {
	br := &Bridge{
		Backend: b, App: a, DB: db, ctx: ctx,
		avatarCache: make(map[string]*gdk.Pixbuf),
		mediaQueue:  make(chan DownloadTask, 100),
		avatarQueue: make(chan string, 500),
		lastGroupSync: make(map[string]time.Time),
	}
	go br.mediaWorker(); go br.avatarWorker()
	a.OnChatSelected = func(jidStr string) {
		jid, err := types.ParseJID(jidStr); if err != nil { return }
		jid = jid.ToNonAD(); br.selectedJID = &jid; br.lastSender = "" 
		if contact, err := br.DB.GetContact(jid.String()); err == nil {
			headerName := contact.DisplayName()
			if jid.Server == types.GroupServer { headerName = "[G] " + headerName }
			br.App.SetChatHeader(headerName, nil)
		} else { br.App.SetChatHeader(jid.String(), nil) }
		br.refreshMessages(jid)
		if strings.HasSuffix(jid.String(), "@lid") && !br.isSyncing {
			go func(lidJID types.JID) {
				_, err := br.Backend.Client.GetUserInfo(br.ctx, []types.JID{lidJID})
				if err == nil { br.resolveLIDMapping(lidJID.String()) }
			}(jid)
		}
		if jid.Server == types.GroupServer {
			lastSync, exists := br.lastGroupSync[jid.String()]
			if !exists || time.Since(lastSync) > 30*time.Minute {
				go func(groupJID types.JID) {
					info, err := br.Backend.GetGroupInfo(ctx, groupJID); if err != nil { return }
					br.lastGroupSync[groupJID.String()] = time.Now()
					for _, p := range info.Participants {
						pn := p.PhoneNumber.ToNonAD().String(); lid := p.LID.ToNonAD().String()
						if pn != "" && lid != "" { br.DB.MergeLID(pn, lid) } else { br.DB.SaveContact(database.Contact{JID: p.JID.ToNonAD().String()}) }
					}
					if br.selectedJID != nil && br.selectedJID.ToNonAD().String() == groupJID.ToNonAD().String() {
						glib.IdleAdd(func() { br.refreshMessages(groupJID) })
					}
				}(jid)
			}
		}
	}
	a.OnSendMessage = func(text string) {
		if br.selectedJID != nil {
			targetJID := *br.selectedJID; now := time.Now().Format("15:04")
			glib.IdleAdd(func() {
				if br.selectedJID != nil && br.selectedJID.ToNonAD().String() == targetJID.ToNonAD().String() {
					isCont := br.lastSender == br.Backend.Device.ID.ToNonAD().String()
					br.App.AddMessageWithID("temp", "", "", text, true, isCont, "pending", now, nil)
					br.lastSender = br.Backend.Device.ID.ToNonAD().String()
				}
			})
			go func() {
				resp, err := br.Backend.SendText(ctx, targetJID, text); if err != nil { return }
				glib.IdleAdd(func() {
					if br.selectedJID != nil && br.selectedJID.ToNonAD().String() == targetJID.ToNonAD().String() {
						br.App.UpdateMessageStatus("temp", "sent")
						if b, exists := br.App.MessageRows["temp"]; exists {
							br.App.MessageRows[resp.ID] = b; delete(br.App.MessageRows, "temp")
						}
					}
				})
				br.DB.SaveMessage(database.Message{ID: resp.ID, ChatJID: targetJID.ToNonAD().String(), SenderJID: br.Backend.Device.ID.ToNonAD().String(), Content: text, Type: "text", Timestamp: resp.Timestamp, Status: "sent", IsFromMe: true})
				br.DB.UpdateContactTimestamp(targetJID.ToNonAD().String(), resp.Timestamp)
			}()
		}
	}
	a.OnSearch = func(t string) {
		go func() {
			var c []database.Contact; if t == "" { c, _ = br.DB.GetAllContacts(100) } else { c, _ = br.DB.SearchContacts(t, 100) }
			br.refreshSidebar(c)
		}()
	}
	return br
}

func (br *Bridge) mediaWorker() {
	for task := range br.mediaQueue {
		data, err := br.Backend.DownloadMedia(br.ctx, task.Message)
		if err == nil {
			ext := ".jpg"; if task.MsgType == "sticker" { ext = ".webp" }
			path := filepath.Join("media", task.Message.Info.ID+ext)
			os.WriteFile(path, data, 0644); br.persistMediaMessage(task.Message, task.MsgType, path)
			glib.IdleAdd(func() {
				if br.selectedJID != nil && br.selectedJID.ToNonAD().String() == task.Message.Info.Chat.ToNonAD().String() {
					loader, lErr := gdk.PixbufLoaderNew(); if lErr != nil { return }
					if _, err := loader.Write(data); err != nil { loader.Close(); return }
					loader.Close(); pixbuf, _ := loader.GetPixbuf()
					senderJID := task.Message.Info.Sender.ToNonAD().String()
					if task.MsgType == "image" { br.App.AddImage(task.Message.Info.ID, senderJID, task.SenderName, pixbuf, task.Message.Info.IsFromMe, task.IsCont, "", task.TimeStr, task.Avatar)
					} else { br.App.AddSticker(task.Message.Info.ID, senderJID, task.SenderName, pixbuf, task.Message.Info.IsFromMe, task.IsCont, "", task.TimeStr, task.Avatar) }
				}
			})
		} else {
			fmt.Printf("Bridge: Media Download Failed: %v\n", err)
		}
		time.Sleep(2 * time.Second)
	}
}

func (br *Bridge) avatarWorker() {
	for jStr := range br.avatarQueue {
		if _, exists := br.avatarCache[jStr]; exists { continue }
		jid, _ := types.ParseJID(jStr)
		time.Sleep(10 * time.Second) 
		info, err := br.Backend.Client.GetProfilePictureInfo(br.ctx, jid, &whatsmeow.GetProfilePictureParams{Preview: true})
		if err == nil && info != nil && info.URL != "" {
			resp, err := http.Get(info.URL)
			if err == nil {
				defer resp.Body.Close(); data, _ := io.ReadAll(resp.Body)
				path := filepath.Join("media", "avatar_"+jid.ToNonAD().String()+".jpg")
				os.WriteFile(path, data, 0644); br.DB.SaveContact(database.Contact{JID: jid.ToNonAD().String(), AvatarPath: path})
				glib.IdleAdd(func() {
					loader, _ := gdk.PixbufLoaderNew(); loader.Write(data); loader.Close(); pixbuf, _ := loader.GetPixbuf()
					if pixbuf != nil { br.avatarCache[jStr] = pixbuf; br.App.SetAvatar(jStr, pixbuf) }
				})
			}
		}
	}
}

func (br *Bridge) refreshMessages(jid types.JID) {
	go func() {
		msgs, err := br.DB.GetMessages(jid.ToNonAD().String(), 50); if err != nil { return }
		seen := make(map[string]bool)
		for i := len(msgs) - 1; i >= 0; i-- {
			if !msgs[i].IsFromMe && !seen[msgs[i].SenderJID] { br.getAvatar(msgs[i].SenderJID); seen[msgs[i].SenderJID] = true }
		}
		glib.IdleAdd(func() {
			if br.selectedJID == nil || br.selectedJID.ToNonAD().String() != jid.ToNonAD().String() { return }
			br.App.ClearMessages(); br.lastSender = "" 
			for _, m := range msgs {
				tStr := m.Timestamp.Format("15:04"); sName := ""; var av *gdk.Pixbuf; isCont := m.SenderJID == br.lastSender
				if jid.Server == types.GroupServer && !m.IsFromMe {
					if !isCont { sName = br.resolveSenderName(m.SenderJID); av = br.getAvatar(m.SenderJID) }
				}
				br.lastSender = m.SenderJID
				if m.Type == "image" || m.Type == "sticker" {
					if _, err := os.Stat(m.Content); err == nil {
						data, _ := os.ReadFile(m.Content); loader, lErr := gdk.PixbufLoaderNew()
						if lErr == nil {
							if _, err := loader.Write(data); err == nil {
								loader.Close(); pixbuf, _ := loader.GetPixbuf()
								if m.Type == "image" { br.App.AddImage(m.ID, m.SenderJID, sName, pixbuf, m.IsFromMe, isCont, m.Status, tStr, av)
								} else { br.App.AddSticker(m.ID, m.SenderJID, sName, pixbuf, m.IsFromMe, isCont, m.Status, tStr, av) }
								continue
							}
							loader.Close()
						}
					}
					br.App.AddMessageWithID(m.ID, m.SenderJID, sName, "["+m.Type+"]", m.IsFromMe, isCont, m.Status, tStr, av)
				} else { br.App.AddMessageWithID(m.ID, m.SenderJID, sName, m.Content, m.IsFromMe, isCont, m.Status, tStr, av) }
			}
		})
	}()
}

func (br *Bridge) resolveLIDMapping(lid string) {
	jidObj, _ := types.ParseJID(lid); info, err := br.Backend.Client.Store.Contacts.GetContact(br.ctx, jidObj)
	if err == nil && info.Found && info.FullName != "" {
		all, _ := br.Backend.Client.Store.Contacts.GetAllContacts(br.ctx)
		for pnJID, pnInfo := range all {
			if !strings.HasSuffix(pnJID.String(), "@lid") && pnInfo.FullName == info.FullName {
				br.DB.MergeLID(pnJID.ToNonAD().String(), lid); break
			}
		}
	}
}

func (br *Bridge) resolveSenderName(j string) string {
	if strings.HasSuffix(j, "@lid") { br.resolveLIDMapping(j) }
	c, err := br.DB.GetContact(j)
	if err == nil {
		if c.SavedName.Valid && c.SavedName.String != "" { return c.SavedName.String }
		if c.PushName != "" { return br.formatNonAddedName(j, c.PushName) }
	}
	// Fallback to store
	jidObj, _ := types.ParseJID(j)
	if info, err := br.Backend.Client.Store.Contacts.GetContact(br.ctx, jidObj); err == nil && info.Found {
		if info.FullName != "" {
			br.DB.SaveContact(database.Contact{JID: j, SavedName: sql.NullString{String: info.FullName, Valid: true}})
			return info.FullName
		}
		if info.PushName != "" {
			br.DB.SaveContact(database.Contact{JID: j, PushName: info.PushName})
			return br.formatNonAddedName(j, info.PushName)
		}
	}
	return j
}

func (br *Bridge) formatNonAddedName(j, p string) string {
	parts := strings.Split(j, "@"); n := parts[0]; if !strings.HasSuffix(j, "@lid") { n = br.formatPhoneNumber(n) }
	return fmt.Sprintf("%s <span foreground='#8696a0' size='small'>%s</span>", p, n)
}

func (br *Bridge) formatPhoneNumber(n string) string {
	if len(n) < 10 { return n }
	if strings.HasPrefix(n, "55") && len(n) >= 12 {
		return fmt.Sprintf("+55 %s %s-%s", n[2:4], n[4:9], n[9:])
	}
	return "+" + n
}

func (br *Bridge) getAvatar(j string) *gdk.Pixbuf {
	if pix, exists := br.avatarCache[j]; exists { return pix }
	if c, err := br.DB.GetContact(j); err == nil && c.AvatarPath != "" {
		if _, err := os.Stat(c.AvatarPath); err == nil {
			if data, err := os.ReadFile(c.AvatarPath); err == nil {
				loader, _ := gdk.PixbufLoaderNew(); loader.Write(data); loader.Close(); pix, _ := loader.GetPixbuf()
				if pix != nil { br.avatarCache[j] = pix; return pix }
			}
		}
	}
	if !br.isSyncing { select { case br.avatarQueue <- j: default: } }
	return nil
}

func (br *Bridge) Start(ctx context.Context) {
	os.MkdirAll("media", 0755); br.Backend.SetEventHandler(br.HandleEvent); br.Backend.Connect()
	go func() {
		c, err := br.DB.GetAllContacts(100); if err == nil && len(c) > 0 { br.refreshSidebar(c) }
	}()
}

func (br *Bridge) Sync(ctx context.Context) {
	groups, err := br.Backend.GetJoinedGroups(ctx)
	if err == nil {
		for _, g := range groups { br.DB.SaveContact(database.Contact{JID: g.JID.ToNonAD().String(), SavedName: sql.NullString{String: g.Name, Valid: g.Name != ""}, IsGroup: true}) }
	}
	contacts, err := br.Backend.GetAllContacts(ctx)
	if err == nil {
		for j, i := range contacts { br.DB.SaveContact(database.Contact{JID: j.ToNonAD().String(), SavedName: sql.NullString{String: i.FullName, Valid: i.FullName != ""}, PushName: i.PushName, IsGroup: j.Server == types.GroupServer}) }
	}
	c, _ := br.DB.GetAllContacts(200); br.refreshSidebar(c)
}

func (br *Bridge) HealContacts(ctx context.Context) {
	contacts, err := br.DB.GetUnresolvedPNs(100); if err != nil || len(contacts) == 0 { return }
	for i := 0; i < len(contacts); i += 10 {
		end := i + 10; if end > len(contacts) { end = len(contacts) }
		batch := contacts[i:end]; jids := make([]types.JID, 0)
		for _, c := range batch { j, _ := types.ParseJID(c.JID); jids = append(jids, j) }
		resp, err := br.Backend.Client.GetUserInfo(br.ctx, jids)
		if err == nil {
			for jid, info := range resp {
				if !info.LID.IsEmpty() { br.DB.MergeLID(jid.ToNonAD().String(), info.LID.ToNonAD().String()) }
			}
		}
		time.Sleep(10 * time.Second)
	}
}

func (br *Bridge) refreshSidebar(contacts []database.Contact) {
	br.sidebarMutex.Lock(); defer br.sidebarMutex.Unlock()
	glib.IdleAdd(func() {
		br.App.ClearChats(); br.jids = nil
		for _, c := range contacts {
			jid, err := types.ParseJID(c.JID)
			if err == nil {
				br.jids = append(br.jids, jid); prefix := ""
				if c.IsGroup { prefix = "[G] " }
				br.App.AddChat(c.JID, prefix+c.DisplayName())
			}
		}
	})
}

func (br *Bridge) HandleEvent(evt backend.AppEvent) {
	switch v := evt.(type) {
	case *backend.HistorySyncEvent:
		br.isSyncing = true
		go func() {
			for _, conv := range v.Data.Data.GetConversations() {
				chatJID, _ := types.ParseJID(conv.GetID()); chatJID = chatJID.ToNonAD()
				br.DB.SaveContact(database.Contact{JID: chatJID.String(), IsGroup: chatJID.Server == types.GroupServer})
				for _, hMsg := range conv.GetMessages() {
					pMsg, err := br.Backend.Client.ParseWebMessage(chatJID, hMsg.GetMessage())
					if err == nil { br.persistMessage(pMsg) }
				}
			}
			br.isSyncing = false; c, _ := br.DB.GetAllContacts(100); br.refreshSidebar(c)
		}()
		return
	case *backend.MessageEvent:
		msg := v.Info; br.persistMessage(msg)
		if !br.isSyncing || (br.selectedJID != nil && msg.Info.Chat.ToNonAD().String() == br.selectedJID.ToNonAD().String()) {
			jid := msg.Info.Chat.ToNonAD().String()
			glib.IdleAdd(func() {
				br.App.MoveChatToTop(jid)
				if br.selectedJID != nil && br.selectedJID.ToNonAD().String() == br.selectedJID.ToNonAD().String() {
					tStr := msg.Info.Timestamp.Format("15:04"); sName := ""; var av *gdk.Pixbuf; isG := msg.Info.Chat.Server == types.GroupServer
					sJID := msg.Info.Sender.ToNonAD().String(); isCont := sJID == br.lastSender
					if isG && !msg.Info.IsFromMe {
						if !isCont { sName = br.resolveSenderName(sJID); av = br.getAvatar(sJID) }
					}
					br.lastSender = sJID
					if msg.Message.GetImageMessage() != nil || msg.Message.GetStickerMessage() != nil {
						mType := "image"; if msg.Message.GetStickerMessage() != nil { mType = "sticker" }
						br.mediaQueue <- DownloadTask{Message: msg, SenderName: sName, TimeStr: tStr, Avatar: av, MsgType: mType, IsCont: isCont}
					} else {
						content := br.extractContent(msg); if content != "" { br.persistMessage(msg); br.App.AddMessageWithID(msg.Info.ID, sJID, sName, content, msg.Info.IsFromMe, isCont, "", tStr, av) }
					}
				}
			})
		}
	case *backend.ConnectedEvent: go br.Sync(br.ctx)
	case *backend.OfflineSyncCompletedEvent:
		br.isSyncing = false; go func() { c, _ := br.DB.GetAllContacts(100); br.refreshSidebar(c) }()
	case *backend.ReceiptEvent:
		chatJID := v.Info.Chat.ToNonAD().String(); status := "sent"
		if v.Info.Type == types.ReceiptTypeDelivered { status = "delivered" }
		if v.Info.Type == types.ReceiptTypeRead || v.Info.Type == types.ReceiptTypeReadSelf { status = "read" }
		for _, id := range v.Info.MessageIDs {
			br.DB.UpdateMessageStatus(id, chatJID, status)
			if br.selectedJID != nil && br.selectedJID.ToNonAD().String() == chatJID { br.App.UpdateMessageStatus(id, status) }
		}
	case *backend.ContactEvent:
		if v.Info.Action != nil {
			jid := v.Info.JID.ToNonAD().String(); pnJID := v.Info.Action.GetPnJID()
			if pnJID != "" && strings.HasSuffix(jid, "@lid") { br.DB.MergeLID(pnJID+"@s.whatsapp.net", jid) }
			br.DB.SaveContact(database.Contact{JID: jid, SavedName: sql.NullString{String: v.Info.Action.GetFullName(), Valid: v.Info.Action.GetFullName() != ""}, PushName: v.Info.Action.GetFirstName()})
		}
	case *backend.PushNameEvent:
		br.DB.SaveContact(database.Contact{JID: v.Info.JID.ToNonAD().String(), PushName: v.Info.NewPushName})
	}
}

func (br *Bridge) persistMediaMessage(msg *events.Message, msgType, path string) {
	chatJID := msg.Info.Chat.ToNonAD().String(); senderJID := msg.Info.Sender.ToNonAD().String()
	br.DB.SaveMessage(database.Message{ID: msg.Info.ID, ChatJID: chatJID, SenderJID: senderJID, Content: path, Type: msgType, Timestamp: msg.Info.Timestamp, IsFromMe: msg.Info.IsFromMe})
	br.DB.UpdateContactTimestamp(chatJID, msg.Info.Timestamp)
}

func (br *Bridge) persistMessage(msg *events.Message) {
	msgType := "text"; content := br.extractContent(msg)
	if msg.Message.GetImageMessage() != nil { msgType = "image" } else if msg.Message.GetStickerMessage() != nil { msgType = "sticker" }
	chatJID := msg.Info.Chat.ToNonAD().String(); senderJID := msg.Info.Sender.ToNonAD().String()
	if strings.HasSuffix(senderJID, "@lid") { br.resolveLIDMapping(senderJID) }
	br.DB.SaveMessage(database.Message{ID: msg.Info.ID, ChatJID: chatJID, SenderJID: senderJID, Content: content, Type: msgType, Timestamp: msg.Info.Timestamp, IsFromMe: msg.Info.IsFromMe})
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
