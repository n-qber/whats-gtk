package bridge

import (
	"context"
	"fmt"
	"sync"
	"whats-gtk/internal/backend"
	"whats-gtk/internal/database"
	"whats-gtk/internal/ui"

	"github.com/gotk3/gotk3/glib"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type Bridge struct {
	Backend     *backend.Backend
	App         *ui.App
	DB          *database.AppDB
	jids        []types.JID
	selectedJID *types.JID
	
	sidebarMutex sync.Mutex
	isSyncing    bool
}

func NewBridge(b *backend.Backend, a *ui.App, db *database.AppDB, ctx context.Context) *Bridge {
	br := &Bridge{
		Backend: b,
		App:     a,
		DB:      db,
	}
	
	a.OnChatSelected = func(index int) {
		if index < len(br.jids) {
			jid := br.jids[index]
			br.selectedJID = &jid
			fmt.Printf("Selected chat: %s\n", jid)
			
			// Load history from DB
			go func() {
				msgs, err := br.DB.GetMessages(jid.String(), 50)
				if err != nil {
					fmt.Printf("Failed to load history: %v\n", err)
					return
				}
				
				glib.IdleAdd(func() {
					br.App.ClearMessages()
					isGroup := jid.Server == types.GroupServer
					
					for _, m := range msgs {
						displayMessage := m.Content
						if isGroup && !m.IsFromMe {
							senderName := m.SenderJID
							contact, err := br.DB.GetContact(m.SenderJID)
							if err == nil {
								senderName = contact.DisplayName()
							}
							displayMessage = fmt.Sprintf("%s: %s", senderName, m.Content)
						}
						br.App.AddMessage(displayMessage, m.IsFromMe)
					}
				})
			}()
		}
	}

	a.OnSendMessage = func(text string) {
		if br.selectedJID != nil {
			go func() {
				_, err := br.Backend.SendText(ctx, *br.selectedJID, text)
				if err != nil {
					fmt.Printf("Failed to send message: %v\n", err)
				}
			}()
		}
	}

	return br
}

func (br *Bridge) Start(ctx context.Context) {
	fmt.Println("Bridge: Starting...")
	br.Backend.SetEventHandler(br.HandleEvent)
	
	err := br.Backend.Connect()
	if err != nil {
		fmt.Printf("Bridge: Failed to connect: %v\n", err)
		return
	}
	fmt.Println("Bridge: Connected to WhatsApp")

	// 1. Initial load from local DB for instant UI
	go func() {
		fmt.Println("Bridge: Attempting initial contact load from DB...")
		contacts, err := br.DB.GetAllContacts(50)
		if err != nil {
			fmt.Printf("Bridge: Error loading initial contacts: %v\n", err)
			return
		}
		fmt.Printf("Bridge: Found %d contacts in local DB\n", len(contacts))
		if len(contacts) > 0 {
			br.refreshSidebar(contacts)
		} else {
			fmt.Println("Bridge: Local DB is empty, waiting for sync...")
		}
	}()

	// 2. Background sync to refresh from WhatsApp
	go func() {
		fmt.Println("Bridge: Starting background sync refresh...")
		// Fetch groups
		groups, err := br.Backend.GetJoinedGroups(ctx)
		if err == nil {
			fmt.Printf("Bridge: Synced %d groups from server\n", len(groups))
			for _, g := range groups {
				br.DB.SaveContact(database.Contact{
					JID:       g.JID.String(),
					SavedName: g.Name,
					IsGroup:   true,
				})
			}
		} else {
			fmt.Printf("Bridge: Failed to sync groups: %v\n", err)
		}

		// Fetch contacts
		contacts, err := br.Backend.GetAllContacts(ctx)
		if err == nil {
			fmt.Printf("Bridge: Synced %d contacts from store\n", len(contacts))
			for jid, info := range contacts {
				br.DB.SaveContact(database.Contact{
					JID:       jid.String(),
					SavedName: info.FullName,
					PushName:  info.PushName,
					IsGroup:   jid.Server == types.GroupServer,
				})
			}
		} else {
			fmt.Printf("Bridge: Failed to sync contacts from store: %v\n", err)
		}
		
		// Refresh UI from updated DB
		contactsDB, err := br.DB.GetAllContacts(50)
		if err == nil {
			fmt.Printf("Bridge: Refreshing UI with %d contacts after sync\n", len(contactsDB))
			br.refreshSidebar(contactsDB)
		} else {
			fmt.Printf("Bridge: Error fetching updated contacts from DB: %v\n", err)
		}
	}()
}

func (br *Bridge) refreshSidebar(contacts []database.Contact) {
	fmt.Printf("Bridge: refreshSidebar called with %d contacts\n", len(contacts))
	br.sidebarMutex.Lock()
	defer br.sidebarMutex.Unlock()
	
	glib.IdleAdd(func() {
		fmt.Println("Bridge: Executing sidebar refresh on main thread...")
		br.App.ClearChats()
		br.jids = nil
		for _, c := range contacts {
			jid, err := types.ParseJID(c.JID)
			if err != nil {
				fmt.Printf("Bridge: Error parsing JID %s: %v\n", c.JID, err)
				continue
			}
			br.jids = append(br.jids, jid)
			prefix := ""
			if c.IsGroup {
				prefix = "[G] "
			}
			br.App.AddChat(prefix + c.DisplayName())
		}
		fmt.Printf("Bridge: Finished sidebar refresh. jids length: %d\n", len(br.jids))
	})
}

func (br *Bridge) HandleEvent(evt backend.AppEvent) {
	// 1. Heavy Persistence / Background Processing
	switch v := evt.(type) {
	case *backend.HistorySyncEvent:
		br.isSyncing = true
		go func() {
			fmt.Printf("Bridge: Processing HistorySync blob...\n")
			for _, conv := range v.Data.Data.GetConversations() {
				chatJID, _ := types.ParseJID(conv.GetID())
				for _, historyMsg := range conv.GetMessages() {
					parsedMsg, err := br.Backend.Client.ParseWebMessage(chatJID, historyMsg.GetMessage())
					if err == nil {
						br.persistMessage(parsedMsg)
					}
				}
			}
			fmt.Printf("Bridge: Finished processing HistorySync blob.\n")
		}()
		return

	case *backend.MessageEvent:
		br.persistMessage(v.Info)
	}

	// 2. UI Updates (Optimized)
	switch v := evt.(type) {
	case *backend.ConnectedEvent:
		glib.IdleAdd(func() { fmt.Println("UI: Connected") })
	
	case *backend.OfflineSyncCompletedEvent:
		br.isSyncing = false
		glib.IdleAdd(func() { fmt.Println("UI: Offline sync completed") })
		// Refresh sidebar one last time after sync is complete
		go func() {
			contacts, err := br.DB.GetAllContacts(50)
			if err == nil {
				br.refreshSidebar(contacts)
			}
		}()

	case *backend.MessageEvent:
		msg := v.Info
		// Only trigger UI update if it's NOT a massive sync OR it's the active chat
		if !br.isSyncing || (br.selectedJID != nil && msg.Info.Chat.String() == br.selectedJID.String()) {
			glib.IdleAdd(func() {
				if br.selectedJID != nil && msg.Info.Chat.String() == br.selectedJID.String() {
					content := br.extractContent(msg)
					if content != "" {
						displayMessage := content
						if msg.Info.Chat.Server == types.GroupServer && !msg.Info.IsFromMe {
							senderName := msg.Info.Sender.String()
							contact, err := br.DB.GetContact(msg.Info.Sender.String())
							if err == nil {
								senderName = contact.DisplayName()
							}
							displayMessage = fmt.Sprintf("%s: %s", senderName, content)
						}
						br.App.AddMessage(displayMessage, msg.Info.IsFromMe)
					}
				}
			})
		}

	case *backend.ReceiptEvent:
		receipt := v.Info
		status := "sent"
		if receipt.Type == types.ReceiptTypeDelivered {
			status = "delivered"
		} else if receipt.Type == types.ReceiptTypeRead || receipt.Type == types.ReceiptTypeReadSelf {
			status = "read"
		}
		for _, id := range receipt.MessageIDs {
			br.DB.UpdateMessageStatus(id, receipt.Chat.String(), status)
		}

	case *backend.IdentityChangeEvent:
		glib.IdleAdd(func() { fmt.Printf("UI: Identity changed for %s\n", v.Info.JID) })
	}
}

func (br *Bridge) persistMessage(msg *events.Message) {
	content := br.extractContent(msg)
	br.DB.SaveMessage(database.Message{
		ID:        msg.Info.ID,
		ChatJID:   msg.Info.Chat.String(),
		SenderJID: msg.Info.Sender.String(),
		Content:   content,
		Type:      "text",
		Timestamp: msg.Info.Timestamp,
		IsFromMe:  msg.Info.IsFromMe,
	})
	br.DB.UpdateContactTimestamp(msg.Info.Chat.String(), msg.Info.Timestamp)
}

func (br *Bridge) extractContent(msg *events.Message) string {
	if msg.Message.GetConversation() != "" {
		return msg.Message.GetConversation()
	} else if msg.Message.GetExtendedTextMessage().GetText() != "" {
		return msg.Message.GetExtendedTextMessage().GetText()
	}
	return ""
}
