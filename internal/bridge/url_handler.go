package bridge

import (
	"database/sql"
	"net/url"
	"strings"
	"time"

	"whats-gtk/internal/database"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"go.mau.fi/whatsmeow/types"
)

type WhatsAppURLInfo struct {
	Phone string // e.g. "551132434441"
	JID   string // e.g. "551132434441@s.whatsapp.net"
	Text  string // e.g. "Olá! Gostaria de iniciar o atendimento na Sefaz-SP"
}

// ParseWhatsAppURL parses whatsapp://, wa.me, and api.whatsapp.com URLs.
func ParseWhatsAppURL(rawURL string) (*WhatsAppURLInfo, bool) {
	cleanURL := strings.TrimSpace(rawURL)
	if cleanURL == "" {
		return nil, false
	}

	uStr := cleanURL
	if strings.HasPrefix(uStr, "whatsapp://") {
		uStr = "http://" + strings.TrimPrefix(uStr, "whatsapp://")
	}

	u, err := url.Parse(uStr)
	if err != nil {
		return nil, false
	}

	q := u.Query()
	text := q.Get("text")
	phone := q.Get("phone")

	if phone == "" {
		path := strings.TrimPrefix(u.Path, "/")
		if path != "" && path != "send" && path != "send/" {
			parts := strings.Split(path, "/")
			if len(parts) > 0 && parts[0] != "" {
				phone = parts[0]
			}
		}
	}

	var sb strings.Builder
	for _, ch := range phone {
		if ch >= '0' && ch <= '9' {
			sb.WriteRune(ch)
		}
	}
	cleanPhone := sb.String()

	if cleanPhone == "" {
		return nil, false
	}

	jid := cleanPhone + "@s.whatsapp.net"

	return &WhatsAppURLInfo{
		Phone: cleanPhone,
		JID:   jid,
		Text:  text,
	}, true
}

// EnsureContactName attempts to resolve contact name from local DB, whatsmeow store, or WhatsApp server (VerifiedName).
func (cc *ChatController) EnsureContactName(jidStr string, phone string) string {
	parsedJID, err := types.ParseJID(jidStr)
	if err != nil {
		return cc.Contacts.formatPhoneNumber(phone)
	}

	// 1. Check if contact exists in local AppDB with a valid SavedName or PushName
	if c, err := cc.DB.GetContact(jidStr); err == nil {
		if c.SavedName.Valid && c.SavedName.String != "" {
			return c.SavedName.String
		}
		if c.PushName.Valid && c.PushName.String != "" && !strings.HasPrefix(c.PushName.String, "+") {
			return c.PushName.String
		}
	}

	// 2. Check whatsmeow device store contacts
	initialName := ""
	if cc.Backend != nil && cc.Backend.Client != nil && cc.Backend.Client.Store != nil {
		if info, err := cc.Backend.Client.Store.Contacts.GetContact(cc.ctx, parsedJID); err == nil && info.Found {
			if info.FullName != "" {
				initialName = info.FullName
			} else if info.BusinessName != "" {
				initialName = info.BusinessName
			} else if info.PushName != "" {
				initialName = info.PushName
			}
		}
	}

	formattedPhone := cc.Contacts.formatPhoneNumber(phone)
	saveName := initialName
	if saveName == "" {
		saveName = formattedPhone
	}

	_ = cc.DB.SaveContact(database.Contact{
		JID:           jidStr,
		PushName:      sql.NullString{String: saveName, Valid: true},
		LastMessageAt: sql.NullTime{Time: time.Now(), Valid: true},
	})

	// 3. Fetch remote info (VerifiedName / BusinessName / PushName) from WhatsApp server asynchronously
	go func() {
		if cc.Backend == nil || cc.Backend.Client == nil || !cc.Backend.Client.IsConnected() {
			return
		}

		resolvedName := ""

		// Check IsOnWhatsApp for VerifiedName
		isOn, err := cc.Backend.Client.IsOnWhatsApp(cc.ctx, []string{phone})
		if err == nil && len(isOn) > 0 && isOn[0].IsIn {
			if isOn[0].VerifiedName != nil && isOn[0].VerifiedName.Details != nil {
				resolvedName = isOn[0].VerifiedName.Details.GetVerifiedName()
			}
		}

		// Fallback: Check GetUserInfo
		if resolvedName == "" {
			resp, err := cc.Backend.Client.GetUserInfo(cc.ctx, []types.JID{parsedJID})
			if err == nil {
				if ui, ok := resp[parsedJID]; ok {
					if ui.VerifiedName != nil && ui.VerifiedName.Details != nil {
						resolvedName = ui.VerifiedName.Details.GetVerifiedName()
					}
					if !ui.LID.IsEmpty() {
						_ = cc.DB.MergeLID(jidStr, ui.LID.ToNonAD().String())
					}
				}
			}
		}

		if resolvedName != "" {
			_ = cc.DB.SaveContact(database.Contact{
				JID:           jidStr,
				SavedName:     sql.NullString{String: resolvedName, Valid: true},
				LastMessageAt: sql.NullTime{Time: time.Now(), Valid: true},
			})

			glib.IdleAdd(func() {
				if cc.App != nil && cc.App.Sidebar != nil {
					cc.App.Sidebar.UpdateChatRow(jidStr, resolvedName, false, 0, false)
				}
				if cc.App != nil && cc.App.ChatView != nil {
					cc.App.ChatView.SetTopBarInfo(resolvedName, nil)
				}
			})
		}
	}()

	if initialName != "" {
		return initialName
	}
	return formattedPhone
}

// HandleOpenURL processes a WhatsApp URL link, opening the chat and pre-filling input text.
func (cc *ChatController) HandleOpenURL(rawURL string) {
	info, ok := ParseWhatsAppURL(rawURL)
	if !ok {
		return
	}

	go func() {
		contactName := cc.EnsureContactName(info.JID, info.Phone)

		glib.IdleAdd(func() {
			if cc.App != nil && cc.App.Window != nil {
				cc.App.Window.Present()
			}

			cc.HandleChatSelected(info.JID)

			if cc.App.Sidebar != nil {
				cc.App.Sidebar.SelectChat(info.JID)
				if contactName != "" {
					cc.App.Sidebar.UpdateChatRow(info.JID, contactName, false, 0, false)
				}
			}

			if info.Text != "" && cc.App.ChatView != nil {
				cc.App.ChatView.SetInputText(info.Text)
			}
		})
	}()
}
