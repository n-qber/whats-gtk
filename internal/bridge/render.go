package bridge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"whats-gtk/internal/backend"
	"whats-gtk/internal/database"
	"whats-gtk/internal/ui"

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

	// Chat is set after ChatController is created to provide selectedJID/lastSender access.
	Chat *ChatController
}

// NewRenderer creates a new Renderer.
func NewRenderer(app *ui.App, db *database.AppDB, contacts *ContactService, b *backend.Backend, msgs *MessageService, ctx context.Context) *Renderer {
	return &Renderer{
		App:      app,
		DB:       db,
		Contacts: contacts,
		Backend:  b,
		Messages: msgs,
		ctx:      ctx,
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
				qID := m.QuotedMsgID.String; qSender := m.QuotedMsgSender.String; qContent := m.QuotedMsgContent.String
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
						// Auto-download missing stickers
						go r.Chat.HandleDownloadMedia(m.ID)
					}
					
					mW := int(m.MediaWidth.Int64); mH := int(m.MediaHeight.Int64)
					caption := m.Caption.String
					
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
				} else if m.Type == "document" {
					fileName := "file"
					if m.Content != "" {
						if strings.HasPrefix(m.Content, "[Document: ") {
							fileName = strings.TrimSuffix(strings.TrimPrefix(m.Content, "[Document: "), "]")
						} else if strings.Contains(m.Content, "media/") {
							// Extract original name from saved path: media/ID_FileName.ext
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
						cv.AddMessage(m.ID, m.SenderJID, sName, m.Content, m.IsFromMe, isCont, m.Status, tStr, av, qID, qSenderName, qContent)
					}
				}
				
				// Set reactions
				reacts, _ := r.DB.GetReactions(m.ID)
				if len(reacts) > 0 {
					cv.UpdateMessageReactions(m.ID, uniqueReactions(reacts))
				}

				if m.IsPinned {
					cv.UpdateMessagePinned(m.ID, true)
					cv.SetPinnedMessage(m.Content)
				}
				if m.IsViewOnce {
					if b, exists := cv.MessageRows[m.ID]; exists {
						b.SetViewOnce(true)
					}
				}
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
			prefix := ""
			if c.IsGroup.Valid && c.IsGroup.Bool { prefix = "[G] " }
			r.App.Sidebar.AddChat(c.JID, prefix+c.DisplayName())
			
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
					cv.AddImage(msg.Info.ID, sJID, sName, img.GetCaption(), nil, texThumb, "", msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent, mW, mH)
				} else if stkr := protoMsg.GetStickerMessage(); stkr != nil {
					mW = int(stkr.GetWidth()); mH = int(stkr.GetHeight())
					texThumb := bytesToTexture(stkr.GetPngThumbnail())
					cv.AddSticker(msg.Info.ID, sJID, sName, nil, nil, texThumb, msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent, mW, mH)
				} else if vid := protoMsg.GetVideoMessage(); vid != nil {
					mW = int(vid.GetWidth()); mH = int(vid.GetHeight())
					texThumb := bytesToTexture(vid.GetJPEGThumbnail())
					cv.AddVideo(msg.Info.ID, sJID, sName, vid.GetCaption(), texThumb, "", msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent, mW, mH)
				} else if aud := protoMsg.GetAudioMessage(); aud != nil {
					cv.AddAudio(msg.Info.ID, sJID, sName, msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent)
				} else if doc := protoMsg.GetDocumentMessage(); doc != nil {
					texThumb := bytesToTexture(doc.GetJPEGThumbnail())
					cv.AddDocument(msg.Info.ID, sJID, sName, doc.GetFileName(), texThumb, msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent)
				} else if poll := protoMsg.GetPollCreationMessage(); poll != nil {
					var opts []string
					for _, o := range poll.GetOptions() {
						opts = append(opts, o.GetOptionName())
					}
					cv.AddPoll(msg.Info.ID, sJID, sName, poll.GetName(), opts, msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent)
				} else {
					content := r.Messages.ExtractContent(msg)
					if content != "" {
						cv.AddMessage(msg.Info.ID, sJID, sName, content, msg.Info.IsFromMe, isCont, "", tStr, av, qID, qSenderName, qContent)
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
