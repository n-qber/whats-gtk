package bridge

import (
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"whats-gtk/internal/backend"
	"whats-gtk/internal/database"
	"whats-gtk/internal/ui"
	"whats-gtk/internal/ui/chat"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// Renderer handles all UI rendering — message display, sidebar refresh,
// and live message rendering from the pipeline.
type Renderer struct {
	App      *ui.App
	DB       *database.AppDB
	Contacts *ContactService
	Backend  *backend.Backend
	Messages *MessageService
	ctx      context.Context

	OldestMessageTimes map[string]time.Time

	// Chat is set after ChatController is created to provide selectedJID/lastSender access.
	Chat *ChatController
}

// NewRenderer creates a new Renderer.
func NewRenderer(app *ui.App, db *database.AppDB, contacts *ContactService, b *backend.Backend, msgs *MessageService, ctx context.Context) *Renderer {
	return &Renderer{
		App:                app,
		DB:                 db,
		Contacts:           contacts,
		Backend:            b,
		Messages:           msgs,
		ctx:                ctx,
		OldestMessageTimes: make(map[string]time.Time),
	}
}

func formatMessageDate(t time.Time) string {
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

func (r *Renderer) RenderMessageUI(cv *chat.ChatView, m database.Message, sName, tStr string, av *gdk.Texture, isCont bool, updatePinnedBar bool) {
	qID := m.QuotedMsgID.String
	qSender := m.QuotedMsgSender.String
	qContent := r.formatMentions(m.QuotedMsgContent.String)
	qSenderName := qSender
	if qSender != "" {
		qSenderName = r.Contacts.ResolveSenderName(qSender)
	}

	if m.Type == "image" || m.Type == "sticker" || m.Type == "video" {
		var texImg, texThumb *gdk.Texture
		var anim *gdkpixbuf.PixbufAnimation
		texThumb = bytesToTexture(m.Thumbnail)
		
		if _, err := os.Stat(m.Content); err == nil {
			if m.Type == "sticker" {
				anim, _ = gdkpixbuf.NewPixbufAnimationFromFile(m.Content)
				if anim != nil && anim.IsStaticImage() {
					texImg = gdk.NewTextureForPixbuf(anim.StaticImage())
					anim = nil
				}
			} else {
				pixbuf, _ := gdkpixbuf.NewPixbufFromFile(m.Content)
				if pixbuf != nil {
					texImg = gdk.NewTextureForPixbuf(pixbuf)
				}
			}
		} else if m.Type == "sticker" {
			go r.Chat.HandleDownloadMedia(m.ID)
		}
		
		mW := int(m.MediaWidth.Int64); mH := int(m.MediaHeight.Int64)
		caption := r.formatMentions(m.Caption.String)
		
		imgPath := ""
		if _, err := os.Stat(m.Content); err == nil {
			imgPath = m.Content
		}

		if m.Type == "image" {
			cv.AddImage(m.ID, m.SenderJID, sName, caption, texImg, texThumb, imgPath, m.IsFromMe, isCont, m.Status, tStr, av, qID, qSenderName, qContent, mW, mH)
		} else if m.Type == "sticker" {
			cv.AddSticker(m.ID, m.SenderJID, sName, anim, texImg, texThumb, m.IsFromMe, isCont, m.Status, tStr, av, qID, qSenderName, qContent, mW, mH)
		} else if m.Type == "video" {
			cv.AddVideo(m.ID, m.SenderJID, sName, caption, texThumb, imgPath, m.IsFromMe, isCont, m.Status, tStr, av, qID, qSenderName, qContent, mW, mH)
		}
	} else if m.Type == "audio" {
		cv.AddAudio(m.ID, m.SenderJID, sName, m.IsFromMe, isCont, m.Status, tStr, av, qID, qSenderName, qContent)
		if m.Content != "" {
			if _, err := os.Stat(m.Content); err == nil {
				cv.UpdateMessageAudio(m.ID, m.Content)
			}
		}
	} else if m.Type == "poll" {
		question := "Poll"
		if strings.HasPrefix(m.Content, "[Poll: ") {
			question = strings.TrimSuffix(strings.TrimPrefix(m.Content, "[Poll: "), "]")
		}
		optsMap, _ := r.DB.GetPollOptions(m.ID)
		var opts []string
		for _, name := range optsMap {
			opts = append(opts, name)
		}
		votes, _ := r.DB.GetPollVotes(m.ID)
		myJID := r.Backend.Client.Store.ID.ToNonAD().String()
		cv.AddPoll(m.ID, m.SenderJID, sName, question, opts, votes, myJID, m.IsFromMe, isCont, m.Status, tStr, av, qID, qSenderName, qContent)
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
		texThumb := bytesToTexture(m.Thumbnail)
		cv.AddDocument(m.ID, m.SenderJID, sName, fileName, texThumb, m.IsFromMe, isCont, m.Status, tStr, av, qID, qSenderName, qContent)
		if m.Content != "" && !strings.HasPrefix(m.Content, "[Document: ") {
			if _, err := os.Stat(m.Content); err == nil {
				cv.UpdateMessageDocument(m.ID, m.Content)
			}
		}
	} else {
		if m.Content != "" {
			cv.AddMessage(m.ID, m.SenderJID, sName, r.formatMentions(m.Content), m.IsFromMe, isCont, m.Status, tStr, av, qID, qSenderName, qContent)
		}
	}
	
	reacts, _ := r.DB.GetReactions(m.ID)
	if len(reacts) > 0 {
		cv.UpdateMessageReactions(m.ID, uniqueReactions(reacts))
	}

	if m.IsPinned {
		cv.UpdateMessagePinned(m.ID, true)
		if updatePinnedBar {
			cv.SetPinnedMessage(m.Content)
		}
	}
	if m.IsViewOnce {
		if b, exists := cv.MessageRows[m.ID]; exists {
			b.SetViewOnce(true)
		}
	}
}

var mentionRegex = regexp.MustCompile(`(?:\s|^)@(\d{8,15})`)

// formatMentions escapes text for Pango markup and converts @phonenumber to a styled clickable link.
func (r *Renderer) formatMentions(text string) string {
	escaped := html.EscapeString(text)
	return mentionRegex.ReplaceAllStringFunc(escaped, func(match string) string {
		space := ""
		if strings.HasPrefix(match, " ") || strings.HasPrefix(match, "\n") || strings.HasPrefix(match, "\t") {
			space = match[:1]
			match = match[1:]
		}
		
		phone := strings.TrimPrefix(match, "@")
		jid := phone + "@s.whatsapp.net"
		name := r.Contacts.ResolveSenderName(jid)
		
		if name == jid {
			// Try as LID
			lidJid := phone + "@lid"
			lidName := r.Contacts.ResolveSenderName(lidJid)
			if lidName != lidJid {
				name = lidName
				jid = lidJid
			} else {
				name = phone // fallback if not found at all
			}
		}
		
		// Use WhatsApp green (#25D366) and remove underline by default for cleanliness (or keep it if preferred)
		markup := fmt.Sprintf(`<a href="mention:%s"><span foreground="#25D366">@%s</span></a>`, jid, name)
		return space + markup
	})
}

// RefreshMessages loads messages from the database and renders them in the chat view.
func (r *Renderer) RefreshMessages(jid types.JID) {
	go func() {
		jids := []string{jid.ToNonAD().String()}
		if contact, err := r.DB.GetContact(jid.ToNonAD().String()); err == nil {
			if contact.LID.Valid && contact.LID.String != "" {
				jids = append(jids, contact.LID.String)
			}
		}

		msgs, err := r.DB.GetMessages(jids, 50)
		if err != nil {
			fmt.Printf("Bridge: GetMessages failed: %v\n", err)
			return
		}
		seen := make(map[string]bool)
		for i := len(msgs) - 1; i >= 0; i-- {
			if !msgs[i].IsFromMe && !seen[msgs[i].SenderJID] {
				r.Contacts.GetAvatar(msgs[i].SenderJID)
				seen[msgs[i].SenderJID] = true
			}
		}
		
		if len(msgs) > 0 {
			r.OldestMessageTimes[jid.ToNonAD().String()] = msgs[0].Timestamp
		}

		glib.IdleAdd(func() {
			cv := r.App.GetChatViewForJID(jid.ToNonAD().String())
			if cv == nil { return }
			cv.Clear(); r.Chat.SetLastSender(""); r.Chat.SetLastDateStr("")
			for _, m := range msgs {
				dateStr := formatMessageDate(m.Timestamp)
				if dateStr != r.Chat.LastDateStr() {
					cv.AddSeparator(dateStr)
					r.Chat.SetLastDateStr(dateStr)
				}
				
				tStr := m.Timestamp.Format("15:04"); sName := ""; var av *gdk.Texture; isCont := m.SenderJID == r.Chat.LastSender()
				if jid.Server == types.GroupServer && !m.IsFromMe {
					if !isCont {
						sName = r.Contacts.ResolveSenderName(m.SenderJID)
						av = r.Contacts.GetAvatar(m.SenderJID)
					}
				}
				r.Chat.SetLastSender(m.SenderJID)
				r.RenderMessageUI(cv, m, sName, tStr, av, isCont, true)
			}
			cv.ScrollToBottom()
		})
	}()
}

// LoadOlderMessages fetches older messages for a chat when scrolled to the top.
func (r *Renderer) LoadOlderMessages(jidStr string, cv *chat.ChatView, targetID string) {
	go func() {
		before, ok := r.OldestMessageTimes[jidStr]
		if !ok {
			glib.IdleAdd(func() { cv.SetLoadingOlder(false) })
			return
		}

		jids := []string{jidStr}
		if contact, err := r.DB.GetContact(jidStr); err == nil {
			if contact.LID.Valid && contact.LID.String != "" {
				jids = append(jids, contact.LID.String)
			}
		}

		var msgs []database.Message
		var err error
		if targetID == "" {
			msgs, err = r.DB.GetOlderMessages(jids, before, 50)
		} else {
			targetMsg, e := r.DB.GetMessage(targetID)
			if e != nil || !targetMsg.Timestamp.Before(before) {
				glib.IdleAdd(func() { cv.SetLoadingOlder(false) })
				return
			}
			msgs, err = r.DB.GetMessagesBetween(jids, targetMsg.Timestamp, before, 300)
		}

		if err != nil || len(msgs) == 0 {
			glib.IdleAdd(func() { cv.SetLoadingOlder(false) })
			return
		}

		r.OldestMessageTimes[jidStr] = msgs[0].Timestamp

		seen := make(map[string]bool)
		for i := len(msgs) - 1; i >= 0; i-- {
			if !msgs[i].IsFromMe && !seen[msgs[i].SenderJID] {
				r.Contacts.GetAvatar(msgs[i].SenderJID)
				seen[msgs[i].SenderJID] = true
			}
		}

		glib.IdleAdd(func() {
			adj := cv.MessageScrolledWindow.VAdjustment()
			oldMax := adj.Upper()
			
			cv.InsertIndex = 0
			var lastDateStr string

			for _, m := range msgs {
				dateStr := formatMessageDate(m.Timestamp)
				if dateStr != lastDateStr {
					cv.AddSeparator(dateStr)
					lastDateStr = dateStr
				}
				
				tStr := m.Timestamp.Format("15:04"); sName := ""; var av *gdk.Texture; isCont := false
				targetJID, _ := types.ParseJID(jidStr)
				if targetJID.Server == types.GroupServer && !m.IsFromMe {
					sName = r.Contacts.ResolveSenderName(m.SenderJID)
					av = r.Contacts.GetAvatar(m.SenderJID)
				}
				
				r.RenderMessageUI(cv, m, sName, tStr, av, isCont, false)
			}
			
			cv.InsertIndex = -1 // Reset
			
			// Adjust scroll to prevent jump
			glib.TimeoutAdd(50, func() bool {
				newMax := adj.Upper()
				if newMax > oldMax {
					adj.SetValue(newMax - oldMax)
				}
				if targetID != "" {
					cv.ScrollToMessage(targetID)
				}
				cv.SetLoadingOlder(false)
				return false
			})
		})
	}()
}

// RenderMessageSearch fetches and renders messages matching the search query in the current chat.
func (r *Renderer) RenderMessageSearch(jidStr string, query string) {
	go func() {
		jids := []string{jidStr}
		if contact, err := r.DB.GetContact(jidStr); err == nil {
			if contact.LID.Valid && contact.LID.String != "" {
				jids = append(jids, contact.LID.String)
			}
		}

		msgs, err := r.DB.SearchMessagesInChat(jids, query, 100)
		if err != nil {
			fmt.Printf("Bridge: SearchMessagesInChat failed: %v\n", err)
			return
		}

		seen := make(map[string]bool)
		for i := len(msgs) - 1; i >= 0; i-- {
			if !msgs[i].IsFromMe && !seen[msgs[i].SenderJID] {
				r.Contacts.GetAvatar(msgs[i].SenderJID)
				seen[msgs[i].SenderJID] = true
			}
		}

		glib.IdleAdd(func() {
			cv := r.App.GetChatViewForJID(jidStr)
			if cv == nil { return }
			cv.Clear(); r.Chat.SetLastSender(""); r.Chat.SetLastDateStr("")
			cv.IsSearching = true

			for _, m := range msgs {
				dateStr := formatMessageDate(m.Timestamp)
				if dateStr != r.Chat.LastDateStr() {
					cv.AddSeparator(dateStr)
					r.Chat.SetLastDateStr(dateStr)
				}
				
				tStr := m.Timestamp.Format("15:04"); sName := ""; var av *gdk.Texture; isCont := m.SenderJID == r.Chat.LastSender()
				targetJID, _ := types.ParseJID(jidStr)
				if targetJID.Server == types.GroupServer && !m.IsFromMe {
					if !isCont {
						sName = r.Contacts.ResolveSenderName(m.SenderJID)
						av = r.Contacts.GetAvatar(m.SenderJID)
					}
				}
				r.Chat.SetLastSender(m.SenderJID)
				r.RenderMessageUI(cv, m, sName, tStr, av, isCont, false)
			}
			cv.ScrollToBottom()
		})
	}()
}

// RefreshSidebar rebuilds the sidebar chat list from the given contacts.
func (r *Renderer) RefreshSidebar(contacts []database.Contact) {
	glib.IdleAdd(func() {
		r.App.Sidebar.SetRefreshing(true)
		r.App.Sidebar.ClearChats()
		for _, c := range contacts {
			if c.IsArchived {
				continue // Skip archived chats for now, or just show them? Let's show them, no wait, hiding them is better for now unless searched. Let's just pass the info. Actually, if they are archived, they shouldn't clutter the main view. We'll leave them in for now.
			}
			isGroup := c.IsGroup.Valid && c.IsGroup.Bool
			r.App.Sidebar.AddChat(c.JID, c.DisplayName(), isGroup, c.UnreadCount, c.IsPinned)
			
			if tex := r.Contacts.GetAvatarNoFetch(c.JID); tex != nil {
				r.App.Sidebar.SetAvatar(c.JID, tex)
			}
		}
		
		selectedJID := r.Chat.SelectedJID()
		if selectedJID != nil {
			r.App.Sidebar.SelectChat(selectedJID.ToNonAD().String())
		}
		r.App.Sidebar.SetRefreshing(false)
	})
}

// RenderLiveMessage renders an incoming live message from the pipeline to the UI.
// It handles sidebar reordering, auto-read marking, and message rendering.
func (r *Renderer) RenderLiveMessage(msg *events.Message, isSyncing bool) {
	resolvedChat := r.Messages.ResolveJID(msg.Info.Chat)
	selectedJID := r.Chat.SelectedJID()
	if !isSyncing || (selectedJID != nil && resolvedChat.ToNonAD().String() == selectedJID.ToNonAD().String()) {
		jid := resolvedChat.ToNonAD().String()

		// Automatically mark as read if this chat is currently selected
		if selectedJID != nil && jid == selectedJID.ToNonAD().String() {
			if r.App.Window.IsActive() {
				go r.Backend.MarkRead(r.ctx, msg.Info.Chat, []string{msg.Info.ID}, msg.Info.Sender, time.Now())
			}
		}

		glib.IdleAdd(func() {
			r.App.Sidebar.MoveChatToTop(jid)
			cv := r.App.GetChatViewForJID(resolvedChat.ToNonAD().String())
			if cv != nil {
				dateStr := formatMessageDate(msg.Info.Timestamp)
				if dateStr != r.Chat.LastDateStr() {
					cv.AddSeparator(dateStr)
					r.Chat.SetLastDateStr(dateStr)
				}
				
				tStr := msg.Info.Timestamp.Format("15:04"); sName := ""; var av *gdk.Texture; isG := msg.Info.Chat.Server == types.GroupServer
				
				resolvedSender := r.Messages.ResolveJID(msg.Info.Sender)
				sJID := resolvedSender.ToNonAD().String()
				isCont := sJID == r.Chat.LastSender()
				
				if isG && !msg.Info.IsFromMe {
					if !isCont {
						sName = r.Contacts.ResolveSenderName(sJID)
						av = r.Contacts.GetAvatar(sJID)
					}
				}
				r.Chat.SetLastSender(sJID)
				
				var qID, qSender, qContent string
				var qSenderName string
				if ci := r.Messages.ExtractContextInfo(msg); ci != nil && ci.GetStanzaID() != "" {
					qID = ci.GetStanzaID()
					qSender = ci.GetParticipant()
					if qSender != "" {
						qSenderName = r.Contacts.ResolveSenderName(qSender)
					}
					if qm := ci.GetQuotedMessage(); qm != nil {
						if qm.GetConversation() != "" {
							qContent = qm.GetConversation()
						} else if qm.GetExtendedTextMessage() != nil {
							qContent = qm.GetExtendedTextMessage().GetText()
						} else {
							qContent = "[Quoted Media]"
						}
					}
				}
				
				protoMsg, isViewOnce := r.Messages.UnwrapMessage(msg.Message)
				
				var mW, mH int
				if img := protoMsg.GetImageMessage(); img != nil {
					mW = int(img.GetWidth()); mH = int(img.GetHeight())
					texThumb := bytesToTexture(img.GetJPEGThumbnail())
					cv.AddImage(msg.Info.ID, sJID, sName, r.formatMentions(img.GetCaption()), nil, texThumb, "", msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, r.formatMentions(qContent), mW, mH)
				} else if stkr := protoMsg.GetStickerMessage(); stkr != nil {
					mW = int(stkr.GetWidth()); mH = int(stkr.GetHeight())
					texThumb := bytesToTexture(stkr.GetPngThumbnail())
					cv.AddSticker(msg.Info.ID, sJID, sName, nil, nil, texThumb, msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent, mW, mH)
				} else if vid := protoMsg.GetVideoMessage(); vid != nil {
					mW = int(vid.GetWidth()); mH = int(vid.GetHeight())
					texThumb := bytesToTexture(vid.GetJPEGThumbnail())
					cv.AddVideo(msg.Info.ID, sJID, sName, r.formatMentions(vid.GetCaption()), texThumb, "", msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, r.formatMentions(qContent), mW, mH)
				} else if aud := protoMsg.GetAudioMessage(); aud != nil {
					cv.AddAudio(msg.Info.ID, sJID, sName, msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent)
				} else if doc := protoMsg.GetDocumentMessage(); doc != nil {
					texThumb := bytesToTexture(doc.GetJPEGThumbnail())
					cv.AddDocument(msg.Info.ID, sJID, sName, doc.GetFileName(), texThumb, msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent)
				} else if poll := r.Messages.GetPollCreationMessage(protoMsg); poll != nil {
					var opts []string
					for _, o := range poll.GetOptions() {
						opts = append(opts, o.GetOptionName())
					}
					cv.AddPoll(msg.Info.ID, sJID, sName, poll.GetName(), opts, nil, "", msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent)
				} else {
					content := r.Messages.ExtractContent(msg)
					if content != "" {
						cv.AddMessage(msg.Info.ID, sJID, sName, r.formatMentions(content), msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, r.formatMentions(qContent))
					}
				}
				if isViewOnce {
					if b, exists := cv.MessageRows[msg.Info.ID]; exists {
						b.SetViewOnce(true)
					}
				}
				cv.ScrollToBottom()
			}
		})
	}
}

// CancelMessageSearch clears the search state and restores the original chat messages.
func (r *Renderer) CancelMessageSearch(jidStr string) {
	glib.IdleAdd(func() {
		cv := r.App.GetChatViewForJID(jidStr)
		if cv != nil {
			cv.IsSearching = false
		}
	})
	targetJID, _ := types.ParseJID(jidStr)
	r.RefreshMessages(targetJID)
}
