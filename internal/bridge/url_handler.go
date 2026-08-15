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

// EnsureContactName attempts to resolve the saved contact name from DB or whatsmeow store.
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
	if cc.Backend != nil && cc.Backend.Client != nil && cc.Backend.Client.Store != nil {
		if info, err := cc.Backend.Client.Store.Contacts.GetContact(cc.ctx, parsedJID); err == nil && info.Found {
			name := info.FullName
			if name == "" {
				name = info.BusinessName
			}
			if name == "" {
				name = info.PushName
			}
			if name != "" {
				_ = cc.DB.SaveContact(database.Contact{
					JID:           jidStr,
					SavedName:     sql.NullString{String: name, Valid: true},
					LastMessageAt: sql.NullTime{Time: time.Now(), Valid: true},
				})
				return name
			}
		}
	}

	// 3. Fallback: Save initial contact with formatted phone number
	formattedPhone := cc.Contacts.formatPhoneNumber(phone)
	_ = cc.DB.SaveContact(database.Contact{
		JID:           jidStr,
		PushName:      sql.NullString{String: formattedPhone, Valid: true},
		LastMessageAt: sql.NullTime{Time: time.Now(), Valid: true},
	})

	// 4. Async fetch from WhatsApp server if connected
	go func() {
		if cc.Backend != nil && cc.Backend.Client != nil && cc.Backend.Client.IsConnected() {
			resp, err := cc.Backend.Client.GetUserInfo(cc.ctx, []types.JID{parsedJID})
			if err == nil {
				if ui, ok := resp[parsedJID]; ok {
					if !ui.LID.IsEmpty() {
						_ = cc.DB.MergeLID(jidStr, ui.LID.ToNonAD().String())
					}
				}
			}
		}
	}()

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
