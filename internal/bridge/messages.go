package bridge

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"whats-gtk/internal/backend"
	"whats-gtk/internal/database"
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

// ResolveJID resolves LID JIDs to phone number JIDs via DB lookup.
func (ms *MessageService) ResolveJID(jid types.JID) types.JID {
	jStr := jid.ToNonAD().String()
	if strings.HasSuffix(jStr, "@lid") {
		if c, err := ms.DB.GetContact(jStr); err == nil && c.LID.Valid && c.LID.String != "" {
			// This is actually tricky because MergeLID stores PN as JID and LID as LID.
			// Let's check if we have a contact where LID is this JID.
			if target, err := ms.DB.GetContactByLID(jStr); err == nil {
				if parsed, err := types.ParseJID(target.JID); err == nil {
					return parsed.ToNonAD()
				}
			}
		}
	}
	return jid.ToNonAD()
}

// ExtractContent extracts the text content from various WhatsApp message types.
func (ms *MessageService) ExtractContent(msg *events.Message) string {
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

// ExtractContextInfo extracts the ContextInfo (quoted message info) from various message types.
func (ms *MessageService) ExtractContextInfo(msg *events.Message) *waProto.ContextInfo {
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

// PersistMessage extracts message type, content, and media metadata from protobuf
// and saves it to the database.
func (ms *MessageService) PersistMessage(msg *events.Message) {
	if msg.Message.GetReactionMessage() != nil || 
	   msg.Message.GetProtocolMessage() != nil ||
	   msg.Message.GetSenderKeyDistributionMessage() != nil {
		return
	}

	msgType := "text"; content := ms.ExtractContent(msg); var thumb []byte
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

	chatJID := ms.ResolveJID(msg.Info.Chat).String()
	senderJID := ms.ResolveJID(msg.Info.Sender).String()
	
	if strings.HasSuffix(msg.Info.Sender.String(), "@lid") { ms.Contacts.ResolveLIDMapping(msg.Info.Sender.String()) }
	
	metadata.ID = msg.Info.ID; metadata.ChatJID = chatJID; metadata.SenderJID = senderJID
	metadata.Content = content; metadata.Type = msgType; metadata.Timestamp = msg.Info.Timestamp
	metadata.IsFromMe = msg.Info.IsFromMe; metadata.Thumbnail = thumb

	// Extract Quoted Message Context
	if ci := ms.ExtractContextInfo(msg); ci != nil && ci.GetStanzaID() != "" {
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
	
	ms.DB.SaveMessage(metadata)
	ms.DB.UpdateContactTimestamp(chatJID, msg.Info.Timestamp)
	
	if !msg.Info.IsFromMe {
		pushName := msg.Info.PushName; fullName := ""
		contactInfo, err := ms.Backend.Client.Store.Contacts.GetContact(ms.ctx, msg.Info.Sender)
		if err == nil && contactInfo.Found { if pushName == "" { pushName = contactInfo.PushName }; fullName = contactInfo.FullName }
		if pushName != "" || fullName != "" { ms.DB.SaveContact(database.Contact{JID: senderJID, SavedName: sql.NullString{String: fullName, Valid: fullName != ""}, PushName: sql.NullString{String: pushName, Valid: pushName != ""}}) }
	}
}

// PersistMediaMessage is a simplified persistence for pre-downloaded media messages.
func (ms *MessageService) PersistMediaMessage(msg *events.Message, msgType, path string) {
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
	ms.DB.SaveMessage(database.Message{
		ID: msg.Info.ID, ChatJID: chatJID, SenderJID: senderJID, Content: path, Type: msgType, 
		Timestamp: msg.Info.Timestamp, IsFromMe: msg.Info.IsFromMe, Thumbnail: thumb,
		MediaWidth: sql.NullInt64{Int64: width, Valid: width > 0},
		MediaHeight: sql.NullInt64{Int64: height, Valid: height > 0},
	})
	ms.DB.UpdateContactTimestamp(chatJID, msg.Info.Timestamp)
}

// HandleReaction saves a reaction to the database and updates the UI if the chat is selected.
func (ms *MessageService) HandleReaction(chat, sender types.JID, text, targetID string, timestamp time.Time) {
	react := database.Reaction{
		MessageID: targetID,
		SenderJID: sender.ToNonAD().String(),
		Reaction:  text,
		Timestamp: timestamp,
	}
	ms.DB.SaveReaction(react)
	
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
