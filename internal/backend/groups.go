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

// CreateCommunity creates a new parent community group.
func (b *Backend) CreateCommunity(ctx context.Context, name string, description string) (*types.GroupInfo, error) {
	info, err := b.Client.CreateGroup(ctx, whatsmeow.ReqCreateGroup{
		Name: name,
		GroupParent: types.GroupParent{
			IsParent: true,
		},
	})
	if err != nil {
		return nil, err
	}
	if description != "" {
		_ = b.Client.SetGroupTopic(ctx, info.JID, "", "", description)
	}
	return info, nil
}

// CreateSubGroup creates a new group linked inside an existing community.
func (b *Backend) CreateSubGroup(ctx context.Context, parentCommunityJID types.JID, name string, participants []types.JID) (*types.GroupInfo, error) {
	return b.Client.CreateGroup(ctx, whatsmeow.ReqCreateGroup{
		Name:         name,
		Participants: participants,
		GroupLinkedParent: types.GroupLinkedParent{
			LinkedParentJID: parentCommunityJID,
		},
	})
}

// GetSubGroups returns all groups linked to a community.
func (b *Backend) GetSubGroups(ctx context.Context, community types.JID) ([]*types.GroupLinkTarget, error) {
	return b.Client.GetSubGroups(ctx, community)
}

// LinkGroup links an existing group to a community.
func (b *Backend) LinkGroup(ctx context.Context, parent, child types.JID) error {
	return b.Client.LinkGroup(ctx, parent, child)
}

// UnlinkGroup removes a group from a community.
func (b *Backend) UnlinkGroup(ctx context.Context, parent, child types.JID) error {
	return b.Client.UnlinkGroup(ctx, parent, child)
}

// LeaveGroup leaves a group or community.
func (b *Backend) LeaveGroup(ctx context.Context, jid types.JID) error {
	return b.Client.LeaveGroup(ctx, jid)
}

// UpdateGroupParticipants adds, removes, promotes or demotes participants.
func (b *Backend) UpdateGroupParticipants(ctx context.Context, jid types.JID, participants []types.JID, action whatsmeow.ParticipantChange) ([]types.GroupParticipant, error) {
	return b.Client.UpdateGroupParticipants(ctx, jid, participants, action)
}

// SetGroupName changes the group or community name.
func (b *Backend) SetGroupName(ctx context.Context, jid types.JID, name string) error {
	return b.Client.SetGroupName(ctx, jid, name)
}

// SetGroupDescription changes the group or community topic/description.
func (b *Backend) SetGroupDescription(ctx context.Context, jid types.JID, description string) error {
	return b.Client.SetGroupTopic(ctx, jid, "", "", description)
}

// SetGroupPhoto changes the group avatar.
func (b *Backend) SetGroupPhoto(ctx context.Context, jid types.JID, avatar []byte) (string, error) {
	return b.Client.SetGroupPhoto(ctx, jid, avatar)
}

// SetGroupLocked sets whether only admins can edit group metadata.
func (b *Backend) SetGroupLocked(ctx context.Context, jid types.JID, locked bool) error {
	return b.Client.SetGroupLocked(ctx, jid, locked)
}

// SetGroupAnnounce sets whether only admins can send messages.
func (b *Backend) SetGroupAnnounce(ctx context.Context, jid types.JID, announce bool) error {
	return b.Client.SetGroupAnnounce(ctx, jid, announce)
}

// GetGroupInviteLink retrieves or resets the group invite link.
func (b *Backend) GetGroupInviteLink(ctx context.Context, jid types.JID, reset bool) (string, error) {
	return b.Client.GetGroupInviteLink(ctx, jid, reset)
}
