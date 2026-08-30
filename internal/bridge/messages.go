package bridge

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"
	"whats-gtk/internal/backend"
	"whats-gtk/internal/database"
	"whats-gtk/internal/paths"
	"whats-gtk/internal/ui"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// MessageService handles message persistence, content extraction from protobuf,
// reaction handling, pin processing, and JID resolution.
type MessageService struct {
	DB       *database.AppDB
	Backend  *backend.Backend
	Contacts *ContactService
	App      *ui.App
	ctx      context.Context

	// GetSelectedJID is set after ChatController is created to avoid circular init.
	GetSelectedJID func() *types.JID
}

// NewMessageService creates a new MessageService.
func NewMessageService(db *database.AppDB, b *backend.Backend, contacts *ContactService, app *ui.App, ctx context.Context) *MessageService {
	return &MessageService{
		DB:       db,
		Backend:  b,
		Contacts: contacts,
		App:      app,
		ctx:      ctx,
	}
}

// ResolveJID resolves LID JIDs to phone number JIDs via whatsmeow store and DB lookup.
func (ms *MessageService) ResolveJID(jid types.JID) types.JID {
	jStr := jid.ToNonAD().String()
	if strings.HasSuffix(jStr, "@lid") {
		if ms.Backend != nil && ms.Backend.Client != nil && ms.Backend.Client.Store != nil && ms.Backend.Client.Store.LIDs != nil {
			if pn, err := ms.Backend.Client.Store.LIDs.GetPNForLID(ms.ctx, jid.ToNonAD()); err == nil && !pn.IsEmpty() {
				return pn.ToNonAD()
			}
		}
		if c, err := ms.DB.GetContact(jStr); err == nil && !strings.HasSuffix(c.JID, "@lid") {
			if parsed, err := types.ParseJID(c.JID); err == nil {
				return parsed.ToNonAD()
			}
		}
		if target, err := ms.DB.GetContactByLID(jStr); err == nil && !strings.HasSuffix(target.JID, "@lid") {
			if parsed, err := types.ParseJID(target.JID); err == nil {
				return parsed.ToNonAD()
			}
		}
	}
	return jid.ToNonAD()
}

// ResolveJIDString resolves a JID string to its canonical PN representation if possible.
func (ms *MessageService) ResolveJIDString(jStr string) string {
	if jStr == "" {
		return ""
	}
	parsed, err := types.ParseJID(jStr)
	if err != nil {
		return jStr
	}
	return ms.ResolveJID(parsed).String()
}

// UnwrapMessage removes ViewOnce wrappers and returns the underlying message and a boolean indicating if it was ViewOnce.
func (ms *MessageService) UnwrapMessage(msg *waProto.Message) (*waProto.Message, bool) {
	if msg == nil {
		return nil, false
	}
	if vo := msg.GetViewOnceMessage(); vo != nil && vo.GetMessage() != nil {
		return vo.GetMessage(), true
	}
	if vo2 := msg.GetViewOnceMessageV2(); vo2 != nil && vo2.GetMessage() != nil {
		return vo2.GetMessage(), true
	}
	if vo2e := msg.GetViewOnceMessageV2Extension(); vo2e != nil && vo2e.GetMessage() != nil {
		return vo2e.GetMessage(), true
	}
	return msg, false
}

// ExtractContentFromProto extracts the text content from a waProto.Message
func (ms *MessageService) ExtractContentFromProto(protoMsg *waProto.Message) string {
	protoMsg, _ = ms.UnwrapMessage(protoMsg)
	if protoMsg.GetConversation() != "" {
		return protoMsg.GetConversation()
	}
	if protoMsg.GetExtendedTextMessage().GetText() != "" {
		return protoMsg.GetExtendedTextMessage().GetText()
	}
	if protoMsg.GetImageMessage().GetCaption() != "" {
		return protoMsg.GetImageMessage().GetCaption()
	}
	if protoMsg.GetVideoMessage().GetCaption() != "" {
		return protoMsg.GetVideoMessage().GetCaption()
	}
	if protoMsg.GetDocumentMessage().GetCaption() != "" {
		return protoMsg.GetDocumentMessage().GetCaption()
	}

	if protoMsg.GetAudioMessage() != nil {
		return "[Audio Message]"
	}
	if protoMsg.GetDocumentMessage() != nil {
		return fmt.Sprintf("[Document: %s]", protoMsg.GetDocumentMessage().GetFileName())
	}
	if pm := ms.GetPollCreationMessage(protoMsg); pm != nil {
		return fmt.Sprintf("[Poll: %s]", pm.GetName())
	}
	if protoMsg.GetContactMessage() != nil {
		return fmt.Sprintf("[Contact: %s]", protoMsg.GetContactMessage().GetDisplayName())
	}
	if protoMsg.GetLocationMessage() != nil {
		return "[Location]"
	}

	return ""
}

// GetPollCreationMessage extracts a PollCreationMessage regardless of its version.
func (ms *MessageService) GetPollCreationMessage(msg *waProto.Message) *waProto.PollCreationMessage {
	if msg == nil {
		return nil
	}
	if pm := msg.GetPollCreationMessage(); pm != nil {
		return pm
	}
	if pm := msg.GetPollCreationMessageV2(); pm != nil {
		return pm
	}
	if pm := msg.GetPollCreationMessageV3(); pm != nil {
		return pm
	}
	if pm := msg.GetPollCreationMessageV5(); pm != nil {
		return pm
	}
	if pm := msg.GetPollCreationMessageV6(); pm != nil {
		return pm
	}
	return nil
}

// ExtractContent extracts the text content from various WhatsApp message types.
func (ms *MessageService) ExtractContent(msg *events.Message) string {
	return ms.ExtractContentFromProto(msg.Message)
}

// ExtractContextInfo extracts the ContextInfo (quoted message info) from various message types.
func (ms *MessageService) ExtractContextInfo(msg *events.Message) *waProto.ContextInfo {
	protoMsg, _ := ms.UnwrapMessage(msg.Message)
	if etm := protoMsg.GetExtendedTextMessage(); etm != nil {
		return etm.GetContextInfo()
	}
	if img := protoMsg.GetImageMessage(); img != nil {
		return img.GetContextInfo()
	}
	if vid := protoMsg.GetVideoMessage(); vid != nil {
		return vid.GetContextInfo()
	}
	if aud := protoMsg.GetAudioMessage(); aud != nil {
		return aud.GetContextInfo()
	}
	if doc := protoMsg.GetDocumentMessage(); doc != nil {
		return doc.GetContextInfo()
	}
	if stkr := protoMsg.GetStickerMessage(); stkr != nil {
		return stkr.GetContextInfo()
	}
	return nil
}

// PersistMessage extracts message type, content, and media metadata from protobuf
// and saves it to the database.
func (ms *MessageService) PersistMessage(msg *events.Message) {
	ms.PersistMessageTx(nil, msg)
}

// PersistMessageTx extracts message type, content, and media metadata from protobuf
// and saves it to the database, optionally within an active SQL transaction.
func (ms *MessageService) PersistMessageTx(tx *sql.Tx, msg *events.Message) {
	if msg.Message.GetReactionMessage() != nil || 
	   msg.Message.GetProtocolMessage() != nil ||
	   msg.Message.GetSenderKeyDistributionMessage() != nil {
		return
	}

	protoMsg, isViewOnce := ms.UnwrapMessage(msg.Message)

	msgType := "text"; content := ms.ExtractContent(msg); var thumb []byte
	var metadata database.Message
	metadata.IsViewOnce = isViewOnce

	if img := protoMsg.GetImageMessage(); img != nil {
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
	} else if stkr := protoMsg.GetStickerMessage(); stkr != nil {
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
	} else if vid := protoMsg.GetVideoMessage(); vid != nil {
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
	} else if doc := protoMsg.GetDocumentMessage(); doc != nil {
		msgType = "document"; thumb = doc.GetJPEGThumbnail()
		metadata.MediaURL = sql.NullString{String: doc.GetURL(), Valid: doc.GetURL() != ""}
		metadata.MediaDirectPath = sql.NullString{String: doc.GetDirectPath(), Valid: doc.GetDirectPath() != ""}
		metadata.MediaKey = doc.GetMediaKey()
		metadata.MediaMimetype = sql.NullString{String: doc.GetMimetype(), Valid: doc.GetMimetype() != ""}
		metadata.MediaEncSHA256 = doc.GetFileEncSHA256()
		metadata.MediaSHA256 = doc.GetFileSHA256()
		metadata.MediaLength = sql.NullInt64{Int64: int64(doc.GetFileLength()), Valid: true}
	} else if aud := protoMsg.GetAudioMessage(); aud != nil {
		msgType = "audio"
		metadata.MediaURL = sql.NullString{String: aud.GetURL(), Valid: aud.GetURL() != ""}
		metadata.MediaDirectPath = sql.NullString{String: aud.GetDirectPath(), Valid: aud.GetDirectPath() != ""}
		metadata.MediaKey = aud.GetMediaKey()
		metadata.MediaMimetype = sql.NullString{String: aud.GetMimetype(), Valid: aud.GetMimetype() != ""}
		metadata.MediaEncSHA256 = aud.GetFileEncSHA256()
		metadata.MediaSHA256 = aud.GetFileSHA256()
		metadata.MediaLength = sql.NullInt64{Int64: int64(aud.GetFileLength()), Valid: true}
	} else if poll := ms.GetPollCreationMessage(protoMsg); poll != nil {
		msgType = "poll"
		for _, opt := range poll.GetOptions() {
			ms.DB.SavePollOption(msg.Info.ID, opt.GetOptionName())
		}
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
		path := paths.MediaPath(msg.Info.ID + ext)
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

	chatJID := ms.ResolveJID(msg.Info.Chat).String()
	senderJID := ms.ResolveJID(msg.Info.Sender).String()
	
	if tx == nil && strings.HasSuffix(msg.Info.Sender.String(), "@lid") {
		go ms.Contacts.ResolveLIDMapping(msg.Info.Sender.String())
	}
	
	metadata.ID = msg.Info.ID; metadata.ChatJID = chatJID; metadata.SenderJID = senderJID
	metadata.Content = content; metadata.Type = msgType; metadata.Timestamp = msg.Info.Timestamp
	metadata.IsFromMe = msg.Info.IsFromMe; metadata.Thumbnail = thumb
	if metadata.IsFromMe && metadata.Status == "" {
		metadata.Status = "sent"
	}

	// Extract ContextInfo
	if ci := ms.ExtractContextInfo(msg); ci != nil {
		metadata.IsForwarded = ci.GetIsForwarded()
		if ci.GetStanzaID() != "" {
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
	}
	var err error
	if tx != nil {
		err = ms.DB.SaveMessageTx(tx, metadata)
		_ = ms.DB.UpdateContactTimestampTx(tx, chatJID, msg.Info.Timestamp)
	} else {
		err = ms.DB.SaveMessage(metadata)
		_ = ms.DB.UpdateContactTimestamp(chatJID, msg.Info.Timestamp)
	}
	if err != nil {
		fmt.Printf("Bridge: Error saving message %s: %v\n", metadata.ID, err)
	}
	
	if !msg.Info.IsFromMe {
		pushName := msg.Info.PushName; fullName := ""
		contactInfo, err := ms.Backend.Client.Store.Contacts.GetContact(ms.ctx, msg.Info.Sender)
		if err == nil && contactInfo.Found { if pushName == "" { pushName = contactInfo.PushName }; fullName = contactInfo.FullName }
		if pushName != "" || fullName != "" {
			c := database.Contact{JID: senderJID, SavedName: sql.NullString{String: fullName, Valid: fullName != ""}, PushName: sql.NullString{String: pushName, Valid: pushName != ""}}
			if tx != nil {
				_ = ms.DB.SaveContactTx(tx, c)
			} else {
				_ = ms.DB.SaveContact(c)
			}
		}
	}
}

// PersistMediaMessage is a simplified persistence for pre-downloaded media messages.
func (ms *MessageService) PersistMediaMessage(msg *events.Message, msgType string, path string) {
	protoMsg, isViewOnce := ms.UnwrapMessage(msg.Message)
	chatJID := msg.Info.Chat.ToNonAD().String(); senderJID := msg.Info.Sender.ToNonAD().String()
	var thumb []byte
	var width, height int64
	if img := protoMsg.GetImageMessage(); img != nil { 
		thumb = img.GetJPEGThumbnail()
		width = int64(img.GetWidth())
		height = int64(img.GetHeight())
	} else if stkr := protoMsg.GetStickerMessage(); stkr != nil { 
		thumb = stkr.GetPngThumbnail() 
		width = int64(stkr.GetWidth())
		height = int64(stkr.GetHeight())
	}
	status := ""
	if msg.Info.IsFromMe {
		status = "sent"
	}
	ms.DB.SaveMessage(database.Message{
		ID: msg.Info.ID, ChatJID: chatJID, SenderJID: senderJID, Content: path, Type: msgType, 
		Timestamp: msg.Info.Timestamp, Status: status, IsFromMe: msg.Info.IsFromMe, Thumbnail: thumb,
		MediaWidth: sql.NullInt64{Int64: width, Valid: width > 0},
		MediaHeight: sql.NullInt64{Int64: height, Valid: height > 0},
		IsViewOnce: isViewOnce,
	})
	ms.DB.UpdateContactTimestamp(chatJID, msg.Info.Timestamp)
}

// HandleReaction saves a reaction to the database and updates the UI if the chat is selected.
func (ms *MessageService) HandleReaction(chat, sender types.JID, text, targetID string, timestamp time.Time) {
	ms.HandleReactionTx(nil, chat, sender, text, targetID, timestamp)
}

func (ms *MessageService) HandleReactionTx(tx *sql.Tx, chat, sender types.JID, text, targetID string, timestamp time.Time) {
	react := database.Reaction{
		MessageID: targetID,
		SenderJID: sender.ToNonAD().String(),
		Reaction:  text,
		Timestamp: timestamp,
	}
	if tx != nil {
		_ = ms.DB.SaveReactionTx(tx, react)
	} else {
		_ = ms.DB.SaveReaction(react)
	}
	
	selectedJID := ms.GetSelectedJID()
	if selectedJID != nil && chat.ToNonAD().String() == selectedJID.ToNonAD().String() {
		reactions, _ := ms.DB.GetReactions(targetID)
		ms.App.ChatView.UpdateMessageReactions(targetID, uniqueReactions(reactions))
	}
}

// ProcessPin recursively processes pin/unpin protocol messages.
// Returns true if a pin message was processed.
func (ms *MessageService) ProcessPin(msg *waProto.Message, chat types.JID) bool {
	if pinMsg := msg.GetPinInChatMessage(); pinMsg != nil {
		targetID := pinMsg.GetKey().GetID()
		isPinned := pinMsg.GetType() == waProto.PinInChatMessage_PIN_FOR_ALL
		chatJID := chat.ToNonAD().String()
		ms.DB.UpdateMessagePinned(targetID, chatJID, isPinned)
		
		selectedJID := ms.GetSelectedJID()
		if selectedJID != nil && chatJID == selectedJID.ToNonAD().String() {
			ms.App.ChatView.UpdateMessagePinned(targetID, isPinned)
			if isPinned {
				if m, err := ms.DB.GetMessage(targetID); err == nil {
					ms.App.ChatView.SetPinnedMessage(m.Content)
				}
			} else {
				ms.App.ChatView.SetPinnedMessage("")
			}
		}
		return true
	}

	if msg.GetProtocolMessage() != nil && msg.GetProtocolMessage().GetEditedMessage() != nil {
		return ms.ProcessPin(msg.GetProtocolMessage().GetEditedMessage(), chat)
	}

	return false
}

func (ms *MessageService) HandleEdit(protoMsg *waProto.ProtocolMessage, chat types.JID) {
	if protoMsg.GetEditedMessage() == nil || protoMsg.GetKey() == nil {
		return
	}
	targetID := protoMsg.GetKey().GetID()
	newContent := ms.ExtractContentFromProto(protoMsg.GetEditedMessage())
	if newContent == "" {
		return
	}
	
	chatJID := chat.ToNonAD().String()
	err := ms.DB.UpdateMessageContent(targetID, chatJID, newContent, true)
	if err != nil {
		fmt.Printf("Bridge: Failed to update edited message %s: %v\n", targetID, err)
		return
	}
	
	// Update UI
	chatView := ms.App.GetChatViewForJID(chatJID)
	if chatView != nil {
		chatView.UpdateMessageContent(targetID, newContent, true)
	}
}
