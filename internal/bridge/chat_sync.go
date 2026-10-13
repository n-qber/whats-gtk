package bridge

import (
	"fmt"
	"time"

	"whats-gtk/internal/database"
	"whats-gtk/internal/events"
	"whats-gtk/internal/ui/info"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"go.mau.fi/whatsmeow/types"
)

// GetGroupParticipantCount returns the total number of participants in a group.
func (cc *ChatController) GetGroupParticipantCount(jid types.JID) int {
	cleanJID := jid.ToNonAD().String()

	cc.groupMutex.RLock()
	info, ok := cc.cachedGroupInfo[cleanJID]
	cc.groupMutex.RUnlock()

	if ok && info != nil {
		return len(info.Participants)
	}

	cc.SyncGroupIfNeeded(jid)
	return 0
}

// SyncGroupIfNeeded fetches group info if it hasn't been synced recently (30 min throttle).
func (cc *ChatController) SyncGroupIfNeeded(jid types.JID) {
	if cc.Backend == nil || cc.Backend.Client == nil {
		return
	}

	cleanJID := jid.ToNonAD().String()

	cc.groupMutex.RLock()
	lastSync, exists := cc.lastGroupSync[cleanJID]
	cached, hasCached := cc.cachedGroupInfo[cleanJID]
	cc.groupMutex.RUnlock()

	if hasCached && cached != nil {
		cc.updateGroupInfoUI(cached)
	}

	if !exists || time.Since(lastSync) > 30*time.Minute {
		cc.SyncGroupForce(jid)
	}
}

// SyncGroupForce forces fetching fresh group information and sub-groups from WhatsApp servers.
func (cc *ChatController) SyncGroupForce(jid types.JID) {
	if cc.Backend == nil || cc.Backend.Client == nil {
		return
	}

	go func(groupJID types.JID) {
		info, err := cc.Backend.GetGroupInfo(cc.ctx, groupJID)
		if err != nil || info == nil {
			fmt.Printf("Bridge: Failed to get group info for %s: %v\n", groupJID, err)
			return
		}

		cc.groupMutex.Lock()
		cc.lastGroupSync[groupJID.ToNonAD().String()] = time.Now()
		cc.cachedGroupInfo[groupJID.ToNonAD().String()] = info
		cc.groupMutex.Unlock()

		isComm := info.IsParent
		parentJIDStr := info.LinkedParentJID.ToNonAD().String()
		_ = cc.DB.SetContactCommunity(groupJID.String(), isComm, parentJIDStr)

		if isComm {
			if targets, err := cc.Backend.GetSubGroups(cc.ctx, groupJID); err == nil {
				var dbSubs []database.CommunitySubGroup
				for _, t := range targets {
					dbSubs = append(dbSubs, database.CommunitySubGroup{
						CommunityJID: groupJID.String(),
						GroupJID:     t.JID.String(),
						GroupName:    t.Name,
						IsDefaultSub: t.IsDefaultSubGroup,
					})
				}
				_ = cc.DB.SaveCommunitySubGroups(groupJID.String(), dbSubs)
			}
		}

		for _, p := range info.Participants {
			pn := p.PhoneNumber.ToNonAD().String()
			lid := p.LID.ToNonAD().String()
			if pn != "" && lid != "" {
				cc.DB.MergeLID(pn, lid)
			} else {
				cc.DB.SaveContact(database.Contact{JID: p.JID.ToNonAD().String()})
			}
		}

		cc.updateGroupInfoUI(info)
	}(jid)
}

func (cc *ChatController) updateGroupInfoUI(grpInfo *types.GroupInfo) {
	if grpInfo == nil {
		return
	}

	cleanJID := grpInfo.JID.ToNonAD().String()

	var myJIDs []string
	if cc.Backend != nil && cc.Backend.Client != nil && cc.Backend.Client.Store != nil && cc.Backend.Client.Store.ID != nil {
		myJIDs = append(myJIDs, cc.Backend.Client.Store.ID.ToNonAD().String())
		if cc.Backend.Client.Store.LID.User != "" {
			myJIDs = append(myJIDs, cc.Backend.Client.Store.LID.ToNonAD().String())
		}
	}

	isAdmin := false
	isOwner := false
	var participants []info.ParticipantModel

	for _, p := range grpInfo.Participants {
		name := p.JID.User
		contact, err := cc.DB.GetContact(p.JID.String())
		if err == nil && contact != nil && contact.DisplayName() != "" {
			name = contact.DisplayName()
		} else if p.DisplayName != "" {
			name = p.DisplayName
		}

		pClean := p.JID.ToNonAD().String()
		pPN := p.PhoneNumber.ToNonAD().String()
		pLID := p.LID.ToNonAD().String()

		isPartOwner := p.IsSuperAdmin || (grpInfo.OwnerJID.String() != "" && grpInfo.OwnerJID.ToNonAD().String() == pClean)

		for _, myJ := range myJIDs {
			if pClean == myJ || (pPN != "" && pPN == myJ) || (pLID != "" && pLID == myJ) {
				if p.IsAdmin || p.IsSuperAdmin {
					isAdmin = true
				}
				if isPartOwner {
					isOwner = true
				}
			}
		}

		participants = append(participants, info.ParticipantModel{
			Name:    name,
			JID:     p.JID.String(),
			IsAdmin: p.IsAdmin || p.IsSuperAdmin,
			IsOwner: isPartOwner,
			Avatar:  cc.Contacts.GetAvatarNoFetch(p.JID.String()),
		})
	}

	var subGroups []info.CommunitySubGroupModel
	if grpInfo.IsParent {
		dbSubs, _ := cc.DB.GetCommunitySubGroups(grpInfo.JID.String())
		for _, s := range dbSubs {
			subGroups = append(subGroups, info.CommunitySubGroupModel{
				JID:          s.GroupJID,
				Name:         s.GroupName,
				IsDefaultSub: s.IsDefaultSub,
			})
		}
	}

	parentJID := grpInfo.LinkedParentJID.ToNonAD().String()
	parentName := ""
	if parentJID != "" {
		if parentContact, err := cc.DB.GetContact(parentJID); err == nil && parentContact != nil {
			parentName = parentContact.DisplayName()
		}
	}

	myJIDStr := ""
	if len(myJIDs) > 0 {
		myJIDStr = myJIDs[0]
	}

	extDetails := info.ExtendedGroupDetails{
		JID:             grpInfo.JID.String(),
		Description:     grpInfo.Topic,
		IsCommunity:     grpInfo.IsParent,
		IsAdmin:         isAdmin,
		IsOwner:         isOwner,
		IsAnnounce:      grpInfo.IsAnnounce,
		IsLocked:        grpInfo.IsLocked,
		ParentCommunity: parentJID,
		ParentName:      parentName,
		MyJID:           myJIDStr,
		SubGroups:       subGroups,
		Participants:    participants,
	}

	glib.IdleAdd(func() {
		if cc.selectedJID != nil && cc.selectedJID.ToNonAD().String() == cleanJID {
			cc.App.InfoView.SetExtendedGroupDetails(extDetails)
		}
	})
}

// RefreshSidebarUI fetches contacts and publishes them as SidebarItems.
func (cc *ChatController) RefreshSidebarUI() {
	if c, err := cc.DB.GetAllContacts(cc.activeProfileID, 200); err == nil {
		var items []events.SidebarItem
		for _, contact := range c {
			if cc.activeProfileID == database.ProfileArchivedID {
				if !contact.IsArchived {
					continue
				}
			} else if contact.IsArchived {
				continue
			}
			tex := cc.Contacts.GetAvatarNoFetch(contact.JID)
			isComm := cc.DB.IsCommunity(contact.JID)
			parentJID := cc.DB.GetParentCommunity(contact.JID)
			items = append(items, events.SidebarItem{
				JID:                contact.JID,
				Name:               contact.DisplayName(),
				IsGroup:            contact.IsGroup.Valid && contact.IsGroup.Bool,
				IsCommunity:        isComm,
				ParentCommunityJID: parentJID,
				UnreadCount:        contact.UnreadCount,
				IsPinned:           contact.IsPinned,
				IsMuted:            contact.IsMuted(),
				Avatar:             tex,
			})
		}
		cc.EventBus.Publish(events.Event{
			Type: events.EventContactsUpdated,
			Data: items,
		})
	}
}

func (cc *ChatController) GetActiveProfileID() int64 {
	return cc.activeProfileID
}

func (cc *ChatController) SetActiveProfileID(id int64) {
	cc.activeProfileID = id
	_ = cc.DB.SetActiveProfileID(id)
	cc.RefreshSidebarUI()
	cc.EventBus.Publish(events.Event{
		Type: events.EventActiveProfileChanged,
		Data: id,
	})
}
