package bridge

import (
	"fmt"
	"strings"

	"whats-gtk/internal/database"
	"whats-gtk/internal/events"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"go.mau.fi/whatsmeow/types"
)

// RefreshMessagesAround loads messages centered around targetID.
func (cc *ChatController) RefreshMessagesAround(jid types.JID, targetID string) {
	go func() {
		jids := cc.GetChatJIDs(jid.ToNonAD().String())

		msgs, err := cc.DB.GetMessagesAround(jids, targetID, 50)
		if err != nil || len(msgs) == 0 {
			cc.RefreshMessages(jid)
			return
		}

		if len(msgs) > 0 {
			cc.OldestMessageTimes[jid.ToNonAD().String()] = msgs[0].Timestamp
		}

		var uiMsgs []events.UIMessage
		var lastSender string
		var localLastDateStr string

		for _, m := range msgs {
			sName := ""
			var av *gdk.Texture
			isCont := m.SenderJID == lastSender

			if jid.Server == types.GroupServer && !m.IsFromMe {
				if !isCont {
					sName = cc.Contacts.ResolveSenderName(m.SenderJID)
					av = cc.Contacts.GetAvatarNoFetch(m.SenderJID)
				}
			}
			lastSender = m.SenderJID

			dateStr := cc.Mapper.FormatMessageDate(m.Timestamp)
			tStr := m.Timestamp.Format("15:04")

			uiMsg := cc.Mapper.MapMessage(m, sName, tStr, av, isCont)
			if dateStr != localLastDateStr {
				uiMsg.DateSeparator = dateStr
				localLastDateStr = dateStr
			} else {
				uiMsg.DateSeparator = ""
			}
			uiMsgs = append(uiMsgs, uiMsg)
		}

		cc.EventBus.Publish(events.Event{
			Type: events.EventChatMessagesLoaded,
			Data: events.ChatMessagesPayload{
				JID:      jid.ToNonAD().String(),
				Messages: uiMsgs,
				Append:   false,
			},
		})
	}()
}

// CancelMessageSearch clears the search state and restores the original chat messages.
func (cc *ChatController) CancelMessageSearch(jidStr string) {
	targetJID, _ := types.ParseJID(jidStr)
	cc.RefreshMessages(targetJID)
}

func (cc *ChatController) CancelMessageSearchAndJump(jidStr string, targetID string) {
	targetJID, _ := types.ParseJID(jidStr)
	cc.RefreshMessagesAround(targetJID, targetID)
}

// HandleSearch performs a debounced search on contacts.
func (cc *ChatController) HandleSearch(t string) {
	cc.sidebarMutex.Lock()
	cc.searchSerial++
	serial := cc.searchSerial
	cc.sidebarMutex.Unlock()

	go func() {
		var c []database.Contact
		var err error
		if strings.TrimSpace(t) == "" {
			c, err = cc.DB.GetAllContacts(cc.activeProfileID, 100)
		} else {
			c, err = cc.DB.SearchContacts(cc.activeProfileID, t, 200)
		}

		cc.sidebarMutex.Lock()
		if serial != cc.searchSerial {
			cc.sidebarMutex.Unlock()
			return
		}
		cc.sidebarMutex.Unlock()

		if err != nil {
			fmt.Printf("Bridge: Search failed: %v\n", err)
			return
		}
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
	}()
}

func (cc *ChatController) RenderMessageSearch(jidStr string, query string) {
	go func() {
		jids := cc.GetChatJIDs(jidStr)

		msgs, err := cc.DB.SearchMessagesInChat(jids, query, 100)
		if err != nil {
			fmt.Printf("Controller: SearchMessagesInChat failed: %v\n", err)
			return
		}

		var uiMsgs []events.UIMessage
		var lastSender string
		var localLastDateStr string

		for _, m := range msgs {
			sName := ""
			var av *gdk.Texture
			isCont := m.SenderJID == lastSender

			targetJID, _ := types.ParseJID(jidStr)
			if targetJID.Server == types.GroupServer && !m.IsFromMe {
				if !isCont {
					sName = cc.Contacts.ResolveSenderName(m.SenderJID)
					av = cc.Contacts.GetAvatarNoFetch(m.SenderJID)
				}
			}
			lastSender = m.SenderJID

			dateStr := cc.Mapper.FormatMessageDate(m.Timestamp)
			tStr := m.Timestamp.Format("15:04")

			uiMsg := cc.Mapper.MapMessage(m, sName, tStr, av, isCont)
			if dateStr != localLastDateStr {
				uiMsg.DateSeparator = dateStr
				localLastDateStr = dateStr
			} else {
				uiMsg.DateSeparator = ""
			}
			uiMsgs = append(uiMsgs, uiMsg)
		}

		cc.EventBus.Publish(events.Event{
			Type: events.EventChatMessagesLoaded,
			Data: events.ChatMessagesPayload{
				JID:      jidStr,
				Messages: uiMsgs,
				Append:   false,
			},
		})
	}()
}

func (cc *ChatController) LoadOlderMessages(jidStr string, targetID string) {
	go func() {
		publishEmpty := func() {
			cc.EventBus.Publish(events.Event{
				Type: events.EventChatMessagesLoaded,
				Data: events.ChatMessagesPayload{
					JID:      jidStr,
					Messages: nil,
					Append:   true,
				},
			})
		}

		before, ok := cc.OldestMessageTimes[jidStr]
		if !ok {
			publishEmpty()
			return
		}

		jids := cc.GetChatJIDs(jidStr)

		var msgs []database.Message
		var err error
		if targetID == "" {
			msgs, err = cc.DB.GetOlderMessages(jids, before, 50)
		} else {
			targetMsg, e := cc.DB.GetMessage(targetID)
			if e != nil || !targetMsg.Timestamp.Before(before) {
				publishEmpty()
				return
			}
			msgs, err = cc.DB.GetMessagesBetween(jids, targetMsg.Timestamp, before, 300)
		}

		if err != nil || len(msgs) == 0 {
			publishEmpty()
			return
		}

		cc.OldestMessageTimes[jidStr] = msgs[0].Timestamp

		var uiMsgs []events.UIMessage
		var localLastDateStr string
		for _, m := range msgs {
			sName := ""
			var av *gdk.Texture
			isCont := false

			targetJID, _ := types.ParseJID(jidStr)
			if targetJID.Server == types.GroupServer && !m.IsFromMe {
				sName = cc.Contacts.ResolveSenderName(m.SenderJID)
				av = cc.Contacts.GetAvatarNoFetch(m.SenderJID)
			}

			dateStr := cc.Mapper.FormatMessageDate(m.Timestamp)
			tStr := m.Timestamp.Format("15:04")

			uiMsg := cc.Mapper.MapMessage(m, sName, tStr, av, isCont)
			if dateStr != localLastDateStr {
				uiMsg.DateSeparator = dateStr
				localLastDateStr = dateStr
			} else {
				uiMsg.DateSeparator = ""
			}
			uiMsgs = append(uiMsgs, uiMsg)
		}

		cc.EventBus.Publish(events.Event{
			Type: events.EventChatMessagesLoaded,
			Data: events.ChatMessagesPayload{
				JID:      jidStr,
				Messages: uiMsgs,
				Append:   true,
			},
		})
	}()
}
