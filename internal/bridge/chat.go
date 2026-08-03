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
	"whats-gtk/internal/ui"
	"whats-gtk/internal/ui/info"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"go.mau.fi/whatsmeow"
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
	Renderer *Renderer
	ctx      context.Context

	selectedJID   *types.JID
	lastSender    string
	lastDateStr   string
	sidebarMutex  sync.Mutex
	searchSerial  int
	lastGroupSync   map[string]time.Time
	cachedGroupInfo map[string]*types.GroupInfo
	groupMutex      sync.RWMutex
}

// NewChatController creates a new ChatController.
func NewChatController(b *backend.Backend, app *ui.App, db *database.AppDB, msgs *MessageService, contacts *ContactService, media *MediaService, ctx context.Context) *ChatController {
	return &ChatController{
		Backend:       b,
		App:           app,
		DB:            db,
		Messages:      msgs,
		Contacts:      contacts,
		Media:         media,
		ctx:           ctx,
		lastGroupSync:   make(map[string]time.Time),
		cachedGroupInfo: make(map[string]*types.GroupInfo),
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

// SetLastSender updates the last sender (used by Renderer for continuation tracking).
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
	if c, err := cc.DB.GetAllContacts(150); err == nil {
		cc.Renderer.RefreshSidebar(c)
	}

	cc.Renderer.RefreshMessages(jid)
	
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
			c, err = cc.DB.GetAllContacts(100)
		} else {
			c, err = cc.DB.SearchContacts(t, 200)
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
		cc.Renderer.RefreshSidebar(c)
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
				glib.IdleAdd(func() { cc.Renderer.RefreshMessages(groupJID) })
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
				c.Renderer.LoadOlderMessages(targetJID.ToNonAD().String(), cv, "")
			}
		}
		cv.OnLoadMessageRequest = func(id string) {
			if !cv.IsSearching {
				c.Renderer.LoadOlderMessages(targetJID.ToNonAD().String(), cv, id)
			}
		}
		cv.OnSearchMessages = func(query string) {
			c.Renderer.RenderMessageSearch(targetJID.ToNonAD().String(), query)
		}
		cv.OnCancelSearch = func() {
			c.Renderer.CancelMessageSearch(targetJID.ToNonAD().String())
		}
		cv.OnSearchResultClick = func(id string) {
			glib.IdleAdd(func() {
				cv.SearchBar.Close()
			})
			c.Renderer.CancelMessageSearchAndJump(targetJID.ToNonAD().String(), id)
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
		c.Renderer.RefreshMessages(targetJID)
		
		// Clear main chat view since it's now detached
		c.App.ChatView.Clear()
		c.App.ChatView.SetHeader("Select a chat", nil)
		c.App.ActiveMainJID = ""
		c.selectedJID = nil
	})
}
