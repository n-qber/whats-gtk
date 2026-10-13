package bridge

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"whats-gtk/internal/database"
	"whats-gtk/internal/ui/info"
)

// WireInfoViewCallbacks sets up all event handlers for group and community actions on the InfoView.
func (cc *ChatController) WireInfoViewCallbacks() {
	if cc.App == nil || cc.App.InfoView == nil {
		return
	}

	iv := cc.App.InfoView

	iv.OnExitGroup = func() {
		if cc.selectedJID == nil {
			return
		}
		cc.HandleLeaveGroup(*cc.selectedJID)
	}

	iv.OnAddParticipants = func() {
		if cc.selectedJID == nil {
			return
		}
		cc.HandleOpenAddParticipants(*cc.selectedJID)
	}

	iv.OnRemoveParticipant = func(partJID string) {
		if cc.selectedJID == nil {
			return
		}
		cc.HandleRemoveParticipant(*cc.selectedJID, partJID)
	}

	iv.OnPromoteParticipant = func(partJID string) {
		if cc.selectedJID == nil {
			return
		}
		cc.HandlePromoteParticipant(*cc.selectedJID, partJID)
	}

	iv.OnDemoteParticipant = func(partJID string) {
		if cc.selectedJID == nil {
			return
		}
		cc.HandleDemoteParticipant(*cc.selectedJID, partJID)
	}

	iv.OnMessageParticipant = func(partJID string) {
		cc.App.InfoFlap.SetRevealFlap(false)
		cc.HandleChatSelected(partJID)
	}

	iv.OnEditGroupName = func(newName string) {
		if cc.selectedJID == nil {
			return
		}
		cc.HandleUpdateGroupName(*cc.selectedJID, newName)
	}

	iv.OnEditGroupDescription = func(newDesc string) {
		if cc.selectedJID == nil {
			return
		}
		cc.HandleUpdateGroupDescription(*cc.selectedJID, newDesc)
	}

	iv.OnToggleAnnounce = func(announce bool) {
		if cc.selectedJID == nil {
			return
		}
		cc.HandleSetGroupAnnounce(*cc.selectedJID, announce)
	}

	iv.OnToggleLocked = func(locked bool) {
		if cc.selectedJID == nil {
			return
		}
		cc.HandleSetGroupLocked(*cc.selectedJID, locked)
	}

	iv.OnGetInviteLink = func() {
		if cc.selectedJID == nil {
			return
		}
		cc.HandleGetGroupInviteLink(*cc.selectedJID)
	}

	iv.OnSelectSubGroup = func(subGroupJID string) {
		cc.App.InfoFlap.SetRevealFlap(false)
		cc.HandleChatSelected(subGroupJID)
	}

	iv.OnUnlinkSubGroup = func(subGroupJID string) {
		if cc.selectedJID == nil {
			return
		}
		cc.HandleUnlinkGroup(*cc.selectedJID, subGroupJID)
	}

	iv.OnLinkExistingGroup = func() {
		if cc.selectedJID == nil {
			return
		}
		cc.HandleOpenLinkGroupDialog(*cc.selectedJID)
	}

	iv.OnCreateSubGroup = func() {
		if cc.selectedJID == nil {
			return
		}
		cc.HandleOpenCreateSubGroupDialog(*cc.selectedJID)
	}

	iv.OnOpenParentCommunity = func(parentJID string) {
		cc.App.InfoFlap.SetRevealFlap(false)
		cc.HandleChatSelected(parentJID)
	}
}

// HandleLeaveGroup leaves a group or community.
func (cc *ChatController) HandleLeaveGroup(jid types.JID) {
	go func() {
		if cc.Backend == nil {
			return
		}
		err := cc.Backend.LeaveGroup(cc.ctx, jid)
		if err != nil {
			fmt.Printf("Bridge: Error leaving group %s: %v\n", jid, err)
			return
		}

		glib.IdleAdd(func() {
			if cc.App.InfoFlap != nil {
				cc.App.InfoFlap.SetRevealFlap(false)
			}
			if cc.App.ChatView != nil {
				cc.App.ChatView.Clear()
			}
			cc.App.ActiveMainJID = ""
			cc.RefreshSidebarUI()
		})
	}()
}

// HandleOpenAddParticipants opens the dialog to add participants to a group.
func (cc *ChatController) HandleOpenAddParticipants(groupJID types.JID) {
	contacts, err := cc.DB.GetAllContacts(0, 1000)
	if err != nil {
		return
	}

	// Filter out current group participants
	var existingMap = make(map[string]bool)
	cc.groupMutex.RLock()
	cached := cc.cachedGroupInfo[groupJID.ToNonAD().String()]
	cc.groupMutex.RUnlock()
	if cached != nil {
		for _, p := range cached.Participants {
			existingMap[p.JID.ToNonAD().String()] = true
			if p.PhoneNumber.User != "" {
				existingMap[p.PhoneNumber.ToNonAD().String()] = true
			}
			if p.LID.User != "" {
				existingMap[p.LID.ToNonAD().String()] = true
			}
		}
	}

	var items []info.ContactItem
	for _, c := range contacts {
		if c.IsGroup.Valid && c.IsGroup.Bool {
			continue
		}
		clean := c.JID
		if existingMap[clean] {
			continue
		}
		items = append(items, info.ContactItem{
			JID:    c.JID,
			Name:   c.DisplayName(),
			Avatar: cc.Contacts.GetAvatarNoFetch(c.JID),
		})
	}

	var parentWin *gtk.Window
	if cc.App.Window != nil {
		parentWin = &cc.App.Window.Window
	}

	info.ShowAddParticipantsDialog(parentWin, items, func(targetJIDs []string) {
		cc.HandleAddParticipants(groupJID, targetJIDs)
	})
}

// HandleAddParticipants adds participants to a group.
func (cc *ChatController) HandleAddParticipants(groupJID types.JID, targetJIDs []string) {
	var jids []types.JID
	for _, s := range targetJIDs {
		if parsed, err := types.ParseJID(s); err == nil {
			jids = append(jids, parsed)
		}
	}
	if len(jids) == 0 {
		return
	}

	go func() {
		_, err := cc.Backend.UpdateGroupParticipants(cc.ctx, groupJID, jids, whatsmeow.ParticipantChangeAdd)
		if err != nil {
			fmt.Printf("Bridge: Error adding participants to %s: %v\n", groupJID, err)
			return
		}
		cc.SyncGroupForce(groupJID)
	}()
}

// HandleRemoveParticipant removes a participant from a group.
func (cc *ChatController) HandleRemoveParticipant(groupJID types.JID, targetJID string) {
	parsed, err := types.ParseJID(targetJID)
	if err != nil {
		return
	}

	go func() {
		_, err := cc.Backend.UpdateGroupParticipants(cc.ctx, groupJID, []types.JID{parsed}, whatsmeow.ParticipantChangeRemove)
		if err != nil {
			fmt.Printf("Bridge: Error removing participant %s from %s: %v\n", targetJID, groupJID, err)
			return
		}
		cc.SyncGroupForce(groupJID)
	}()
}

// HandlePromoteParticipant promotes a participant to admin.
func (cc *ChatController) HandlePromoteParticipant(groupJID types.JID, targetJID string) {
	parsed, err := types.ParseJID(targetJID)
	if err != nil {
		return
	}

	go func() {
		_, err := cc.Backend.UpdateGroupParticipants(cc.ctx, groupJID, []types.JID{parsed}, whatsmeow.ParticipantChangePromote)
		if err != nil {
			fmt.Printf("Bridge: Error promoting participant %s in %s: %v\n", targetJID, groupJID, err)
			return
		}
		cc.SyncGroupForce(groupJID)
	}()
}

// HandleDemoteParticipant removes admin status from a participant.
func (cc *ChatController) HandleDemoteParticipant(groupJID types.JID, targetJID string) {
	parsed, err := types.ParseJID(targetJID)
	if err != nil {
		return
	}

	go func() {
		_, err := cc.Backend.UpdateGroupParticipants(cc.ctx, groupJID, []types.JID{parsed}, whatsmeow.ParticipantChangeDemote)
		if err != nil {
			fmt.Printf("Bridge: Error demoting participant %s in %s: %v\n", targetJID, groupJID, err)
			return
		}
		cc.SyncGroupForce(groupJID)
	}()
}

// HandleUpdateGroupName updates the name of a group or community.
func (cc *ChatController) HandleUpdateGroupName(groupJID types.JID, newName string) {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return
	}

	go func() {
		err := cc.Backend.SetGroupName(cc.ctx, groupJID, newName)
		if err != nil {
			fmt.Printf("Bridge: Error updating group name for %s: %v\n", groupJID, err)
			return
		}
		glib.IdleAdd(func() {
			if cc.selectedJID != nil && cc.selectedJID.ToNonAD().String() == groupJID.ToNonAD().String() {
				isComm := cc.DB.IsCommunity(groupJID.String())
				title := newName
				if isComm {
					title = "[C] " + title
				} else {
					title = "[G] " + title
				}
				cc.App.ChatView.SetHeader(title, cc.Contacts.GetAvatarNoFetch(groupJID.String()))
				cc.App.InfoView.NameLabel.SetText(newName)
			}
			cc.RefreshSidebarUI()
		})
	}()
}

// HandleUpdateGroupDescription updates the description of a group or community.
func (cc *ChatController) HandleUpdateGroupDescription(groupJID types.JID, newDesc string) {
	go func() {
		err := cc.Backend.SetGroupDescription(cc.ctx, groupJID, newDesc)
		if err != nil {
			fmt.Printf("Bridge: Error updating group description for %s: %v\n", groupJID, err)
			return
		}
		glib.IdleAdd(func() {
			if cc.selectedJID != nil && cc.selectedJID.ToNonAD().String() == groupJID.ToNonAD().String() {
				cc.App.InfoView.SetGroupDescription(newDesc)
				cc.App.InfoView.GroupDescGroup.Show()
			}
		})
	}()
}

// HandleSetGroupAnnounce changes the message sending restriction (only admins vs all).
func (cc *ChatController) HandleSetGroupAnnounce(groupJID types.JID, announce bool) {
	go func() {
		err := cc.Backend.SetGroupAnnounce(cc.ctx, groupJID, announce)
		if err != nil {
			fmt.Printf("Bridge: Error setting group announce for %s: %v\n", groupJID, err)
			return
		}
		cc.SyncGroupForce(groupJID)
	}()
}

// HandleSetGroupLocked changes the group info editing restriction (only admins vs all).
func (cc *ChatController) HandleSetGroupLocked(groupJID types.JID, locked bool) {
	go func() {
		err := cc.Backend.SetGroupLocked(cc.ctx, groupJID, locked)
		if err != nil {
			fmt.Printf("Bridge: Error setting group locked for %s: %v\n", groupJID, err)
			return
		}
		cc.SyncGroupForce(groupJID)
	}()
}

// HandleGetGroupInviteLink retrieves the group invite link and copies it to clipboard.
func (cc *ChatController) HandleGetGroupInviteLink(groupJID types.JID) {
	go func() {
		link, err := cc.Backend.GetGroupInviteLink(cc.ctx, groupJID, false)
		if err != nil {
			fmt.Printf("Bridge: Error getting invite link for %s: %v\n", groupJID, err)
			return
		}
		glib.IdleAdd(func() {
			display := gdk.DisplayGetDefault()
			if display != nil {
				display.Clipboard().SetText(link)
			}
			if cc.App.InfoView != nil {
				cc.App.InfoView.InviteLinkRow.SetSubtitle(link)
			}

			var parentWin *gtk.Window
			if cc.App.Window != nil {
				parentWin = &cc.App.Window.Window
			}
			dialog := adw.NewMessageDialog(parentWin, "Link de convite copiado!", link)
			dialog.AddResponse("ok", "Fechar")
			dialog.SetDefaultResponse("ok")
			dialog.Present()
		})
	}()
}

// HandleOpenLinkGroupDialog opens the dialog to link an existing group to a community.
func (cc *ChatController) HandleOpenLinkGroupDialog(communityJID types.JID) {
	groups, err := cc.DB.GetAvailableGroupsForCommunity(communityJID.String())
	if err != nil {
		return
	}

	var items []info.ContactItem
	for _, g := range groups {
		items = append(items, info.ContactItem{
			JID:    g.JID,
			Name:   g.DisplayName(),
			Avatar: cc.Contacts.GetAvatarNoFetch(g.JID),
		})
	}

	var parentWin *gtk.Window
	if cc.App.Window != nil {
		parentWin = &cc.App.Window.Window
	}

	info.ShowLinkGroupDialog(parentWin, items, func(targetJID string) {
		cc.HandleLinkGroupToCommunity(communityJID, targetJID)
	})
}

// HandleLinkGroupToCommunity links an existing group to a community.
func (cc *ChatController) HandleLinkGroupToCommunity(communityJID types.JID, groupJIDStr string) {
	childJID, err := types.ParseJID(groupJIDStr)
	if err != nil {
		return
	}

	go func() {
		err := cc.Backend.LinkGroup(cc.ctx, communityJID, childJID)
		if err != nil {
			fmt.Printf("Bridge: Error linking group %s to community %s: %v\n", groupJIDStr, communityJID, err)
			return
		}
		_ = cc.DB.SetContactCommunity(childJID.String(), false, communityJID.String())
		cc.SyncGroupForce(communityJID)
	}()
}

// HandleUnlinkGroup unlinks a sub-group from a community.
func (cc *ChatController) HandleUnlinkGroup(communityJID types.JID, groupJIDStr string) {
	childJID, err := types.ParseJID(groupJIDStr)
	if err != nil {
		return
	}

	go func() {
		err := cc.Backend.UnlinkGroup(cc.ctx, communityJID, childJID)
		if err != nil {
			fmt.Printf("Bridge: Error unlinking group %s from community %s: %v\n", groupJIDStr, communityJID, err)
			return
		}
		_ = cc.DB.SetContactCommunity(childJID.String(), false, "")
		cc.SyncGroupForce(communityJID)
	}()
}

// HandleOpenCreateSubGroupDialog opens the dialog to create a new sub-group in a community.
func (cc *ChatController) HandleOpenCreateSubGroupDialog(communityJID types.JID) {
	var parentWin *gtk.Window
	if cc.App.Window != nil {
		parentWin = &cc.App.Window.Window
	}

	commName := communityJID.String()
	if c, err := cc.DB.GetContact(communityJID.String()); err == nil && c != nil {
		commName = c.DisplayName()
	}

	info.ShowCreateSubGroupDialog(parentWin, commName, func(name string) {
		cc.HandleCreateSubGroup(communityJID, name)
	})
}

// HandleCreateSubGroup creates a new group inside a community.
func (cc *ChatController) HandleCreateSubGroup(communityJID types.JID, name string) {
	go func() {
		info, err := cc.Backend.CreateSubGroup(cc.ctx, communityJID, name, nil)
		if err != nil {
			fmt.Printf("Bridge: Error creating sub-group in %s: %v\n", communityJID, err)
			return
		}
		_ = cc.DB.SetContactCommunity(info.JID.String(), false, communityJID.String())
		cc.SyncGroupForce(communityJID)

		glib.IdleAdd(func() {
			cc.RefreshSidebarUI()
			cc.HandleChatSelected(info.JID.String())
		})
	}()
}

// HandleCreateCommunity creates a new parent community from scratch.
func (cc *ChatController) HandleCreateCommunity(name, desc string) {
	go func() {
		info, err := cc.Backend.CreateCommunity(cc.ctx, name, desc)
		if err != nil {
			fmt.Printf("Bridge: Error creating community: %v\n", err)
			return
		}
		_ = cc.DB.SetContactCommunity(info.JID.String(), true, "")
		_ = cc.DB.SaveContact(database.Contact{
			JID:       info.JID.String(),
			SavedName: sql.NullString{String: name, Valid: true},
			IsGroup:   sql.NullBool{Bool: true, Valid: true},
		})

		glib.IdleAdd(func() {
			cc.RefreshSidebarUI()
			cc.HandleChatSelected(info.JID.String())
		})
	}()
}

// HandleCreateGroup creates a new standard group from scratch.
func (cc *ChatController) HandleCreateGroup(name string, participants []string) {
	var jids []types.JID
	for _, p := range participants {
		if parsed, err := types.ParseJID(p); err == nil {
			jids = append(jids, parsed)
		}
	}

	go func() {
		info, err := cc.Backend.CreateGroup(cc.ctx, name, jids)
		if err != nil {
			fmt.Printf("Bridge: Error creating group: %v\n", err)
			return
		}
		_ = cc.DB.SaveContact(database.Contact{
			JID:       info.JID.String(),
			SavedName: sql.NullString{String: name, Valid: true},
			IsGroup:   sql.NullBool{Bool: true, Valid: true},
		})

		glib.IdleAdd(func() {
			cc.RefreshSidebarUI()
			cc.HandleChatSelected(info.JID.String())
		})
	}()
}
