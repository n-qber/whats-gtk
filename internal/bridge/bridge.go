package bridge

import (
	"context"
	"fmt"
	"whats-gtk/internal/backend"
	"whats-gtk/internal/ui"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"go.mau.fi/whatsmeow/types"
)

type Bridge struct {
	Backend     *backend.Backend
	App         *ui.App
	jids        []types.JID
	selectedJID *types.JID
}

func NewBridge(b *backend.Backend, a *ui.App, ctx context.Context) *Bridge {
	br := &Bridge{
		Backend: b,
		App:     a,
	}
	
	a.OnChatSelected = func(index int) {
		if index < len(br.jids) {
			jid := br.jids[index]
			br.selectedJID = &jid
			fmt.Printf("Selected chat: %s\n", jid)
			glib.IdleAdd(func() {
				br.App.ClearMessages()
			})
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

	// Fetch contacts in a background goroutine
	go func() {
		contacts, err := br.Backend.GetAllContacts(ctx)
		if err != nil {
			fmt.Printf("Failed to fetch contacts: %v\n", err)
			return
		}
		
		glib.IdleAdd(func() {
			for jid, info := range contacts {
				name := info.FullName
				if name == "" {
					name = info.PushName
				}
				if name == "" {
					name = jid.User
				}
				br.jids = append(br.jids, jid)
				br.App.AddChat(name)
			}
		})
	}()
}

func (br *Bridge) HandleEvent(evt backend.AppEvent) {
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

			text := ""
			if msg.Message.GetConversation() != "" {
				text = msg.Message.GetConversation()
			} else if msg.Message.GetExtendedTextMessage().GetText() != "" {
				text = msg.Message.GetExtendedTextMessage().GetText()
			}
			
			if text != "" {
				sender := msg.Info.Sender.String()
				br.App.AddMessage(fmt.Sprintf("%s: %s", sender, text), msg.Info.IsFromMe)
			}
		default:
			fmt.Printf("UI received unhandled event: %T\n", evt)
		}
	})
}
