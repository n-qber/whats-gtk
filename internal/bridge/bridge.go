package bridge

import (
	"context"
	"fmt"
	"whats-gtk/internal/backend"
	"whats-gtk/internal/database"
	"whats-gtk/internal/ui"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"go.mau.fi/whatsmeow/types"
)

type Bridge struct {
	Backend     *backend.Backend
	App         *ui.App
	DB          *database.AppDB
	jids        []types.JID
	selectedJID *types.JID
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
					for _, m := range msgs {
						sender := m.SenderJID
						if m.IsFromMe {
							sender = "Me"
						}
						br.App.AddMessage(fmt.Sprintf("%s: %s", sender, m.Content), m.IsFromMe)
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
	br.Backend.SetEventHandler(br.HandleEvent)
	
	err := br.Backend.Connect()
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}

	// Fetch contacts and groups in a background goroutine
	go func() {
		addedJIDs := make(map[string]bool)

		// 1. Fetch joined groups from server
		groups, err := br.Backend.GetJoinedGroups(ctx)
		if err == nil {
			glib.IdleAdd(func() {
				for _, g := range groups {
					if addedJIDs[g.JID.String()] {
						continue
					}
					br.jids = append(br.jids, g.JID)
					br.App.AddChat(fmt.Sprintf("[G] %s", g.Name))
					addedJIDs[g.JID.String()] = true
					
					// Persist to DB
					br.DB.SaveContact(database.Contact{
						JID:     g.JID.String(),
						Name:    g.Name,
						IsGroup: true,
					})
				}
			})
		}

		// 2. Fetch all contacts from store
		contacts, err := br.Backend.GetAllContacts(ctx)
		if err != nil {
			fmt.Printf("Failed to fetch contacts: %v\n", err)
			return
		}
		
		glib.IdleAdd(func() {
			for jid, info := range contacts {
				if addedJIDs[jid.String()] {
					continue
				}
				name := info.FullName
				if name == "" {
					name = info.PushName
				}
				if name == "" {
					name = jid.User
				}
				
				prefix := ""
				if jid.Server == types.GroupServer {
					prefix = "[G] "
				}
				
				br.jids = append(br.jids, jid)
				br.App.AddChat(prefix + name)
				addedJIDs[jid.String()] = true

				// Persist to DB
				br.DB.SaveContact(database.Contact{
					JID:      jid.String(),
					Name:     name,
					PushName: info.PushName,
					IsGroup:  jid.Server == types.GroupServer,
				})
			}
		})
	}()
}

func (br *Bridge) HandleEvent(evt backend.AppEvent) {
	// 1. Persistence & Heavy Processing (Background)
	switch v := evt.(type) {
	case *backend.HistorySyncEvent:
		go func() {
			for _, conv := range v.Data.Data.GetConversations() {
				chatJID, _ := types.ParseJID(conv.GetID())
				for _, historyMsg := range conv.GetMessages() {
					evt, err := br.Backend.Client.ParseWebMessage(chatJID, historyMsg.GetMessage())
					if err == nil {
						br.HandleEvent(&backend.MessageEvent{Info: evt})
					}
				}
			}
		}()
		return

	case *backend.MessageEvent:
		msg := v.Info
		content := ""
		if msg.Message.GetConversation() != "" {
			content = msg.Message.GetConversation()
		} else if msg.Message.GetExtendedTextMessage().GetText() != "" {
			content = msg.Message.GetExtendedTextMessage().GetText()
		}

		br.DB.SaveMessage(database.Message{
			ID:        msg.Info.ID,
			ChatJID:   msg.Info.Chat.String(),
			SenderJID: msg.Info.Sender.String(),
			Content:   content,
			Type:      "text",
			Timestamp: msg.Info.Timestamp,
			IsFromMe:  msg.Info.IsFromMe,
		})
	}

	// 2. UI Updates (Main Thread)
	glib.IdleAdd(func() {
		switch v := evt.(type) {
		case *backend.ConnectedEvent:
			fmt.Println("UI: Connected")
		case *backend.MessageEvent:
			msg := v.Info
			if br.selectedJID == nil || msg.Info.Chat.String() != br.selectedJID.String() {
				return
			}

			// Handle Polls
			if pollCreation := msg.Message.GetPollCreationMessage(); pollCreation != nil {
				question := pollCreation.GetName()
				var options []string
				for _, opt := range pollCreation.GetOptions() {
					options = append(options, opt.GetOptionName())
				}
				br.App.AddPoll(question, options, msg.Info.IsFromMe)
				return
			}

			// Handle Media (Simplified)
			if msg.Message.GetImageMessage() != nil || msg.Message.GetStickerMessage() != nil {
				go func() {
					data, err := br.Backend.DownloadMedia(context.Background(), msg)
					if err != nil {
						fmt.Printf("Failed to download media: %v\n", err)
						return
					}
					
					glib.IdleAdd(func() {
						loader, _ := gdk.PixbufLoaderNew()
						loader.Write(data)
						loader.Close()
						pixbuf, _ := loader.GetPixbuf()
						if pixbuf != nil {
							if msg.Message.GetStickerMessage() != nil {
								br.App.AddSticker(pixbuf, msg.Info.IsFromMe)
							} else {
								br.App.AddImage(pixbuf, msg.Info.IsFromMe)
							}
						}
					})
				}()
				return
			}

			// Handle Audio
			if msg.Message.GetAudioMessage() != nil {
				br.App.AddAudio(msg.Info.IsFromMe)
				return
			}

			// Handle Text
			content := ""
			if msg.Message.GetConversation() != "" {
				content = msg.Message.GetConversation()
			} else if msg.Message.GetExtendedTextMessage().GetText() != "" {
				content = msg.Message.GetExtendedTextMessage().GetText()
			}
			
			if content != "" {
				sender := msg.Info.Sender.String()
				br.App.AddMessage(fmt.Sprintf("%s: %s", sender, content), msg.Info.IsFromMe)
			}
		default:
			fmt.Printf("UI received unhandled event: %T\n", evt)
		}
	})
}
