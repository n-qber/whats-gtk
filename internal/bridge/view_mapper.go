package bridge

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"whats-gtk/internal/database"
	"whats-gtk/internal/events"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"go.mau.fi/whatsmeow/types"
)

var mentionRegex = regexp.MustCompile(`(?:\s|^)@(\d{8,15})`)

// ViewMapper formats database messages into UI-agnostic events.UIMessage structs.
type ViewMapper struct {
	Contacts *ContactService
	DB       *database.AppDB
	MyJID    string
}

func NewViewMapper(contacts *ContactService, db *database.AppDB, myJID string) *ViewMapper {
	return &ViewMapper{Contacts: contacts, DB: db, MyJID: myJID}
}

func (vm *ViewMapper) FormatMessageDate(t time.Time) string {
	now := time.Now()
	msgDate := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	days := int(today.Sub(msgDate).Hours() / 24)

	if days == 0 {
		return "Hoje"
	} else if days == 1 {
		return "Ontem"
	} else {
		months := []string{"janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho", "agosto", "setembro", "outubro", "novembro", "dezembro"}
		return fmt.Sprintf("%d de %s de %d", t.Day(), months[t.Month()-1], t.Year())
	}
}

// formatMentions escapes text for Pango markup and converts @phonenumber to a styled clickable link.
func (vm *ViewMapper) formatMentions(text string) string {
	escaped := html.EscapeString(text)
	return mentionRegex.ReplaceAllStringFunc(escaped, func(match string) string {
		space := ""
		if strings.HasPrefix(match, " ") || strings.HasPrefix(match, "\n") || strings.HasPrefix(match, "\t") {
			space = match[:1]
			match = match[1:]
		}

		phone := strings.TrimPrefix(match, "@")
		jid := phone + "@s.whatsapp.net"
		name := vm.Contacts.ResolveSenderName(jid)

		if name == jid {
			// Try as LID
			lidJid := phone + "@lid"
			lidName := vm.Contacts.ResolveSenderName(lidJid)
			if lidName != lidJid {
				name = lidName
				jid = lidJid
			} else {
				name = phone // fallback if not found at all
			}
		}

		// Use WhatsApp green (#25D366) and remove underline by default for cleanliness
		markup := fmt.Sprintf(`<a href="mention:%s"><span foreground="#25D366">@%s</span></a>`, jid, name)
		return space + markup
	})
}

func (vm *ViewMapper) MapMessage(m database.Message, sName, tStr string, av *gdk.Texture, isCont bool) events.UIMessage {
	qID := m.QuotedMsgID.String
	qSender := m.QuotedMsgSender.String
	qContent := vm.formatMentions(m.QuotedMsgContent.String)
	qSenderName := qSender
	if qSender != "" {
		qSenderName = vm.Contacts.ResolveSenderName(qSender)
	}

	uiMsg := events.UIMessage{
		ID:              m.ID,
		JID:             m.SenderJID,
		SenderName:      sName,
		IsFromMe:        m.IsFromMe,
		IsContinuation:  isCont,
		TimeString:      tStr,
		Type:            m.Type,
		Content:         vm.formatMentions(m.Content),
		IsViewOnce:      m.IsViewOnce,
		IsForwarded:     m.IsForwarded,
		Status:          m.Status,
		Thumbnail:       m.Thumbnail,
		Avatar:          av,
		QuotedMsgID:     qID,
		QuotedMsgSender: qSenderName,
		QuotedMsgText:   qContent,
	}

	if m.Type == "image" || m.Type == "sticker" || m.Type == "video" {
		if _, err := os.Stat(m.Content); err == nil {
			if m.Type == "sticker" {
				anim, _ := gdkpixbuf.NewPixbufAnimationFromFile(m.Content)
				if anim != nil && anim.IsStaticImage() {
					// We can't map this to Texture without GTK context here, but we will keep the path
					uiMsg.Content = m.Content
				} else {
					uiMsg.StickerAnim = anim
					uiMsg.Content = m.Content
				}
			} else {
				uiMsg.Content = m.Content // It's a valid path
			}
		}
		
		uiMsg.Content = vm.formatMentions(m.Caption.String) // For caption
		if m.Type == "sticker" {
			uiMsg.Content = m.Content // Keep path for sticker
		}

		if m.Type == "video" || m.Type == "image" {
			uiMsg.DocumentPath = ""
			if _, err := os.Stat(m.Content); err == nil {
				uiMsg.DocumentPath = m.Content // Let's use DocumentPath for the media file path
			}
		}

	} else if m.Type == "audio" {
		uiMsg.Content = vm.formatMentions(m.QuotedMsgContent.String)
		if m.Content != "" {
			if _, err := os.Stat(m.Content); err == nil {
				uiMsg.AudioPath = m.Content
			}
		}
	} else if m.Type == "poll" {
		question := "Poll"
		if strings.HasPrefix(m.Content, "[Poll: ") {
			question = strings.TrimSuffix(strings.TrimPrefix(m.Content, "[Poll: "), "]")
		}
		uiMsg.Content = question
		
		optsMap, _ := vm.DB.GetPollOptions(m.ID)
		var opts []string
		for _, name := range optsMap {
			opts = append(opts, name)
		}
		uiMsg.PollOptions = opts
		uiMsg.PollVotes, _ = vm.DB.GetPollVotes(m.ID)
		uiMsg.MyJID = vm.MyJID
	} else if m.Type == "document" {
		fileName := "file"
		if m.Content != "" {
			if strings.HasPrefix(m.Content, "[Document: ") {
				fileName = strings.TrimSuffix(strings.TrimPrefix(m.Content, "[Document: "), "]")
			} else if strings.Contains(m.Content, "media/") {
				base := filepath.Base(m.Content)
				if idx := strings.Index(base, "_"); idx != -1 {
					fileName = base[idx+1:]
				}
			}
		}
		uiMsg.Content = fileName
		if m.Content != "" && !strings.HasPrefix(m.Content, "[Document: ") {
			if _, err := os.Stat(m.Content); err == nil {
				uiMsg.DocumentPath = m.Content
			}
		}
	}

	if m.IsPinned {
		uiMsg.IsPinned = true 
	}
	
	reacts, _ := vm.DB.GetReactions(m.ID)
	if len(reacts) > 0 {
		var uniqueReacts []string
		seenReacts := make(map[string]bool)
		for _, r := range reacts {
			if !seenReacts[r.Reaction] {
				seenReacts[r.Reaction] = true
				uniqueReacts = append(uniqueReacts, r.Reaction)
			}
		}
		uiMsg.Reactions = uniqueReacts
	}

	uiMsg.MediaWidth = int(m.MediaWidth.Int64)
	uiMsg.MediaHeight = int(m.MediaHeight.Int64)

	return uiMsg
}

// MapSidebarItems maps a list of database contacts to UI sidebar items.
func (vm *ViewMapper) MapSidebarItems(contacts []database.Contact) []events.SidebarItem {
	var items []events.SidebarItem
	for _, c := range contacts {
		if c.IsArchived {
			continue
		}
		
		isGroup := c.IsGroup.Valid && c.IsGroup.Bool
		
		jidParsed, _ := types.ParseJID(c.JID)
		avatar := vm.Contacts.GetAvatar(jidParsed.String())
		
		items = append(items, events.SidebarItem{
			JID:         c.JID,
			Name:        c.DisplayName(),
			IsGroup:     isGroup,
			UnreadCount: c.UnreadCount,
			IsPinned:    c.IsPinned,
			Avatar:      avatar,
		})
	}
	return items
}
