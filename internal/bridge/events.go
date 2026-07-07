package bridge

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
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
	Renderer *Renderer
	Pipeline *core.MessagePipeline
	ctx      context.Context

	isSyncing           bool
	isOfflineSyncing    bool
	offlineSyncTotal    int
	offlineSyncReceived int
}

// NewEventHandler creates a new EventHandler.
func NewEventHandler(b *backend.Backend, app *ui.App, db *database.AppDB, msgs *MessageService, chat *ChatController, contacts *ContactService, media *MediaService, rend *Renderer, pipeline *core.MessagePipeline, ctx context.Context) *EventHandler {
	return &EventHandler{
		Backend:  b,
		App:      app,
		DB:       db,
		Messages: msgs,
		Chat:     chat,
		Contacts: contacts,
		Media:    media,
		Renderer: rend,
		Pipeline: pipeline,
		ctx:      ctx,
	}
}

// IsSyncing returns whether a history sync is currently in progress.
func (eh *EventHandler) IsSyncing() bool { return eh.isSyncing }

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
	eh.isSyncing = true
	progress := v.Data.Data.GetProgress()
	fraction := float64(progress) / 100.0
	glib.IdleAdd(func() { 
		eh.App.Sidebar.ShowSyncing(true) 
		eh.App.Sidebar.SetSyncProgress(fraction)
	})
	go func() {
		for _, conv := range v.Data.Data.GetConversations() {
			chatJID, _ := types.ParseJID(conv.GetID()); chatJID = chatJID.ToNonAD()
			eh.DB.SaveContact(database.Contact{JID: chatJID.String(), IsGroup: sql.NullBool{Bool: chatJID.Server == types.GroupServer, Valid: true}})
			for _, hMsg := range conv.GetMessages() {
				pMsg, err := eh.Backend.Client.ParseWebMessage(chatJID, hMsg.GetMessage())
				if err == nil {
					eh.Messages.PersistMessage(pMsg)
					if pMsg.Message.GetReactionMessage() != nil {
						eh.Messages.HandleReaction(pMsg.Info.Chat, pMsg.Info.Sender, pMsg.Message.GetReactionMessage().GetText(), pMsg.Message.GetReactionMessage().GetKey().GetID(), pMsg.Info.Timestamp)
					}
				}
			}
		}
		eh.isSyncing = false
		glib.IdleAdd(func() { eh.App.Sidebar.ShowSyncing(false) })
		c, _ := eh.DB.GetAllContacts(100); eh.Renderer.RefreshSidebar(c)
	}()
}

// handleMessage handles incoming messages: detects reactions, processes pins,
// persists the message, and runs pipeline hooks.
func (eh *EventHandler) handleMessage(v *backend.MessageEvent) {
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
				eh.DB.UpdatePollVote(msgKey.GetID(), msg.Info.Sender.ToNonAD().String(), selectedHashes)
				
				votes, _ := eh.DB.GetPollVotes(msgKey.GetID())
				myJID := eh.Backend.Client.Store.ID.ToNonAD().String()
				glib.IdleAdd(func() {
					cv := eh.App.GetChatViewForJID(msg.Info.Chat.ToNonAD().String())
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

	if eh.isOfflineSyncing {
		eh.offlineSyncReceived++
		if eh.offlineSyncTotal > 0 {
			fraction := float64(eh.offlineSyncReceived) / float64(eh.offlineSyncTotal)
			if fraction > 1.0 { fraction = 1.0 }
			glib.IdleAdd(func() { eh.App.Sidebar.SetSyncProgress(fraction) })
		}
	}
}

// handleConnected hides the QR dialog, syncs contacts, and refreshes the sidebar.
func (eh *EventHandler) handleConnected() {
	glib.IdleAdd(func() {
		eh.App.HideQRCode()
	})
	go func() {
		eh.Contacts.Sync(eh.ctx)
		c, _ := eh.DB.GetAllContacts(200)
		eh.Renderer.RefreshSidebar(c)
	}()
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
	eh.isSyncing = false
	eh.isOfflineSyncing = false
	glib.IdleAdd(func() { eh.App.Sidebar.ShowSyncing(false) })
	go func() {
		c, _ := eh.DB.GetAllContacts(100)
		eh.Renderer.RefreshSidebar(c)
	}()
}

// handleOfflineSyncPreview sets the syncing flag and shows the syncing bar.
func (eh *EventHandler) handleOfflineSyncPreview(v *backend.OfflineSyncPreviewEvent) {
	eh.isSyncing = true
	eh.isOfflineSyncing = true
	eh.offlineSyncTotal = v.Info.Messages
	eh.offlineSyncReceived = 0
	glib.IdleAdd(func() { 
		eh.App.Sidebar.ShowSyncing(true)
		eh.App.Sidebar.SetSyncProgress(0.0)
	})
}

// handleReceipt maps receipt types to status strings and updates DB and UI.
func (eh *EventHandler) handleReceipt(v *backend.ReceiptEvent) {
	chatJID := eh.Messages.ResolveJID(v.Info.Chat).String(); status := "sent"
	if v.Info.Type == types.ReceiptTypeDelivered { status = "delivered" }
	if v.Info.Type == types.ReceiptTypeRead || v.Info.Type == types.ReceiptTypeReadSelf { status = "read" }
	for _, id := range v.Info.MessageIDs {
		err := eh.DB.UpdateMessageStatus(id, chatJID, status)
		if err != nil {
			fmt.Printf("Bridge: Error updating receipt status for %s: %v\n", id, err)
		}
		selectedJID := eh.Chat.SelectedJID()
		if selectedJID != nil && selectedJID.ToNonAD().String() == chatJID {
			glib.IdleAdd(func() {
				eh.App.ChatView.UpdateMessageStatus(id, status)
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
