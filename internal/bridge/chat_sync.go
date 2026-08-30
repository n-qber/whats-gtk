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
}

func (cc *ChatController) updateGroupInfoUI(grpInfo *types.GroupInfo) {
	if grpInfo == nil {
		return
	}

	var participants []info.ParticipantModel
	for _, p := range grpInfo.Participants {
		name := p.JID.User
		contact, err := cc.DB.GetContact(p.JID.String())
		if err == nil && contact.DisplayName() != "" {
			name = contact.DisplayName()
		} else if p.DisplayName != "" {
			name = p.DisplayName
		}

		participants = append(participants, info.ParticipantModel{
			Name:    name,
			JID:     p.JID.String(),
			IsAdmin: p.IsAdmin || p.IsSuperAdmin,
			Avatar:  cc.Contacts.GetAvatarNoFetch(p.JID.String()),
		})
	}

	glib.IdleAdd(func() {
		if cc.selectedJID != nil && cc.selectedJID.ToNonAD().String() == grpInfo.JID.ToNonAD().String() {
			cc.App.InfoView.SetGroupDetails(grpInfo.Topic, participants)
		}
	})
}

// RefreshSidebarUI fetches contacts and publishes them as SidebarItems.
func (cc *ChatController) RefreshSidebarUI() {
	if c, err := cc.DB.GetAllContacts(cc.activeProfileID, 200); err == nil {
		var items []events.SidebarItem
		for _, contact := range c {
			if contact.IsArchived {
				continue
			}
			tex := cc.Contacts.GetAvatarNoFetch(contact.JID)
			items = append(items, events.SidebarItem{
				JID:         contact.JID,
				Name:        contact.DisplayName(),
				IsGroup:     contact.IsGroup.Valid && contact.IsGroup.Bool,
				UnreadCount: contact.UnreadCount,
				IsPinned:    contact.IsPinned,
				Avatar:      tex,
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
