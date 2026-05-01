package backend

import (
	"context"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func (b *Backend) GetGroupInfo(ctx context.Context, jid types.JID) (*types.GroupInfo, error) {
	return b.Client.GetGroupInfo(ctx, jid)
}

func (b *Backend) CreateGroup(ctx context.Context, name string, participants []types.JID) (*types.GroupInfo, error) {
	return b.Client.CreateGroup(ctx, whatsmeow.ReqCreateGroup{
		Name:         name,
		Participants: participants,
	})
}
