package backend

import (
	"context"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

func (b *Backend) SendText(ctx context.Context, to types.JID, text string) (whatsmeow.SendResponse, error) {
	return b.Client.SendMessage(ctx, to, &waProto.Message{
		Conversation: proto.String(text),
	})
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

func (b *Backend) RevokeMessage(ctx context.Context, chat types.JID, sender types.JID, id types.MessageID) (whatsmeow.SendResponse, error) {
	return b.Client.SendMessage(ctx, chat, b.Client.BuildRevoke(chat, sender, id))
}
