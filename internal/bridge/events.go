package bridge

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
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
)

// EventHandler handles the dispatch and processing of all WhatsApp events
// received from the backend.
type EventHandler struct {
	Backend  *backend.Backend
	App      *ui.App
	DB       *database.AppDB
	Messages *MessageService
	Chat     *ChatController
	Contacts *ContactService
	Media    *MediaService
	Pipeline *core.MessagePipeline
	ctx      context.Context

	syncMutex           sync.RWMutex
	isSyncing           bool
	isOfflineSyncing    bool
	offlineSyncTotal    int
	offlineSyncReceived int
}

// NewEventHandler creates a new EventHandler.
func NewEventHandler(b *backend.Backend, app *ui.App, db *database.AppDB, msgs *MessageService, chat *ChatController, contacts *ContactService, media *MediaService, pipeline *core.MessagePipeline, ctx context.Context) *EventHandler {
	return &EventHandler{
		Backend:  b,
		App:      app,
		DB:       db,
		Messages: msgs,
		Chat:     chat,
		Contacts: contacts,
		Media:    media,
		Pipeline: pipeline,
		ctx:      ctx,
	}
}

// IsSyncing returns whether a history sync is currently in progress.
func (eh *EventHandler) IsSyncing() bool {
	eh.syncMutex.RLock()
	defer eh.syncMutex.RUnlock()
	return eh.isSyncing
}

// HandleEvent is the main event dispatcher. It type-switches on AppEvent
// and routes to the appropriate handler.
func (eh *EventHandler) HandleEvent(evt backend.AppEvent) {
	switch v := evt.(type) {
	case *backend.HistorySyncEvent:
		eh.handleHistorySync(v)
	case *backend.MessageEvent:
		eh.handleMessage(v)
	case *backend.ConnectedEvent:
		eh.handleConnected()
	case *backend.DisconnectedEvent:
		eh.handleDisconnected()
	case *backend.QREvent:
		eh.handleQR(v)
	case *backend.OfflineSyncCompletedEvent:
		eh.handleOfflineSyncCompleted()
	case *backend.OfflineSyncPreviewEvent:
		eh.handleOfflineSyncPreview(v)
	case *backend.ReceiptEvent:
		eh.handleReceipt(v)
	case *backend.ContactEvent:
		eh.handleContact(v)
	case *backend.PushNameEvent:
		eh.handlePushName(v)
	case *backend.MediaRetryEvent:
		eh.handleMediaRetry(v)
	case *backend.UndecryptableEvent:
		eh.handleUndecryptable(v)
	}
}

// handleHistorySync processes history sync events: parses web messages,
// persists them, handles reactions, and refreshes the sidebar.
func (eh *EventHandler) handleHistorySync(v *backend.HistorySyncEvent) {
	eh.syncMutex.Lock()
	eh.isSyncing = true
	eh.syncMutex.Unlock()
	
	progress := v.Data.Data.GetProgress()
	fraction := float64(progress) / 100.0
	glib.IdleAdd(func() { 
		eh.App.Sidebar.ShowSyncing(true) 
		eh.App.Sidebar.SetSyncProgress(fraction)
	})
	go func() {
		tx, err := eh.DB.Begin()
		if err != nil {
			fmt.Printf("Bridge: Failed to begin transaction for history sync: %v\n", err)
		}
		commitCount := 0

		commitTx := func() {
			if tx != nil {
				_ = tx.Commit()
				tx, _ = eh.DB.Begin()
				commitCount = 0
			}
		}

		defer func() {
			if tx != nil {
				_ = tx.Commit()
			}
		}()

		for _, conv := range v.Data.Data.GetConversations() {
			chatJID, _ := types.ParseJID(conv.GetID()); chatJID = chatJID.ToNonAD()
			contact := database.Contact{JID: chatJID.String(), IsGroup: sql.NullBool{Bool: chatJID.Server == types.GroupServer, Valid: true}}
			if tx != nil {
				_ = eh.DB.SaveContactTx(tx, contact)
			} else {
				_ = eh.DB.SaveContact(contact)
			}
			
			// Save sync metadata
			unreadCount := int(conv.GetUnreadCount())
			isPinned := conv.GetPinned() > 0
			isArchived := conv.GetArchived()
			timestamp := conv.GetConversationTimestamp()
			if tx != nil {
				_ = eh.DB.SaveSyncDataTx(tx, chatJID.String(), unreadCount, isPinned, isArchived, conv.GetName(), "", timestamp)
			} else {
				_ = eh.DB.SaveSyncData(chatJID.String(), unreadCount, isPinned, isArchived, conv.GetName(), "", timestamp)
			}
			
			for _, hMsg := range conv.GetMessages() {
				pMsg, err := eh.Backend.Client.ParseWebMessage(chatJID, hMsg.GetMessage())
				if err == nil {
					eh.Messages.PersistMessageTx(tx, pMsg)
					if pMsg.Message.GetReactionMessage() != nil {
						eh.Messages.HandleReaction(pMsg.Info.Chat, pMsg.Info.Sender, pMsg.Message.GetReactionMessage().GetText(), pMsg.Message.GetReactionMessage().GetKey().GetID(), pMsg.Info.Timestamp)
					}
					commitCount++
					if commitCount >= 250 {
						commitTx()
					}
				}
			}
		}

		if tx != nil {
			_ = tx.Commit()
			tx = nil
		}

		eh.syncMutex.Lock()
		eh.isSyncing = false
		eh.syncMutex.Unlock()
		
		glib.IdleAdd(func() { eh.App.Sidebar.ShowSyncing(false) })
		eh.Chat.RefreshSidebarUI()
	}()
}

// handleMessage handles incoming messages: detects reactions, processes pins,
// persists the message, and runs pipeline hooks.
func (eh *EventHandler) handleMessage(v *backend.MessageEvent) {
	eh.incrementOfflineSync()
	msg := v.Info
	if react := msg.Message.GetReactionMessage(); react != nil {
		eh.Messages.HandleReaction(msg.Info.Chat, msg.Info.Sender, react.GetText(), react.GetKey().GetID(), msg.Info.Timestamp)
		return
	}
	if protoMsg := msg.Message.GetProtocolMessage(); protoMsg != nil && protoMsg.GetType() == waProto.ProtocolMessage_MESSAGE_EDIT {
		eh.Messages.HandleEdit(protoMsg, msg.Info.Chat)
		return
	}

	if pollUpdate := msg.Message.GetPollUpdateMessage(); pollUpdate != nil {
		pollVote, err := eh.Backend.Client.DecryptPollVote(eh.ctx, msg)
		if err == nil {
			var selectedHashes []string
			for _, hash := range pollVote.GetSelectedOptions() {
				selectedHashes = append(selectedHashes, hex.EncodeToString(hash))
			}
			msgKey := pollUpdate.GetPollCreationMessageKey()
			if msgKey != nil && msgKey.GetID() != "" {
				voterJID := eh.Messages.ResolveJID(msg.Info.Sender).ToNonAD().String()
				eh.DB.UpdatePollVote(msgKey.GetID(), voterJID, selectedHashes)
				
				votes, _ := eh.DB.GetPollVotes(msgKey.GetID())
				chatJIDStr := eh.Messages.ResolveJID(msg.Info.Chat).ToNonAD().String()
				myJID := eh.Backend.Client.Store.ID.ToNonAD().String()
				glib.IdleAdd(func() {
					cv := eh.App.GetChatViewForJID(chatJIDStr)
					if cv != nil {
						cv.UpdatePollVotes(msgKey.GetID(), votes, myJID)
					}
				})
			}
		}
		return
	}

	if eh.Messages.ProcessPin(msg.Message, msg.Info.Chat) {
		return
	}

	eh.Messages.PersistMessage(msg)
	
	// Process message through hooks (Rendering, auto-download, etc.)
	eh.Pipeline.Process(msg)
}

// handleConnected hides the QR dialog, syncs contacts, clears status banner, and refreshes the sidebar.
func (eh *EventHandler) handleConnected() {
	glib.IdleAdd(func() {
		eh.App.HideQRCode()
		if eh.App.ChatView != nil {
			eh.App.ChatView.SetConnectionStatus("", false)
		}
	})
	go func() {
		eh.Contacts.Sync(eh.ctx)
		eh.Chat.RefreshSidebarUI()
	}()
}

// handleDisconnected displays an offline banner and launches an automatic reconnection loop.
func (eh *EventHandler) handleDisconnected() {
	glib.IdleAdd(func() {
		if eh.App.ChatView != nil {
			eh.App.ChatView.SetConnectionStatus("Waiting for network / disconnected...", true)
		}
	})
	go eh.startReconnectLoop()
}

func (eh *EventHandler) startReconnectLoop() {
	backoffs := []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 20 * time.Second, 30 * time.Second}
	for _, delay := range backoffs {
		time.Sleep(delay)
		if eh.Backend.Client.IsConnected() {
			glib.IdleAdd(func() {
				if eh.App.ChatView != nil {
					eh.App.ChatView.SetConnectionStatus("", false)
				}
			})
			return
		}
		glib.IdleAdd(func() {
			if eh.App.ChatView != nil {
				eh.App.ChatView.SetConnectionStatus("Connecting to WhatsApp...", false)
			}
		})
		err := eh.Backend.Connect()
		if err == nil {
			return
		}
	}
	glib.IdleAdd(func() {
		if eh.App.ChatView != nil {
			eh.App.ChatView.SetConnectionStatus("Offline - Could not reconnect automatically", true)
		}
	})
}

// handleQR generates a QR code and displays it in the UI.
func (eh *EventHandler) handleQR(v *backend.QREvent) {
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
		eh.App.ShowQRCode(tex)
	})
}

// handleOfflineSyncCompleted clears the syncing flag and refreshes the sidebar.
func (eh *EventHandler) handleOfflineSyncCompleted() {
	eh.syncMutex.Lock()
	eh.isSyncing = false
	eh.isOfflineSyncing = false
	eh.syncMutex.Unlock()
	
	glib.IdleAdd(func() { eh.App.Sidebar.ShowSyncing(false) })
	go func() {
		eh.Chat.RefreshSidebarUI()
	}()
}

// handleOfflineSyncPreview sets the syncing flag and shows the syncing bar.
func (eh *EventHandler) handleOfflineSyncPreview(v *backend.OfflineSyncPreviewEvent) {
	eh.syncMutex.Lock()
	eh.isSyncing = true
	eh.isOfflineSyncing = true
	eh.offlineSyncTotal = v.Info.Messages + v.Info.Receipts
	if eh.offlineSyncTotal == 0 {
		eh.offlineSyncTotal = v.Info.Total
	}
	eh.offlineSyncReceived = 0
	eh.syncMutex.Unlock()

	glib.IdleAdd(func() { 
		eh.App.Sidebar.ShowSyncing(true)
		eh.App.Sidebar.SetSyncProgress(0.0)
	})
}

func (eh *EventHandler) incrementOfflineSync() {
	eh.syncMutex.Lock()
	defer eh.syncMutex.Unlock()
	
	if eh.isOfflineSyncing {
		eh.offlineSyncReceived++
		if eh.offlineSyncTotal > 0 {
			fraction := float64(eh.offlineSyncReceived) / float64(eh.offlineSyncTotal)
			if fraction > 1.0 { fraction = 1.0 }
			glib.IdleAdd(func() { eh.App.Sidebar.SetSyncProgress(fraction) })
		}
	}
}

// handleReceipt maps receipt types to status strings and updates DB and UI.
func (eh *EventHandler) handleReceipt(v *backend.ReceiptEvent) {
	eh.incrementOfflineSync()

	chatJID := eh.Messages.ResolveJID(v.Info.Chat).ToNonAD()
	chatJIDStr := chatJID.String()

	var rType string
	switch v.Info.Type {
	case types.ReceiptTypeRead, types.ReceiptTypeReadSelf, types.ReceiptTypePlayed, types.ReceiptTypePlayedSelf:
		rType = "read"
	case types.ReceiptTypeDelivered, types.ReceiptTypeSender:
		rType = "delivered"
	default:
		rType = "delivered"
	}

	senderJID := eh.Messages.ResolveJID(v.Info.Sender).ToNonAD().String()
	if senderJID == "" {
		senderJID = eh.Messages.ResolveJID(v.Info.MessageSender).ToNonAD().String()
	}
	if senderJID == "" {
		senderJID = chatJIDStr
	}

	isGroup := chatJID.Server == types.GroupServer

	var myJIDStr string
	if eh.Backend != nil && eh.Backend.Client != nil && eh.Backend.Client.Store != nil && eh.Backend.Client.Store.ID != nil {
		myJIDStr = eh.Backend.Client.Store.ID.ToNonAD().String()
	}

	for _, id := range v.Info.MessageIDs {
		if err := eh.DB.SaveReceipt(id, chatJIDStr, senderJID, rType, v.Info.Timestamp); err != nil {
			fmt.Printf("Bridge: Error saving receipt for %s: %v\n", id, err)
		}

		var newStatus string
		if isGroup {
			totalParticipants := eh.Chat.GetGroupParticipantCount(chatJID)
			
			msgSenderJID := myJIDStr
			if msg, err := eh.DB.GetMessage(id); err == nil && msg != nil && msg.SenderJID != "" {
				msgSenderJID = msg.SenderJID
			}

			readCount, deliveredCount, err := eh.DB.GetGroupReceiptCounts(id, msgSenderJID)
			if err != nil {
				fmt.Printf("Bridge: Error getting group receipt counts for %s: %v\n", id, err)
			}

			reqReadCount := 0
			if totalParticipants > 1 {
				reqReadCount = totalParticipants - 1
			}

			if reqReadCount > 0 && readCount >= reqReadCount {
				newStatus = "read"
			} else if readCount > 0 || deliveredCount > 0 {
				newStatus = "delivered"
			} else {
				newStatus = "sent"
			}
		} else {
			newStatus = rType
		}

		err := eh.DB.UpdateMessageStatus(id, chatJIDStr, newStatus)
		if err != nil {
			fmt.Printf("Bridge: Error updating receipt status for %s: %v\n", id, err)
		}

		selectedJID := eh.Chat.SelectedJID()
		if selectedJID != nil && selectedJID.ToNonAD().String() == chatJIDStr {
			st := newStatus
			msgID := id
			glib.IdleAdd(func() {
				eh.App.ChatView.UpdateMessageStatus(msgID, st)
			})
		}
	}
}

func (eh *EventHandler) handleContact(v *backend.ContactEvent) {
	if v.Info.Action != nil {
		jid := v.Info.JID.ToNonAD().String(); pnJID := v.Info.Action.GetPnJID()
		
		savedName := sql.NullString{String: v.Info.Action.GetFullName(), Valid: v.Info.Action.GetFullName() != ""}
		pushName := sql.NullString{String: v.Info.Action.GetFirstName(), Valid: v.Info.Action.GetFirstName() != ""}
		
		if pnJID != "" && strings.HasSuffix(jid, "@lid") {
			pn := pnJID + "@s.whatsapp.net"
			eh.DB.SaveContact(database.Contact{JID: pn, LID: sql.NullString{String: jid, Valid: true}, SavedName: savedName, PushName: pushName})
			eh.DB.MergeLID(pn, jid)
		} else {
			eh.DB.SaveContact(database.Contact{JID: jid, SavedName: savedName, PushName: pushName})
		}
	}
}

// handlePushName saves push name updates to the database.
func (eh *EventHandler) handlePushName(v *backend.PushNameEvent) {
	eh.DB.SaveContact(database.Contact{JID: v.Info.JID.ToNonAD().String(), PushName: sql.NullString{String: v.Info.NewPushName, Valid: v.Info.NewPushName != ""}})
}

// handleMediaRetry decrypts a media retry notification, updates the DB with
// the new direct path, and re-triggers download.
func (eh *EventHandler) handleMediaRetry(v *backend.MediaRetryEvent) {
	fmt.Printf("Bridge: Received Media Retry for %s\n", v.Info.MessageID)
	
	msg, err := eh.DB.GetMessage(v.Info.MessageID)
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
	
	eh.DB.SaveMessage(*msg)

	// Re-trigger download
	eh.Chat.HandleDownloadMedia(v.Info.MessageID)
}

func protoStr(s string) *string { return &s }

func (eh *EventHandler) handleUndecryptable(v *backend.UndecryptableEvent) {
	if v.Info.IsUnavailable && v.Info.UnavailableType == "view_once" {
		fakeMsg := &events.Message{
			Info: v.Info.Info,
			Message: &waProto.Message{
				ViewOnceMessage: &waProto.FutureProofMessage{
					Message: &waProto.Message{
						Conversation: protoStr("[Mídia]"),
					},
				},
			},
		}
		eh.handleMessage(&backend.MessageEvent{Info: fakeMsg})
	}
}
