package backend

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

func (b *Backend) SendText(ctx context.Context, to types.JID, text string, contextInfo *waProto.ContextInfo) (whatsmeow.SendResponse, error) {
	if contextInfo != nil {
		return b.Client.SendMessage(ctx, to, &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text:        proto.String(text),
				ContextInfo: contextInfo,
			},
		})
	}
	return b.Client.SendMessage(ctx, to, &waProto.Message{
		Conversation: proto.String(text),
	})
}

func (b *Backend) MarkRead(ctx context.Context, jid types.JID, ids []string, sender types.JID, timestamp time.Time) error {
	mids := make([]types.MessageID, len(ids))
	for i, id := range ids { mids[i] = types.MessageID(id) }
	return b.Client.MarkRead(ctx, mids, timestamp, jid, sender)
}

func (b *Backend) GetChats(ctx context.Context) ([]types.JID, error) {
	return nil, nil
}

func (b *Backend) LoadHistory(ctx context.Context, jid types.JID, count int, before types.MessageID) ([]interface{}, error) {
	return nil, nil
}

func (b *Backend) GetAllContacts(ctx context.Context) (map[types.JID]types.ContactInfo, error) {
	return b.Client.Store.Contacts.GetAllContacts(ctx)
}

func (b *Backend) GetJoinedGroups(ctx context.Context) ([]*types.GroupInfo, error) {
	return b.Client.GetJoinedGroups(ctx)
}

func (b *Backend) SendReaction(ctx context.Context, chat types.JID, msgID types.MessageID, fromMe bool, reaction string) (whatsmeow.SendResponse, error) {
	return b.Client.SendMessage(ctx, chat, &waProto.Message{
		ReactionMessage: &waProto.ReactionMessage{
			Key: &waProto.MessageKey{
				RemoteJID: proto.String(chat.ToNonAD().String()),
				FromMe:    proto.Bool(fromMe),
				ID:        proto.String(msgID),
			},
			Text:              proto.String(reaction),
			GroupingKey:       proto.String(reaction),
			SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
	})
}


func (b *Backend) SendImage(ctx context.Context, to types.JID, data []byte, mimetype string) (whatsmeow.SendResponse, error) {
	resp, err := b.Client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	return b.Client.SendMessage(ctx, to, &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			MediaKey:      resp.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	})
}
