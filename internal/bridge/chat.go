package bridge

import (
	"context"
	"database/sql"
	"fmt"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"whats-gtk/internal/backend"
	"whats-gtk/internal/database"
	"whats-gtk/internal/events"
	"whats-gtk/internal/ui"
	"whats-gtk/internal/ui/info"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"go.mau.fi/whatsmeow"
	meowEvents "go.mau.fi/whatsmeow/types/events"
	"go.mau.fi/whatsmeow/types"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"whats-gtk/internal/ui/chat"
)

// ChatController handles all user-initiated actions: chat selection, message sending,
// search, file uploads, reactions, pins, media downloads, and group management.
// It owns the session state (selectedJID, lastSender, search serial).
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
	ch := cc.EventBus.Subscribe(events.EventMessageReceived)
	go func() {
		for ev := range ch {
			if payload, ok := ev.Data.(events.LiveMessagePayload); ok {
				if msg, ok := payload.Msg.(*meowEvents.Message); ok {
					cc.HandleLiveMessage(msg, payload.IsSyncing)
				}
			}
		}
	}()
}

func (cc *ChatController) HandleLiveMessage(msg *meowEvents.Message, isSyncing bool) {
	resolvedChat := cc.Messages.ResolveJID(msg.Info.Chat)
	selectedJID := cc.selectedJID
	if !isSyncing || (selectedJID != nil && resolvedChat.ToNonAD().String() == selectedJID.ToNonAD().String()) {
		jidStr := resolvedChat.ToNonAD().String()

		// Automatically mark as read if this chat is currently selected
		if selectedJID != nil && jidStr == selectedJID.ToNonAD().String() {
			if cc.App.Window.IsActive() {
				go cc.Backend.MarkRead(cc.ctx, msg.Info.Chat, []string{msg.Info.ID}, msg.Info.Sender, time.Now())
			}
		}

		// Notify UI to move chat to top in sidebar
		cc.EventBus.Publish(events.Event{
			Type: events.EventChatUpdated,
			Data: jidStr,
		})

		// Fetch the persisted message from DB
		dbMsg, err := cc.DB.GetMessage(msg.Info.ID)
		if err == nil {
			tStr := msg.Info.Timestamp.Format("15:04")
			sName := ""
			var av *gdk.Texture
			isCont := false // we don't know easily for a live message unless we check last, but UI can handle it or we pass it
			
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

// SelectedJID returns the currently selected chat JID.
func (cc *ChatController) SelectedJID() *types.JID { return cc.selectedJID }

// LastSender returns the JID string of the last message sender (for continuation grouping).
func (cc *ChatController) LastSender() string { return cc.lastSender }

func (cc *ChatController) HandleWindowActive() {
	if cc.App.Window.IsActive() {
		jid := cc.SelectedJID()
		if jid != nil && cc.Backend != nil && cc.Backend.Client != nil {
			go cc.Backend.MarkRead(cc.ctx, *jid, []string{}, types.JID{}, time.Now())
		}
	}
}

// SetLastSender updates the last sender (used for continuation tracking).
func (cc *ChatController) SetLastSender(s string) { cc.lastSender = s }

// LastDateStr returns the date string of the last message.
func (cc *ChatController) LastDateStr() string { return cc.lastDateStr }

// SetLastDateStr updates the date string of the last message.
func (cc *ChatController) SetLastDateStr(s string) { cc.lastDateStr = s }

// IsSyncing is accessed from EventHandler. We use a separate field there,
// but ChatController needs to check it for LID resolution decisions.
// This is checked via EventHandler reference (set via Bridge wiring).

// HandleChatSelected opens a chat: sets header, refreshes messages, marks read,
// resolves LID if needed, and syncs group info for group chats.
func (cc *ChatController) HandleChatSelected(jidStr string) {
	jid, err := types.ParseJID(jidStr); if err != nil { return }
	jid = cc.Messages.ResolveJID(jid); cc.selectedJID = &jid; cc.lastSender = "" 
	cc.App.ActiveMainJID = jid.ToNonAD().String()
	
	if contact, err := cc.DB.GetContact(jid.String()); err == nil {
		headerName := contact.DisplayName()
		if jid.Server == types.GroupServer { headerName = "[G] " + headerName }
		cc.App.ChatView.SetHeader(headerName, cc.Contacts.GetAvatar(jid.String()))
		cc.App.InfoView.SetInfo(headerName, jid.String(), cc.Contacts.GetAvatar(jid.String()))
	} else {
		cc.App.ChatView.SetHeader(jid.String(), cc.Contacts.GetAvatar(jid.String()))
		cc.App.InfoView.SetInfo(jid.String(), jid.String(), cc.Contacts.GetAvatar(jid.String()))
	}
	cc.App.InfoFlap.SetRevealFlap(false)
	
	// Clear unread count locally and refresh sidebar
	cc.DB.ClearUnreadCount(jid.String())
	cc.RefreshSidebarUI()

	cc.RefreshMessages(jid)
	
	// Mark as read
	if cc.Backend != nil && cc.Backend.Client != nil {
		if cc.App.Window.IsActive() {
			go cc.Backend.MarkRead(cc.ctx, jid, []string{}, types.JID{}, time.Now())
		}
	}

	if strings.HasSuffix(jid.String(), "@lid") {
		go cc.Contacts.ResolveLIDMapping(jid.String())
	}

	if jid.Server == types.GroupServer {
		cc.SyncGroupIfNeeded(jid)
	}

	if cc.App.ChatView != nil {
		cc.App.ChatView.FocusEntry()
	}
}

// HandleSendMessage sends a text message with optional reply context, shows
// optimistic UI, and persists on success.
func (cc *ChatController) HandleSendMessage(targetJID types.JID, text string, replyToID string) {
	now := time.Now().Format("15:04")
	
	// Get Quoted Context if any
	var qID, qSender, qContent string
	if replyToID != "" {
		if qm, err := cc.DB.GetMessage(replyToID); err == nil {
			qID = qm.ID
			qSender = qm.SenderJID
			qContent = qm.Content
			if qm.Type != "text" { qContent = "[" + strings.Title(qm.Type) + "]" }
		}
	}

	tempID := "temp"
	glib.IdleAdd(func() {
		if cv := cc.App.GetChatViewForJID(targetJID.ToNonAD().String()); cv != nil {
			isCont := cc.lastSender == cc.Backend.Device.ID.ToNonAD().String()
			// For own messages, name is empty but avatar should show if available
			cv.AddMessage(tempID, "", "", text, true, isCont, "pending", now, nil, qID, qSender, qContent)
			cc.lastSender = cc.Backend.Device.ID.ToNonAD().String()
			cv.ScrollToBottom()
		}
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

		resp, err := cc.Backend.SendText(cc.ctx, targetJID, text, contextInfo); if err != nil { return }
		cc.promoteTempMessage(targetJID, tempID, resp.ID)
		cc.DB.SaveMessage(database.Message{
			ID: resp.ID, ChatJID: targetJID.ToNonAD().String(), SenderJID: cc.Backend.Device.ID.ToNonAD().String(), 
			Content: text, Type: "text", Timestamp: resp.Timestamp, Status: "sent", IsFromMe: true,
			QuotedMsgID: sql.NullString{String: qID, Valid: qID != ""},
			QuotedMsgSender: sql.NullString{String: qSender, Valid: qSender != ""},
			QuotedMsgContent: sql.NullString{String: qContent, Valid: qContent != ""},
		})
		cc.DB.UpdateContactTimestamp(targetJID.ToNonAD().String(), resp.Timestamp)
	}()
}

// RefreshMessagesAround loads messages centered around targetID.
func (cc *ChatController) RefreshMessagesAround(jid types.JID, targetID string) {
	go func() {
		jids := []string{jid.ToNonAD().String()}
		if contact, err := cc.DB.GetContact(jid.ToNonAD().String()); err == nil {
			if contact.LID.Valid && contact.LID.String != "" {
				jids = append(jids, contact.LID.String)
			}
		}

		msgs, err := cc.DB.GetMessagesAround(jids, targetID, 50)
		if err != nil || len(msgs) == 0 {
			cc.RefreshMessages(jid)
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

// CancelMessageSearch clears the search state and restores the original chat messages.
func (cc *ChatController) CancelMessageSearch(jidStr string) {
	targetJID, _ := types.ParseJID(jidStr)
	cc.RefreshMessages(targetJID)
}

func (cc *ChatController) CancelMessageSearchAndJump(jidStr string, targetID string) {
	targetJID, _ := types.ParseJID(jidStr)
	cc.RefreshMessagesAround(targetJID, targetID)
}

// HandleSearch performs a debounced search on contacts.
func (cc *ChatController) HandleSearch(t string) {
	cc.sidebarMutex.Lock()
	cc.searchSerial++
	serial := cc.searchSerial
	cc.sidebarMutex.Unlock()

	go func() {
		var c []database.Contact
		var err error
		if strings.TrimSpace(t) == "" {
			c, err = cc.DB.GetAllContacts(cc.activeProfileID, 100)
		} else {
			c, err = cc.DB.SearchContacts(cc.activeProfileID, t, 200)
		}
		
		cc.sidebarMutex.Lock()
		if serial != cc.searchSerial {
			cc.sidebarMutex.Unlock()
			return
		}
		cc.sidebarMutex.Unlock()

		if err != nil {
			fmt.Printf("Bridge: Search failed: %v\n", err)
			return
		}
		var items []events.SidebarItem
		for _, contact := range c {
			tex := cc.Contacts.GetAvatarNoFetch(contact.JID)
			items = append(items, events.SidebarItem{
				JID: contact.JID,
				Name: contact.DisplayName(),
				IsGroup: contact.IsGroup.Valid && contact.IsGroup.Bool,
				UnreadCount: contact.UnreadCount,
				IsPinned: contact.IsPinned,
				Avatar: tex,
			})
		}
		cc.EventBus.Publish(events.Event{
			Type: events.EventContactsUpdated,
			Data: items,
		})
	}()
}

// HandlePasteImage sends a pasted image with optimistic UI.
func (cc *ChatController) HandlePasteImage(targetJID types.JID, tex *gdk.Texture) {
	now := time.Now().Format("15:04")
	tempID := fmt.Sprintf("temp_%d", time.Now().UnixNano())

	glib.IdleAdd(func() {
		if cv := cc.App.GetChatViewForJID(targetJID.ToNonAD().String()); cv != nil {
			cv.AddImage(tempID, "", "", "", tex, nil, "", true, false, "pending", now, nil, "", "", "", int(tex.Width()), int(tex.Height()))
			cv.ScrollToBottom()
		}
	})

	go func() {
		// Convert texture to bytes via pixbuf
		pixbuf := gdk.PixbufGetFromTexture(tex)
		if pixbuf == nil { return }

		tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("paste_%d.png", time.Now().UnixNano()))
		err := pixbuf.Savev(tmpPath, "png", nil, nil)
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

		resp, err := cc.Backend.SendImage(cc.ctx, targetJID, data, "image/png")
		if err != nil {
			fmt.Printf("Bridge: SendImage failed: %v\n", err)
			return
		}

		cc.promoteTempMessage(targetJID, tempID, resp.ID)

		// Save to DB and media folder
		path := filepath.Join("media", resp.ID+".jpg")
		os.WriteFile(path, data, 0644)
		cc.DB.SaveMessage(database.Message{
			ID: resp.ID, ChatJID: targetJID.ToNonAD().String(), SenderJID: cc.Backend.Device.ID.ToNonAD().String(),
			Content: path, Type: "image", Timestamp: resp.Timestamp, Status: "sent", IsFromMe: true,
			MediaWidth: sql.NullInt64{Int64: int64(tex.Width()), Valid: true},
			MediaHeight: sql.NullInt64{Int64: int64(tex.Height()), Valid: true},
		})
		cc.DB.UpdateContactTimestamp(targetJID.ToNonAD().String(), resp.Timestamp)
	}()
}

// HandleSendFile sends a file (image, video, audio, or document) with optimistic UI.
func (cc *ChatController) HandleSendFile(targetJID types.JID, path string) {
	now := time.Now().Format("15:04")
	filename := filepath.Base(path)

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("Bridge: Failed to read file: %v\n", err)
		return
	}

	mimetype := mime.TypeByExtension(filepath.Ext(path))
	if mimetype == "" {
		mimetype = http.DetectContentType(data)
	}

	msgType := "document"
	if strings.HasPrefix(mimetype, "image/") {
		msgType = "image"
	} else if strings.HasPrefix(mimetype, "video/") {
		msgType = "video"
	} else if strings.HasPrefix(mimetype, "audio/") {
		msgType = "audio"
	}

	tempID := "temp_file"
	glib.IdleAdd(func() {
		if cv := cc.App.GetChatViewForJID(targetJID.ToNonAD().String()); cv != nil {
			jidStr := targetJID.ToNonAD().String()
			switch msgType {
			case "image":
				// Try to load as texture for preview
				pixbuf, _ := gdkpixbuf.NewPixbufFromFile(path)
				var tex *gdk.Texture
				if pixbuf != nil { tex = gdk.NewTextureForPixbuf(pixbuf) }
				cv.AddImage(tempID, jidStr, "", "", tex, nil, path, true, false, "pending", now, nil, "", "", "", 0, 0)
			case "document":
				cv.AddDocument(tempID, jidStr, "", filename, nil, true, false, "pending", now, nil, "", "", "")
			case "audio":
				cv.AddAudio(tempID, jidStr, "", true, false, "pending", now, nil, "", "", "")
			case "video":
				// For now video uses image bubble with no thumb or a placeholder
				cv.AddVideo(tempID, jidStr, "", "", nil, path, true, false, "pending", now, nil, "", "", "", 0, 0)
			}
			cv.ScrollToBottom()
		}
	})

	go func() {
		var resp whatsmeow.SendResponse
		var err error

		switch msgType {
		case "image":
			resp, err = cc.Backend.SendImage(cc.ctx, targetJID, data, mimetype)
		case "video":
			resp, err = cc.Backend.SendVideo(cc.ctx, targetJID, data, mimetype)
		case "audio":
			resp, err = cc.Backend.SendAudio(cc.ctx, targetJID, data, mimetype)
		case "document":
			resp, err = cc.Backend.SendDocument(cc.ctx, targetJID, data, mimetype, filename)
		}

		if err != nil {
			fmt.Printf("Bridge: SendFile failed: %v\n", err)
			return
		}

		cc.promoteTempMessage(targetJID, tempID, resp.ID)

		// Save to DB and media folder
		ext := filepath.Ext(path)
		if ext == "" {
			ext = ".bin"
			switch msgType {
			case "image": ext = ".jpg"
			case "video": ext = ".mp4"
			case "audio": ext = ".ogg"
			}
		}
		
		dbPath := filepath.Join("media", resp.ID+ext)
		os.WriteFile(dbPath, data, 0644)
		
		cc.DB.SaveMessage(database.Message{
			ID: resp.ID, ChatJID: targetJID.ToNonAD().String(), SenderJID: cc.Backend.Device.ID.ToNonAD().String(),
			Content: dbPath, Type: msgType, Timestamp: resp.Timestamp, Status: "sent", IsFromMe: true,
		})
		cc.DB.UpdateContactTimestamp(targetJID.ToNonAD().String(), resp.Timestamp)
	}()
}

// HandleSendReaction sends a reaction to a message.
func (cc *ChatController) HandleSendReaction(targetJID types.JID, id, emoji string) {
	msg, err := cc.DB.GetMessage(id); if err != nil { return }
	
	go func() {
		_, err := cc.Backend.SendReaction(cc.ctx, targetJID, id, msg.IsFromMe, emoji)
		if err == nil {
			cc.Messages.HandleReaction(targetJID, cc.Backend.Device.ID.ToNonAD(), emoji, id, time.Now())
		} else {
			fmt.Printf("Bridge: SendReaction failed: %v\n", err)
		}
	}()
}

// HandlePinMessage pins or unpins a message.
func (cc *ChatController) HandlePinMessage(targetJID types.JID, id string, pin bool, duration uint32) {
	msg, err := cc.DB.GetMessage(id); if err != nil { return }
	
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

// HandleDownloadMedia reads a message from the DB, builds MediaMetadata, and
// sends a DownloadTask to the MediaService.
func (cc *ChatController) HandleDownloadMedia(id string) {
	fmt.Printf("Bridge: handleDownloadMedia called for %s\n", id)
	msg, err := cc.DB.GetMessage(id)
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
	cc.Media.Download(DownloadTask{
		ID: id, ChatJID: msg.ChatJID, SenderJID: msg.SenderJID, MsgType: msg.Type, Metadata: metadata,
	})
}

// HandleOpenImage opens a file with the system's default application.
func (cc *ChatController) HandleOpenImage(path string) {
	fmt.Printf("Bridge: handleOpenImage called for %s\n", path)
	// On Linux use xdg-open. Should ideally be cross-platform.
	cmd := exec.Command("xdg-open", path)
	err := cmd.Start()
	if err != nil {
		fmt.Printf("Bridge: Failed to open image: %v\n", err)
	}
	// We don't wait for the command to finish
}

// GetGroupParticipantCount returns the total number of participants in a group.
func (cc *ChatController) GetGroupParticipantCount(jid types.JID) int {
	cleanJID := jid.ToNonAD().String()

	cc.groupMutex.RLock()
	info, ok := cc.cachedGroupInfo[cleanJID]
	cc.groupMutex.RUnlock()

	if ok && info != nil {
		return len(info.Participants)
	}

	if cc.Backend != nil && cc.Backend.Client != nil {
		fetched, err := cc.Backend.GetGroupInfo(cc.ctx, jid)
		if err == nil && fetched != nil {
			cc.groupMutex.Lock()
			cc.lastGroupSync[cleanJID] = time.Now()
			cc.cachedGroupInfo[cleanJID] = fetched
			cc.groupMutex.Unlock()
			return len(fetched.Participants)
		}
	}
	return 0
}

// SyncGroupIfNeeded fetches group info if it hasn't been synced recently (30 min throttle).
func (cc *ChatController) SyncGroupIfNeeded(jid types.JID) {
	if cc.Backend == nil || cc.Backend.Client == nil { return }

	cleanJID := jid.ToNonAD().String()

	cc.groupMutex.RLock()
	lastSync, exists := cc.lastGroupSync[cleanJID]
	cached, hasCached := cc.cachedGroupInfo[cleanJID]
	cc.groupMutex.RUnlock()
	
	// If we have cached info, update UI immediately
	if hasCached && cached != nil {
		cc.updateGroupInfoUI(cached)
	}

	if !exists || time.Since(lastSync) > 30*time.Minute {
		go func(groupJID types.JID) {
			info, err := cc.Backend.GetGroupInfo(cc.ctx, groupJID)
			if err != nil || info == nil { 
				fmt.Printf("Bridge: Failed to get group info for %s: %v\n", groupJID, err)
				return 
			}
			
			cc.groupMutex.Lock()
			cc.lastGroupSync[groupJID.ToNonAD().String()] = time.Now()
			cc.cachedGroupInfo[groupJID.ToNonAD().String()] = info
			cc.groupMutex.Unlock()
			
			for _, p := range info.Participants {
				pn := p.PhoneNumber.ToNonAD().String(); lid := p.LID.ToNonAD().String()
				if pn != "" && lid != "" {
					cc.DB.MergeLID(pn, lid)
				} else {
					cc.DB.SaveContact(database.Contact{JID: p.JID.ToNonAD().String()})
				}
			}
			
			cc.updateGroupInfoUI(info)
			
			if cc.selectedJID != nil && cc.selectedJID.ToNonAD().String() == groupJID.ToNonAD().String() {
				cc.RefreshMessages(groupJID)
			}
		}(jid)
	}
}

func (cc *ChatController) updateGroupInfoUI(grpInfo *types.GroupInfo) {
	if grpInfo == nil { return }
	
	var participants []info.ParticipantModel
	for _, p := range grpInfo.Participants {
		name := p.JID.User
		contact, err := cc.DB.GetContact(p.JID.String())
		if err == nil && contact.DisplayName() != "" {
			name = contact.DisplayName()
		} else if p.DisplayName != "" {
			name = p.DisplayName
		}
		
		participants = append(participants, info.ParticipantModel{
			Name:    name,
			JID:     p.JID.String(),
			IsAdmin: p.IsAdmin || p.IsSuperAdmin,
			Avatar:  cc.Contacts.GetAvatar(p.JID.String()),
		})
	}
	
	glib.IdleAdd(func() {
		if cc.selectedJID != nil && cc.selectedJID.ToNonAD().String() == grpInfo.JID.ToNonAD().String() {
			cc.App.InfoView.SetGroupDetails(grpInfo.Topic, participants)
		}
	})
}


// promoteTempMessage replaces a temporary message ID with the real one in the UI.
// This deduplicates the optimistic UI pattern used by HandleSendMessage,
// HandlePasteImage, and HandleSendFile.
func (cc *ChatController) promoteTempMessage(targetJID types.JID, tempID, realID string) {
	glib.IdleAdd(func() {
		if cc.selectedJID != nil && cc.selectedJID.ToNonAD().String() == targetJID.ToNonAD().String() {
			cc.App.ChatView.UpdateMessageStatus(tempID, "sent")
			if b, exists := cc.App.ChatView.MessageList.MessageRows[tempID]; exists {
				cc.App.ChatView.MessageList.MessageRows[realID] = b; delete(cc.App.ChatView.MessageList.MessageRows, tempID)
			}
			if r, exists := cc.App.ChatView.MessageList.MessageListRows[tempID]; exists {
				cc.App.ChatView.MessageList.MessageListRows[realID] = r; delete(cc.App.ChatView.MessageList.MessageListRows, tempID)
			}
		}
	})
}

func (c *ChatController) HandleDetach() {
	if c.selectedJID == nil {
		return
	}
	jidStr := c.selectedJID.ToNonAD().String()

	// If already detached, just focus it
	if _, ok := c.App.DetachedChats[jidStr]; ok {
		// Can't easily focus without window reference, but we could add Window to DetachedChats later if needed.
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
		
		// Fallback wiring
		cv.OnSendMessage = func(text, replyToID string) {
			c.HandleSendMessage(targetJID, text, replyToID)
		}
		cv.OnPasteImage = func(tex *gdk.Texture) {
			c.HandlePasteImage(targetJID, tex)
		}
		cv.OnSendFile = func(path string) {
			c.HandleSendFile(targetJID, path)
		}
		cv.OnSendReaction = func(id, emoji string) {
			c.HandleSendReaction(targetJID, id, emoji)
		}
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
		cv.OnSearchMessages = func(query string) {
			c.HandleSearch(query)
		}
		cv.OnCancelSearch = func() {
			c.CancelMessageSearch(targetJID.ToNonAD().String())
		}
		cv.OnCancelSearchAndJump = func(id string) {
			c.CancelMessageSearchAndJump(targetJID.ToNonAD().String(), id)
		}
		cv.OnSearchResultClick = func(id string) {
			glib.IdleAdd(func() {
				cv.SearchBar.Close()
			})
			c.CancelMessageSearchAndJump(targetJID.ToNonAD().String(), id)
		}
		cv.OnMentionClick = func(mjid string) {
			glib.IdleAdd(func() {
				if c.App.Sidebar != nil {
					c.App.Sidebar.SelectChat(mjid)
				}
			})
		}
		cv.OnSendPollVote = func(msgID string, senderJID string, isFromMe bool, selectedOptions []string) {
			sender, _ := types.ParseJID(senderJID)
			c.Backend.SendPollVote(context.Background(), targetJID, msgID, sender, isFromMe, selectedOptions)
		}
		cv.OnPinMessage = func(id string, pin bool, duration uint32) {
			c.HandlePinMessage(targetJID, id, pin, duration)
		}
		cv.OnDownloadMedia = c.HandleDownloadMedia
		cv.OnOpenImage = c.HandleOpenImage
		cv.OnDetach = c.HandleDetach

		// Connect active state for read receipts
		win.Connect("notify::is-active", func() {
			c.HandleWindowActive() // This will check all detached windows ideally, but wait, HandleWindowActive checks active main window.
		})

		win.SetContent(cv.Box)
		win.Show()

		win.Connect("close-request", func() bool {
			delete(c.App.DetachedChats, jidStr)
			return false // allow close
		})

		// Load chat history for this JID
		// We can reuse the logic from HandleChatSelected for just loading the DB history
		c.RefreshMessages(targetJID)
		
		// Clear main chat view since it's now detached
		c.App.ChatView.SetNoConversation()
		c.App.ActiveMainJID = ""
		c.selectedJID = nil
	})
}

// RefreshMessages loads messages from the database and publishes them via EventBus.
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

		// Update the controller's global tracking at the end for future live messages
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

func (cc *ChatController) RenderMessageSearch(jidStr string, query string) {
	go func() {
		jids := []string{jidStr}
		if contact, err := cc.DB.GetContact(jidStr); err == nil {
			if contact.LID.Valid && contact.LID.String != "" {
				jids = append(jids, contact.LID.String)
			}
		}

		msgs, err := cc.DB.SearchMessagesInChat(jids, query, 100)
		if err != nil {
			fmt.Printf("Controller: SearchMessagesInChat failed: %v\n", err)
			return
		}

		var uiMsgs []events.UIMessage
		var lastSender string
		var localLastDateStr string

		for _, m := range msgs {
			sName := ""
			var av *gdk.Texture
			isCont := m.SenderJID == lastSender
			
			targetJID, _ := types.ParseJID(jidStr)
			if targetJID.Server == types.GroupServer && !m.IsFromMe {
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

		cc.EventBus.Publish(events.Event{
			Type: events.EventChatMessagesLoaded,
			Data: events.ChatMessagesPayload{
				JID:      jidStr,
				Messages: uiMsgs,
				Append:   false,
			},
		})
	}()
}

func (cc *ChatController) LoadOlderMessages(jidStr string, targetID string) {
	go func() {
		before, ok := cc.OldestMessageTimes[jidStr]
		if !ok {
			return // Cannot load older
		}

		jids := []string{jidStr}
		if contact, err := cc.DB.GetContact(jidStr); err == nil {
			if contact.LID.Valid && contact.LID.String != "" {
				jids = append(jids, contact.LID.String)
			}
		}

		var msgs []database.Message
		var err error
		if targetID == "" {
			msgs, err = cc.DB.GetOlderMessages(jids, before, 50)
		} else {
			targetMsg, e := cc.DB.GetMessage(targetID)
			if e != nil || !targetMsg.Timestamp.Before(before) {
				return
			}
			msgs, err = cc.DB.GetMessagesBetween(jids, targetMsg.Timestamp, before, 300)
		}

		if err != nil || len(msgs) == 0 {
			return
		}

		cc.OldestMessageTimes[jidStr] = msgs[0].Timestamp

		var uiMsgs []events.UIMessage
		var localLastDateStr string
		for _, m := range msgs {
			sName := ""
			var av *gdk.Texture
			isCont := false // older messages reset continuation explicitly for safety
			
			targetJID, _ := types.ParseJID(jidStr)
			if targetJID.Server == types.GroupServer && !m.IsFromMe {
				sName = cc.Contacts.ResolveSenderName(m.SenderJID)
				av = cc.Contacts.GetAvatar(m.SenderJID)
			}
			
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

		cc.EventBus.Publish(events.Event{
			Type: events.EventChatMessagesLoaded,
			Data: events.ChatMessagesPayload{
				JID:      jidStr,
				Messages: uiMsgs,
				Append:   true,
			},
		})
	}()
}

// RefreshSidebarUI fetches contacts and publishes them as SidebarItems.
func (cc *ChatController) RefreshSidebarUI() {
	if c, err := cc.DB.GetAllContacts(cc.activeProfileID, 200); err == nil {
		var items []events.SidebarItem
		for _, contact := range c {
			if contact.IsArchived {
				continue
			}
			tex := cc.Contacts.GetAvatarNoFetch(contact.JID)
			items = append(items, events.SidebarItem{
				JID:         contact.JID,
				Name:        contact.DisplayName(),
				IsGroup:     contact.IsGroup.Valid && contact.IsGroup.Bool,
				UnreadCount: contact.UnreadCount,
				IsPinned:    contact.IsPinned,
				Avatar:      tex,
			})
		}
		cc.EventBus.Publish(events.Event{
			Type: events.EventContactsUpdated,
			Data: items,
		})
	}
}

func (cc *ChatController) GetActiveProfileID() int64 {
	return cc.activeProfileID
}

func (cc *ChatController) SetActiveProfileID(id int64) {
	cc.activeProfileID = id
	_ = cc.DB.SetActiveProfileID(id)
	cc.RefreshSidebarUI()
	cc.EventBus.Publish(events.Event{
		Type: events.EventActiveProfileChanged,
		Data: id,
	})
}

