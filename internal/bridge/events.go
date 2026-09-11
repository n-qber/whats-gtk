package bridge

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"whats-gtk/internal/backend"
	"whats-gtk/internal/core"
	"whats-gtk/internal/database"
	"whats-gtk/internal/paths"
	"whats-gtk/internal/ui"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
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

	eventChan           chan backend.AppEvent
	syncMutex           sync.RWMutex
	syncWG              sync.WaitGroup
	isSyncing           bool
	isOfflineSyncing    bool
	offlineSyncTotal    int
	offlineSyncReceived int

	stickerReloadTimer glib.SourceHandle
	stickerReloadMutex sync.Mutex
}

// NewEventHandler creates a new EventHandler.
func NewEventHandler(b *backend.Backend, app *ui.App, db *database.AppDB, msgs *MessageService, chat *ChatController, contacts *ContactService, media *MediaService, pipeline *core.MessagePipeline, ctx context.Context) *EventHandler {
	eh := &EventHandler{
		Backend:   b,
		App:       app,
		DB:        db,
		Messages:  msgs,
		Chat:      chat,
		Contacts:  contacts,
		Media:     media,
		Pipeline:  pipeline,
		ctx:       ctx,
		eventChan: make(chan backend.AppEvent, 2000),
	}
	go eh.eventLoop()
	return eh
}

func (eh *EventHandler) eventLoop() {
	for evt := range eh.eventChan {
		eh.processEvent(evt)
	}
}

// WaitSync waits for all ongoing history sync operations to complete, up to timeout.
func (eh *EventHandler) WaitSync(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		eh.syncWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		fmt.Println("Bridge: Timed out waiting for sync to complete")
	}
}

func (eh *EventHandler) triggerStickerReload() {
	eh.stickerReloadMutex.Lock()
	defer eh.stickerReloadMutex.Unlock()

	if eh.stickerReloadTimer != 0 {
		glib.SourceRemove(eh.stickerReloadTimer)
	}
	eh.stickerReloadTimer = glib.TimeoutAdd(250, func() bool {
		eh.stickerReloadMutex.Lock()
		eh.stickerReloadTimer = 0
		eh.stickerReloadMutex.Unlock()

		if eh.App.ChatView != nil && eh.App.ChatView.InputBar != nil && eh.App.ChatView.InputBar.StickerPicker != nil {
			eh.App.ChatView.InputBar.StickerPicker.Reload()
		}
		return false
	})
}

// IsSyncing returns whether a history sync is currently in progress.
func (eh *EventHandler) IsSyncing() bool {
	eh.syncMutex.RLock()
	defer eh.syncMutex.RUnlock()
	return eh.isSyncing
}

// HandleEvent is the main event dispatcher. It enqueues events without blocking whatsmeow.
func (eh *EventHandler) HandleEvent(evt backend.AppEvent) {
	select {
	case eh.eventChan <- evt:
	default:
		go eh.processEvent(evt)
	}
}

func (eh *EventHandler) processEvent(evt backend.AppEvent) {
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
	case *backend.AppStateEvent:
		eh.handleAppState(v)
	}
}

// handleHistorySync processes history sync events: parses web messages,
// persists them, handles reactions, and refreshes the sidebar.
func (eh *EventHandler) handleHistorySync(v *backend.HistorySyncEvent) {
	if v == nil || v.Data == nil || v.Data.Data == nil {
		return
	}

	eh.syncMutex.Lock()
	eh.isSyncing = true
	eh.syncMutex.Unlock()
	
	progress := v.Data.Data.GetProgress()
	fraction := float64(progress) / 100.0
	glib.IdleAdd(func() { 
		eh.App.Sidebar.ShowSyncing(true) 
		eh.App.Sidebar.SetSyncProgress(fraction)
	})

	eh.syncWG.Add(1)
	go func() {
		defer eh.syncWG.Done()
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Bridge: Recovered from panic in history sync: %v\n", r)
			}
			eh.syncMutex.Lock()
			eh.isSyncing = false
			eh.syncMutex.Unlock()
			
			glib.IdleAdd(func() { eh.App.Sidebar.ShowSyncing(false) })
			eh.Chat.RefreshSidebarUI()
		}()

		tx, err := eh.DB.Begin()
		if err != nil {
			fmt.Printf("Bridge: Failed to begin transaction for history sync: %v\n", err)
		}
		commitCount := 0

		commitTx := func() {
			if tx != nil {
				if err := tx.Commit(); err != nil {
					fmt.Printf("Bridge: History sync commit error: %v\n", err)
				}
				var beginErr error
				tx, beginErr = eh.DB.Begin()
				if beginErr != nil {
					fmt.Printf("Bridge: Failed to begin next tx for history sync: %v\n", beginErr)
					tx = nil
				}
				commitCount = 0
			}
		}

		defer func() {
			if tx != nil {
				_ = tx.Commit()
				tx = nil
			}
		}()

		for _, conv := range v.Data.Data.GetConversations() {
			chatJID, _ := types.ParseJID(conv.GetID())
			chatJID = chatJID.ToNonAD()
			contact := database.Contact{JID: chatJID.String(), IsGroup: sql.NullBool{Bool: chatJID.Server == types.GroupServer, Valid: true}}
			_ = eh.DB.SaveContactTx(tx, contact)
			
			// Save sync metadata
			unreadCount := int(conv.GetUnreadCount())
			isPinned := conv.GetPinned() > 0
			isArchived := conv.GetArchived()
			timestamp := conv.GetConversationTimestamp()
			_ = eh.DB.SaveSyncDataTx(tx, chatJID.String(), unreadCount, isPinned, isArchived, conv.GetName(), "", timestamp)
			
			for _, hMsg := range conv.GetMessages() {
				if hMsg == nil || hMsg.GetMessage() == nil {
					continue
				}
				pMsg, err := eh.Backend.Client.ParseWebMessage(chatJID, hMsg.GetMessage())
				if err == nil && pMsg != nil {
					eh.Messages.PersistMessageTx(tx, pMsg)
					if pMsg.Message != nil && pMsg.Message.GetReactionMessage() != nil {
						eh.Messages.HandleReactionTx(tx, pMsg.Info.Chat, pMsg.Info.Sender, pMsg.Message.GetReactionMessage().GetText(), pMsg.Message.GetReactionMessage().GetKey().GetID(), pMsg.Info.Timestamp)
					}
					commitCount++
					if commitCount >= 100 {
						commitTx()
					}
				}
			}
		}

		if tx != nil {
			_ = tx.Commit()
			tx = nil
		}
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
		if eh.Backend != nil {
			_ = eh.Backend.FetchFavoriteStickers(eh.ctx)
		}
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

	var totalParticipants int
	if isGroup {
		totalParticipants = eh.Chat.GetGroupParticipantCount(chatJID)
	}

	for _, id := range v.Info.MessageIDs {
		if err := eh.DB.SaveReceipt(id, chatJIDStr, senderJID, rType, v.Info.Timestamp); err != nil {
			fmt.Printf("Bridge: Error saving receipt for %s: %v\n", id, err)
		}

		var newStatus string
		if isGroup {
			
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

		cv := eh.App.GetChatViewForJID(chatJIDStr)
		if cv != nil {
			st := newStatus
			msgID := id
			glib.IdleAdd(func() {
				cv.UpdateMessageStatus(msgID, st)
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

func (eh *EventHandler) handleAppState(v *backend.AppStateEvent) {
	if v.Info == nil {
		return
	}
	isFavStkr := len(v.Info.Index) > 0 && v.Info.Index[0] == "favoriteSticker"
	act := v.Info.GetStickerAction()
	if !isFavStkr && act == nil {
		return
	}

	if act == nil && isFavStkr && len(v.Info.Index) > 1 {
		_ = eh.DB.DeleteFavoriteSticker(v.Info.Index[1])
		eh.triggerStickerReload()
		return
	}

	if act != nil {
		id := ""
		if len(v.Info.Index) > 1 {
			id = v.Info.Index[1]
		}
		if len(act.GetFileEncSHA256()) > 0 {
			id = hex.EncodeToString(act.GetFileEncSHA256())
		}
		if id == "" {
			return
		}
		id = strings.Map(func(r rune) rune {
			if strings.ContainsRune("\\/:*?\"<>| ", r) {
				return '_'
			}
			return r
		}, id)

		isFav := true
		if act.IsFavorite != nil {
			isFav = act.GetIsFavorite()
		} else if !isFavStkr {
			isFav = false
		}

		if isFav {
			filePath := paths.MediaPath(id + ".webp")

			// Check if we already have this sticker downloaded in messages
			if _, err := os.Stat(filePath); err != nil && len(act.GetFileEncSHA256()) > 0 {
				if msgContent, err := eh.DB.FindExistingStickerPath(act.GetFileEncSHA256()); err == nil && msgContent != "" {
					if _, err := os.Stat(msgContent); err == nil {
						filePath = msgContent
					}
				}
			}

			item := database.StickerItem{
				ID:              id,
				FilePath:        filePath,
				MediaURL:        act.GetURL(),
				MediaDirectPath: act.GetDirectPath(),
				MediaKey:        act.GetMediaKey(),
				MediaEncSHA256:  act.GetFileEncSHA256(),
				Mimetype:        "image/webp",
				Width:           int(act.GetWidth()),
				Height:          int(act.GetHeight()),
				IsAnimated:      act.GetIsLottie(),
				CreatedAt:       time.Now(),
			}
			_ = eh.DB.SaveFavoriteSticker(item)

			if _, err := os.Stat(filePath); err != nil && len(act.GetMediaKey()) > 0 {
				if (eh.Media == nil || !eh.Media.IsFailed(id)) && (eh.DB == nil || !eh.DB.IsMediaFailed(id)) {
					eh.Media.Download(DownloadTask{
						ID:      id,
						MsgType: "sticker",
						Metadata: &MediaMetadata{
							URL:           act.GetURL(),
							DirectPath:    act.GetDirectPath(),
							MediaKey:      act.GetMediaKey(),
							Mimetype:      "image/webp",
							FileEncSHA256: act.GetFileEncSHA256(),
							FileLength:    act.GetFileLength(),
						},
					})
				}
			}
		} else {
			_ = eh.DB.DeleteFavoriteSticker(id)
			_ = eh.DB.DeleteFavoriteSticker(paths.MediaPath(id + ".webp"))
		}

		eh.triggerStickerReload()
	}
}
