package bridge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"whats-gtk/internal/backend"
	"whats-gtk/internal/database"
	"whats-gtk/internal/paths"
	"whats-gtk/internal/ui"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// MessageService handles message persistence, content extraction from protobuf,
// reaction handling, pin processing, and JID resolution.
type MessageService struct {
	DB       *database.AppDB
	Backend  *backend.Backend
	Contacts *ContactService
	App      *ui.App
	ctx      context.Context

	// GetSelectedJID is set after ChatController is created to avoid circular init.
	GetSelectedJID func() *types.JID
}

// NewMessageService creates a new MessageService.
func NewMessageService(db *database.AppDB, b *backend.Backend, contacts *ContactService, app *ui.App, ctx context.Context) *MessageService {
	return &MessageService{
		DB:       db,
		Backend:  b,
		Contacts: contacts,
		App:      app,
		ctx:      ctx,
	}
}

// ResolveJID resolves LID JIDs to phone number JIDs via whatsmeow store and DB lookup.
func (ms *MessageService) ResolveJID(jid types.JID) types.JID {
	jStr := jid.ToNonAD().String()
	if strings.HasSuffix(jStr, "@lid") {
		if ms.Backend != nil && ms.Backend.Client != nil && ms.Backend.Client.Store != nil && ms.Backend.Client.Store.LIDs != nil {
			if pn, err := ms.Backend.Client.Store.LIDs.GetPNForLID(ms.ctx, jid.ToNonAD()); err == nil && !pn.IsEmpty() {
				return pn.ToNonAD()
			}
		}
		if c, err := ms.DB.GetContact(jStr); err == nil && !strings.HasSuffix(c.JID, "@lid") {
			if parsed, err := types.ParseJID(c.JID); err == nil {
				return parsed.ToNonAD()
			}
		}
		if target, err := ms.DB.GetContactByLID(jStr); err == nil && !strings.HasSuffix(target.JID, "@lid") {
			if parsed, err := types.ParseJID(target.JID); err == nil {
				return parsed.ToNonAD()
			}
		}
	}
	return jid.ToNonAD()
}

// ResolveJIDString resolves a JID string to its canonical PN representation if possible.
func (ms *MessageService) ResolveJIDString(jStr string) string {
	if jStr == "" {
		return ""
	}
	parsed, err := types.ParseJID(jStr)
	if err != nil {
		return jStr
	}
	return ms.ResolveJID(parsed).String()
}

// UnwrapMessage recursively removes wrapper messages (ViewOnce, Ephemeral, DeviceSent, etc.)
// and returns the underlying message and whether it was marked ViewOnce.
func (ms *MessageService) UnwrapMessage(msg *waProto.Message) (*waProto.Message, bool) {
	if msg == nil {
		return nil, false
	}
	isViewOnce := false
	current := msg
	for {
		if vo := current.GetViewOnceMessage(); vo != nil && vo.GetMessage() != nil {
			isViewOnce = true
			current = vo.GetMessage()
			continue
		}
		if vo2 := current.GetViewOnceMessageV2(); vo2 != nil && vo2.GetMessage() != nil {
			isViewOnce = true
			current = vo2.GetMessage()
			continue
		}
		if vo2e := current.GetViewOnceMessageV2Extension(); vo2e != nil && vo2e.GetMessage() != nil {
			isViewOnce = true
			current = vo2e.GetMessage()
			continue
		}
		if eph := current.GetEphemeralMessage(); eph != nil && eph.GetMessage() != nil {
			current = eph.GetMessage()
			continue
		}
		if ds := current.GetDeviceSentMessage(); ds != nil && ds.GetMessage() != nil {
			current = ds.GetMessage()
			continue
		}
		if docCap := current.GetDocumentWithCaptionMessage(); docCap != nil && docCap.GetMessage() != nil {
			current = docCap.GetMessage()
			continue
		}
		if bot := current.GetBotInvokeMessage(); bot != nil && bot.GetMessage() != nil {
			current = bot.GetMessage()
			continue
		}
		if gm := current.GetGroupMentionedMessage(); gm != nil && gm.GetMessage() != nil {
			current = gm.GetMessage()
			continue
		}
		if ed := current.GetEditedMessage(); ed != nil && ed.GetMessage() != nil {
			current = ed.GetMessage()
			continue
		}
		break
	}
	return current, isViewOnce
}

// ExtractContentFromProto extracts the text content from a waProto.Message
func (ms *MessageService) ExtractContentFromProto(protoMsg *waProto.Message) string {
	protoMsg, _ = ms.UnwrapMessage(protoMsg)
	if protoMsg == nil {
		return ""
	}
	if protoMsg.GetConversation() != "" {
		return protoMsg.GetConversation()
	}
	if protoMsg.GetExtendedTextMessage().GetText() != "" {
		return protoMsg.GetExtendedTextMessage().GetText()
	}
	if protoMsg.GetImageMessage().GetCaption() != "" {
		return protoMsg.GetImageMessage().GetCaption()
	}
	if protoMsg.GetVideoMessage().GetCaption() != "" {
		return protoMsg.GetVideoMessage().GetCaption()
	}
	if protoMsg.GetPtvMessage().GetCaption() != "" {
		return protoMsg.GetPtvMessage().GetCaption()
	}
	if protoMsg.GetDocumentMessage().GetCaption() != "" {
		return protoMsg.GetDocumentMessage().GetCaption()
	}

	if protoMsg.GetAudioMessage() != nil {
		return "[Audio Message]"
	}
	if protoMsg.GetDocumentMessage() != nil {
		if protoMsg.GetDocumentMessage().GetFileName() != "" {
			return fmt.Sprintf("[Document: %s]", protoMsg.GetDocumentMessage().GetFileName())
		}
		return "[Document]"
	}
	if pm := ms.GetPollCreationMessage(protoMsg); pm != nil {
		return fmt.Sprintf("[Poll: %s]", pm.GetName())
	}
	if protoMsg.GetContactMessage() != nil {
		return fmt.Sprintf("[Contact: %s]", protoMsg.GetContactMessage().GetDisplayName())
	}
	if protoMsg.GetContactsArrayMessage() != nil {
		return fmt.Sprintf("[Contacts: %s]", protoMsg.GetContactsArrayMessage().GetDisplayName())
	}
	if protoMsg.GetLocationMessage() != nil {
		return "[Location]"
	}
	if protoMsg.GetLiveLocationMessage() != nil {
		return "[Live Location]"
	}
	if im := protoMsg.GetInteractiveMessage(); im != nil {
		return formatInteractiveMessage(im)
	}
	if tm := protoMsg.GetTemplateMessage(); tm != nil {
		return formatTemplateMessage(tm)
	}
	if hsm := protoMsg.GetHighlyStructuredMessage(); hsm != nil {
		text := extractHighlyStructuredMessageText(hsm)
		if text != "" {
			return text
		}
		return "[Template Message]"
	}
	if bm := protoMsg.GetButtonsMessage(); bm != nil {
		return formatButtonsMessage(bm)
	}
	if lm := protoMsg.GetListMessage(); lm != nil {
		return formatListMessage(lm)
	}
	if tbrm := protoMsg.GetTemplateButtonReplyMessage(); tbrm != nil {
		if tbrm.GetSelectedDisplayText() != "" {
			return tbrm.GetSelectedDisplayText()
		}
		if tbrm.GetSelectedID() != "" {
			return tbrm.GetSelectedID()
		}
	}
	if brm := protoMsg.GetButtonsResponseMessage(); brm != nil {
		if brm.GetSelectedDisplayText() != "" {
			return brm.GetSelectedDisplayText()
		}
		if brm.GetSelectedButtonID() != "" {
			return brm.GetSelectedButtonID()
		}
	}
	if irm := protoMsg.GetInteractiveResponseMessage(); irm != nil {
		if irm.GetBody() != nil && irm.GetBody().GetText() != "" {
			return irm.GetBody().GetText()
		}
		if nf := irm.GetNativeFlowResponseMessage(); nf != nil {
			var params struct {
				DisplayText string `json:"display_text"`
			}
			if err := json.Unmarshal([]byte(nf.GetParamsJSON()), &params); err == nil && params.DisplayText != "" {
				return params.DisplayText
			}
			if nf.GetName() != "" {
				return nf.GetName()
			}
		}
	}
	if lrm := protoMsg.GetListResponseMessage(); lrm != nil {
		if lrm.GetTitle() != "" {
			return lrm.GetTitle()
		}
		if lrm.GetSingleSelectReply() != nil && lrm.GetSingleSelectReply().GetSelectedRowID() != "" {
			return lrm.GetSingleSelectReply().GetSelectedRowID()
		}
	}
	if gi := protoMsg.GetGroupInviteMessage(); gi != nil {
		return fmt.Sprintf("[Group Invite: %s]", gi.GetGroupName())
	}
	if ev := protoMsg.GetEventMessage(); ev != nil {
		return fmt.Sprintf("[Event: %s]", ev.GetName())
	}

	return ""
}

type nativeFlowButtonParams struct {
	DisplayText string `json:"display_text"`
	Title       string `json:"title"`
	CopyCode    string `json:"copy_code"`
	URL         string `json:"url"`
	PhoneNumber string `json:"phone_number"`
	ID          string `json:"id"`
}

func formatNativeFlowButton(btn *waProto.InteractiveMessage_NativeFlowMessage_NativeFlowButton) string {
	if btn == nil {
		return ""
	}
	var params nativeFlowButtonParams
	if btn.GetButtonParamsJSON() != "" {
		_ = json.Unmarshal([]byte(btn.GetButtonParamsJSON()), &params)
	}

	label := strings.TrimSpace(params.DisplayText)
	if label == "" {
		label = strings.TrimSpace(params.Title)
	}
	if label == "" && params.CopyCode != "" {
		label = "Copiar: " + strings.TrimSpace(params.CopyCode)
	}
	if label == "" && params.URL != "" {
		label = strings.TrimSpace(params.URL)
	}
	if label == "" && params.PhoneNumber != "" {
		label = strings.TrimSpace(params.PhoneNumber)
	}
	if label == "" {
		label = strings.TrimSpace(btn.GetName())
	}

	rawURL := strings.TrimSpace(params.URL)
	rawPhone := strings.TrimSpace(params.PhoneNumber)
	rawCopy := strings.TrimSpace(params.CopyCode)

	if rawURL != "" {
		if label != "" && label != rawURL {
			return fmt.Sprintf("[🔗 %s: %s]", label, rawURL)
		}
		return fmt.Sprintf("[🔗 %s]", rawURL)
	}
	if rawPhone != "" {
		if label != "" && label != rawPhone {
			return fmt.Sprintf("[📞 %s: %s]", label, rawPhone)
		}
		return fmt.Sprintf("[📞 %s]", rawPhone)
	}
	if rawCopy != "" {
		if label != "" && label != rawCopy && label != "Copiar: "+rawCopy {
			return fmt.Sprintf("[📋 %s: %s]", label, rawCopy)
		}
		return fmt.Sprintf("[📋 %s]", rawCopy)
	}
	if label != "" {
		return fmt.Sprintf("[🔘 %s]", label)
	}
	return ""
}

func formatInteractiveMessage(im *waProto.InteractiveMessage) string {
	if im == nil {
		return ""
	}
	var parts []string
	if h := im.GetHeader(); h != nil {
		title := strings.TrimSpace(h.GetTitle())
		subtitle := strings.TrimSpace(h.GetSubtitle())
		if title != "" && subtitle != "" {
			parts = append(parts, fmt.Sprintf("%s\n%s", title, subtitle))
		} else if title != "" {
			parts = append(parts, title)
		} else if subtitle != "" {
			parts = append(parts, subtitle)
		}
	}
	if b := im.GetBody(); b != nil && strings.TrimSpace(b.GetText()) != "" {
		parts = append(parts, strings.TrimSpace(b.GetText()))
	}
	if f := im.GetFooter(); f != nil && strings.TrimSpace(f.GetText()) != "" {
		parts = append(parts, strings.TrimSpace(f.GetText()))
	}
	if nf := im.GetNativeFlowMessage(); nf != nil {
		var btns []string
		for _, b := range nf.GetButtons() {
			if s := formatNativeFlowButton(b); s != "" {
				btns = append(btns, s)
			}
		}
		if len(btns) > 0 {
			parts = append(parts, strings.Join(btns, "\n"))
		}
	}
	if c := im.GetCarouselMessage(); c != nil {
		for i, card := range c.GetCards() {
			cardText := formatInteractiveMessage(card)
			if cardText != "" && cardText != "[Interactive Message]" {
				parts = append(parts, fmt.Sprintf("Card %d:\n%s", i+1, cardText))
			}
		}
	}
	if len(parts) == 0 {
		return "[Interactive Message]"
	}
	return strings.Join(parts, "\n\n")
}

func formatHydratedFourRowTemplate(h *waProto.TemplateMessage_HydratedFourRowTemplate) string {
	if h == nil {
		return ""
	}
	var parts []string
	if title := strings.TrimSpace(h.GetHydratedTitleText()); title != "" {
		parts = append(parts, title)
	}
	if body := strings.TrimSpace(h.GetHydratedContentText()); body != "" {
		parts = append(parts, body)
	}
	if footer := strings.TrimSpace(h.GetHydratedFooterText()); footer != "" {
		parts = append(parts, footer)
	}
	var btns []string
	for _, b := range h.GetHydratedButtons() {
		if quick := b.GetQuickReplyButton(); quick != nil {
			if text := strings.TrimSpace(quick.GetDisplayText()); text != "" {
				btns = append(btns, fmt.Sprintf("[🔘 %s]", text))
			}
		} else if urlBtn := b.GetUrlButton(); urlBtn != nil {
			text := strings.TrimSpace(urlBtn.GetDisplayText())
			u := strings.TrimSpace(urlBtn.GetURL())
			if text != "" && u != "" && text != u {
				btns = append(btns, fmt.Sprintf("[🔗 %s: %s]", text, u))
			} else if u != "" {
				btns = append(btns, fmt.Sprintf("[🔗 %s]", u))
			} else if text != "" {
				btns = append(btns, fmt.Sprintf("[🔗 %s]", text))
			}
		} else if callBtn := b.GetCallButton(); callBtn != nil {
			text := strings.TrimSpace(callBtn.GetDisplayText())
			phone := strings.TrimSpace(callBtn.GetPhoneNumber())
			if text != "" && phone != "" {
				btns = append(btns, fmt.Sprintf("[📞 %s: %s]", text, phone))
			} else if phone != "" {
				btns = append(btns, fmt.Sprintf("[📞 %s]", phone))
			} else if text != "" {
				btns = append(btns, fmt.Sprintf("[📞 %s]", text))
			}
		}
	}
	if len(btns) > 0 {
		parts = append(parts, strings.Join(btns, "\n"))
	}
	if len(parts) == 0 {
		return "[Template Message]"
	}
	return strings.Join(parts, "\n\n")
}

func formatFourRowTemplate(f *waProto.TemplateMessage_FourRowTemplate) string {
	if f == nil {
		return ""
	}
	var parts []string
	if content := extractHighlyStructuredMessageText(f.GetContent()); content != "" {
		parts = append(parts, content)
	}
	if footer := extractHighlyStructuredMessageText(f.GetFooter()); footer != "" {
		parts = append(parts, footer)
	}
	var btns []string
	for _, b := range f.GetButtons() {
		if quick := b.GetQuickReplyButton(); quick != nil {
			if text := extractHighlyStructuredMessageText(quick.GetDisplayText()); text != "" {
				btns = append(btns, fmt.Sprintf("[🔘 %s]", text))
			}
		} else if urlBtn := b.GetUrlButton(); urlBtn != nil {
			text := extractHighlyStructuredMessageText(urlBtn.GetDisplayText())
			u := extractHighlyStructuredMessageText(urlBtn.GetURL())
			if text != "" && u != "" && text != u {
				btns = append(btns, fmt.Sprintf("[🔗 %s: %s]", text, u))
			} else if u != "" {
				btns = append(btns, fmt.Sprintf("[🔗 %s]", u))
			} else if text != "" {
				btns = append(btns, fmt.Sprintf("[🔗 %s]", text))
			}
		} else if callBtn := b.GetCallButton(); callBtn != nil {
			text := extractHighlyStructuredMessageText(callBtn.GetDisplayText())
			phone := extractHighlyStructuredMessageText(callBtn.GetPhoneNumber())
			if text != "" && phone != "" {
				btns = append(btns, fmt.Sprintf("[📞 %s: %s]", text, phone))
			} else if phone != "" {
				btns = append(btns, fmt.Sprintf("[📞 %s]", phone))
			} else if text != "" {
				btns = append(btns, fmt.Sprintf("[📞 %s]", text))
			}
		}
	}
	if len(btns) > 0 {
		parts = append(parts, strings.Join(btns, "\n"))
	}
	if len(parts) == 0 {
		return "[Template Message]"
	}
	return strings.Join(parts, "\n\n")
}

func formatTemplateMessage(tm *waProto.TemplateMessage) string {
	if tm == nil {
		return ""
	}
	if im := tm.GetInteractiveMessageTemplate(); im != nil {
		return formatInteractiveMessage(im)
	}
	if h := tm.GetHydratedTemplate(); h != nil {
		return formatHydratedFourRowTemplate(h)
	}
	if h := tm.GetHydratedFourRowTemplate(); h != nil {
		return formatHydratedFourRowTemplate(h)
	}
	if f := tm.GetFourRowTemplate(); f != nil {
		return formatFourRowTemplate(f)
	}
	return "[Template Message]"
}

func extractHighlyStructuredMessageText(hsm *waProto.HighlyStructuredMessage) string {
	if hsm == nil {
		return ""
	}
	if tm := hsm.GetHydratedHsm(); tm != nil {
		return formatTemplateMessage(tm)
	}
	if len(hsm.GetParams()) > 0 {
		return strings.Join(hsm.GetParams(), " ")
	}
	if len(hsm.GetLocalizableParams()) > 0 {
		var lParams []string
		for _, lp := range hsm.GetLocalizableParams() {
			if lp.GetDefault() != "" {
				lParams = append(lParams, lp.GetDefault())
			}
		}
		if len(lParams) > 0 {
			return strings.Join(lParams, " ")
		}
	}
	if hsm.GetFallbackLg() != "" {
		return hsm.GetFallbackLg()
	}
	if hsm.GetFallbackLc() != "" {
		return hsm.GetFallbackLc()
	}
	return ""
}

func formatButtonsMessage(bm *waProto.ButtonsMessage) string {
	if bm == nil {
		return ""
	}
	var parts []string
	if bm.GetText() != "" {
		parts = append(parts, strings.TrimSpace(bm.GetText()))
	}
	if bm.GetContentText() != "" {
		parts = append(parts, strings.TrimSpace(bm.GetContentText()))
	}
	if bm.GetFooterText() != "" {
		parts = append(parts, strings.TrimSpace(bm.GetFooterText()))
	}
	var buttons []string
	for _, btn := range bm.GetButtons() {
		label := ""
		if btn.GetButtonText() != nil && btn.GetButtonText().GetDisplayText() != "" {
			label = strings.TrimSpace(btn.GetButtonText().GetDisplayText())
		} else if btn.GetNativeFlowInfo() != nil {
			label = strings.TrimSpace(btn.GetNativeFlowInfo().GetName())
		}
		if label != "" {
			buttons = append(buttons, fmt.Sprintf("[🔘 %s]", label))
		}
	}
	if len(buttons) > 0 {
		parts = append(parts, strings.Join(buttons, "\n"))
	}
	if len(parts) == 0 {
		return "[Buttons Message]"
	}
	return strings.Join(parts, "\n\n")
}

func formatListMessage(lm *waProto.ListMessage) string {
	if lm == nil {
		return ""
	}
	var parts []string
	if lm.GetTitle() != "" {
		parts = append(parts, strings.TrimSpace(lm.GetTitle()))
	}
	if lm.GetDescription() != "" {
		parts = append(parts, strings.TrimSpace(lm.GetDescription()))
	}
	if lm.GetFooterText() != "" {
		parts = append(parts, strings.TrimSpace(lm.GetFooterText()))
	}
	if lm.GetButtonText() != "" {
		parts = append(parts, fmt.Sprintf("[📋 %s]", strings.TrimSpace(lm.GetButtonText())))
	}
	for _, sec := range lm.GetSections() {
		var secLines []string
		if sec.GetTitle() != "" {
			secLines = append(secLines, fmt.Sprintf("• %s:", sec.GetTitle()))
		}
		for _, row := range sec.GetRows() {
			rowTitle := strings.TrimSpace(row.GetTitle())
			rowDesc := strings.TrimSpace(row.GetDescription())
			if rowTitle != "" && rowDesc != "" {
				secLines = append(secLines, fmt.Sprintf("  - %s: %s", rowTitle, rowDesc))
			} else if rowTitle != "" {
				secLines = append(secLines, fmt.Sprintf("  - %s", rowTitle))
			}
		}
		if len(secLines) > 0 {
			parts = append(parts, strings.Join(secLines, "\n"))
		}
	}
	if len(parts) == 0 {
		return "[List Message]"
	}
	return strings.Join(parts, "\n\n")
}

func extractImageMessage(protoMsg *waProto.Message) *waProto.ImageMessage {
	if protoMsg == nil {
		return nil
	}
	if img := protoMsg.GetImageMessage(); img != nil {
		return img
	}
	if tm := protoMsg.GetTemplateMessage(); tm != nil {
		if h := tm.GetHydratedTemplate(); h != nil && h.GetImageMessage() != nil {
			return h.GetImageMessage()
		}
		if h := tm.GetHydratedFourRowTemplate(); h != nil && h.GetImageMessage() != nil {
			return h.GetImageMessage()
		}
		if f := tm.GetFourRowTemplate(); f != nil && f.GetImageMessage() != nil {
			return f.GetImageMessage()
		}
		if im := tm.GetInteractiveMessageTemplate(); im != nil && im.GetHeader() != nil && im.GetHeader().GetImageMessage() != nil {
			return im.GetHeader().GetImageMessage()
		}
	}
	if im := protoMsg.GetInteractiveMessage(); im != nil && im.GetHeader() != nil && im.GetHeader().GetImageMessage() != nil {
		return im.GetHeader().GetImageMessage()
	}
	if bm := protoMsg.GetButtonsMessage(); bm != nil && bm.GetImageMessage() != nil {
		return bm.GetImageMessage()
	}
	return nil
}

func extractDocumentMessage(protoMsg *waProto.Message) *waProto.DocumentMessage {
	if protoMsg == nil {
		return nil
	}
	if doc := protoMsg.GetDocumentMessage(); doc != nil {
		return doc
	}
	if tm := protoMsg.GetTemplateMessage(); tm != nil {
		if h := tm.GetHydratedTemplate(); h != nil && h.GetDocumentMessage() != nil {
			return h.GetDocumentMessage()
		}
		if h := tm.GetHydratedFourRowTemplate(); h != nil && h.GetDocumentMessage() != nil {
			return h.GetDocumentMessage()
		}
		if f := tm.GetFourRowTemplate(); f != nil && f.GetDocumentMessage() != nil {
			return f.GetDocumentMessage()
		}
		if im := tm.GetInteractiveMessageTemplate(); im != nil && im.GetHeader() != nil && im.GetHeader().GetDocumentMessage() != nil {
			return im.GetHeader().GetDocumentMessage()
		}
	}
	if im := protoMsg.GetInteractiveMessage(); im != nil && im.GetHeader() != nil && im.GetHeader().GetDocumentMessage() != nil {
		return im.GetHeader().GetDocumentMessage()
	}
	if bm := protoMsg.GetButtonsMessage(); bm != nil && bm.GetDocumentMessage() != nil {
		return bm.GetDocumentMessage()
	}
	return nil
}

func extractVideoMessage(protoMsg *waProto.Message) *waProto.VideoMessage {
	if protoMsg == nil {
		return nil
	}
	if vid := protoMsg.GetVideoMessage(); vid != nil {
		return vid
	}
	if tm := protoMsg.GetTemplateMessage(); tm != nil {
		if h := tm.GetHydratedTemplate(); h != nil && h.GetVideoMessage() != nil {
			return h.GetVideoMessage()
		}
		if h := tm.GetHydratedFourRowTemplate(); h != nil && h.GetVideoMessage() != nil {
			return h.GetVideoMessage()
		}
		if f := tm.GetFourRowTemplate(); f != nil && f.GetVideoMessage() != nil {
			return f.GetVideoMessage()
		}
		if im := tm.GetInteractiveMessageTemplate(); im != nil && im.GetHeader() != nil && im.GetHeader().GetVideoMessage() != nil {
			return im.GetHeader().GetVideoMessage()
		}
	}
	if im := protoMsg.GetInteractiveMessage(); im != nil && im.GetHeader() != nil && im.GetHeader().GetVideoMessage() != nil {
		return im.GetHeader().GetVideoMessage()
	}
	if bm := protoMsg.GetButtonsMessage(); bm != nil && bm.GetVideoMessage() != nil {
		return bm.GetVideoMessage()
	}
	return nil
}

// GetPollCreationMessage extracts a PollCreationMessage regardless of its version.
func (ms *MessageService) GetPollCreationMessage(msg *waProto.Message) *waProto.PollCreationMessage {
	if msg == nil {
		return nil
	}
	if pm := msg.GetPollCreationMessage(); pm != nil {
		return pm
	}
	if pm := msg.GetPollCreationMessageV2(); pm != nil {
		return pm
	}
	if pm := msg.GetPollCreationMessageV3(); pm != nil {
		return pm
	}
	if pm := msg.GetPollCreationMessageV5(); pm != nil {
		return pm
	}
	if pm := msg.GetPollCreationMessageV6(); pm != nil {
		return pm
	}
	return nil
}

// ExtractContent extracts the text content from various WhatsApp message types.
func (ms *MessageService) ExtractContent(msg *events.Message) string {
	return ms.ExtractContentFromProto(msg.Message)
}

// ExtractContextInfo extracts the ContextInfo (quoted message info) from various message types.
func (ms *MessageService) ExtractContextInfo(msg *events.Message) *waProto.ContextInfo {
	if msg == nil || msg.Message == nil {
		return nil
	}
	protoMsg, _ := ms.UnwrapMessage(msg.Message)
	if protoMsg == nil {
		return nil
	}
	if etm := protoMsg.GetExtendedTextMessage(); etm != nil {
		return etm.GetContextInfo()
	}
	if img := protoMsg.GetImageMessage(); img != nil {
		return img.GetContextInfo()
	}
	if vid := protoMsg.GetVideoMessage(); vid != nil {
		return vid.GetContextInfo()
	}
	if ptv := protoMsg.GetPtvMessage(); ptv != nil {
		return ptv.GetContextInfo()
	}
	if aud := protoMsg.GetAudioMessage(); aud != nil {
		return aud.GetContextInfo()
	}
	if doc := protoMsg.GetDocumentMessage(); doc != nil {
		return doc.GetContextInfo()
	}
	if stkr := protoMsg.GetStickerMessage(); stkr != nil {
		return stkr.GetContextInfo()
	}
	if contact := protoMsg.GetContactMessage(); contact != nil {
		return contact.GetContextInfo()
	}
	if loc := protoMsg.GetLocationMessage(); loc != nil {
		return loc.GetContextInfo()
	}
	if liveLoc := protoMsg.GetLiveLocationMessage(); liveLoc != nil {
		return liveLoc.GetContextInfo()
	}
	if bm := protoMsg.GetButtonsMessage(); bm != nil {
		return bm.GetContextInfo()
	}
	if brm := protoMsg.GetButtonsResponseMessage(); brm != nil {
		return brm.GetContextInfo()
	}
	if lm := protoMsg.GetListMessage(); lm != nil {
		return lm.GetContextInfo()
	}
	if lrm := protoMsg.GetListResponseMessage(); lrm != nil {
		return lrm.GetContextInfo()
	}
	if tm := protoMsg.GetTemplateMessage(); tm != nil {
		if tm.GetContextInfo() != nil {
			return tm.GetContextInfo()
		}
		if im := tm.GetInteractiveMessageTemplate(); im != nil {
			return im.GetContextInfo()
		}
		return nil
	}
	if tbrm := protoMsg.GetTemplateButtonReplyMessage(); tbrm != nil {
		return tbrm.GetContextInfo()
	}
	if im := protoMsg.GetInteractiveMessage(); im != nil {
		return im.GetContextInfo()
	}
	if irm := protoMsg.GetInteractiveResponseMessage(); irm != nil {
		return irm.GetContextInfo()
	}
	if pm := protoMsg.GetPollCreationMessage(); pm != nil {
		return pm.GetContextInfo()
	}
	if pm := protoMsg.GetPollCreationMessageV2(); pm != nil {
		return pm.GetContextInfo()
	}
	if pm := protoMsg.GetPollCreationMessageV3(); pm != nil {
		return pm.GetContextInfo()
	}
	return nil
}

// PersistMessage extracts message type, content, and media metadata from protobuf
// and saves it to the database.
func (ms *MessageService) PersistMessage(msg *events.Message) {
	ms.PersistMessageTx(nil, msg)
}

// PersistMessageTx extracts message type, content, and media metadata from protobuf
// and saves it to the database, optionally within an active SQL transaction.
func (ms *MessageService) PersistMessageTx(tx *sql.Tx, msg *events.Message) {
	if msg == nil || msg.Message == nil {
		return
	}
	if msg.Message.GetReactionMessage() != nil || 
	   msg.Message.GetProtocolMessage() != nil ||
	   msg.Message.GetSenderKeyDistributionMessage() != nil {
		return
	}

	protoMsg, isViewOnce := ms.UnwrapMessage(msg.Message)
	if protoMsg == nil {
		return
	}

	msgType := "text"; content := ms.ExtractContent(msg); var thumb []byte
	var metadata database.Message
	metadata.IsViewOnce = isViewOnce

	if img := extractImageMessage(protoMsg); img != nil {
		msgType = "image"; thumb = img.GetJPEGThumbnail()
		if img.GetCaption() != "" {
			metadata.Caption = sql.NullString{String: img.GetCaption(), Valid: true}
		} else if content != "" && content != "[Image]" {
			metadata.Caption = sql.NullString{String: content, Valid: true}
		}
		metadata.MediaURL = sql.NullString{String: img.GetURL(), Valid: img.GetURL() != ""}
		metadata.MediaDirectPath = sql.NullString{String: img.GetDirectPath(), Valid: img.GetDirectPath() != ""}
		metadata.MediaKey = img.GetMediaKey()
		metadata.MediaMimetype = sql.NullString{String: img.GetMimetype(), Valid: img.GetMimetype() != ""}
		metadata.MediaEncSHA256 = img.GetFileEncSHA256()
		metadata.MediaSHA256 = img.GetFileSHA256()
		metadata.MediaLength = sql.NullInt64{Int64: int64(img.GetFileLength()), Valid: true}
		metadata.MediaWidth = sql.NullInt64{Int64: int64(img.GetWidth()), Valid: true}
		metadata.MediaHeight = sql.NullInt64{Int64: int64(img.GetHeight()), Valid: true}
	} else if stkr := protoMsg.GetStickerMessage(); stkr != nil {
		msgType = "sticker"; thumb = stkr.GetPngThumbnail()
		metadata.MediaURL = sql.NullString{String: stkr.GetURL(), Valid: stkr.GetURL() != ""}
		metadata.MediaDirectPath = sql.NullString{String: stkr.GetDirectPath(), Valid: stkr.GetDirectPath() != ""}
		metadata.MediaKey = stkr.GetMediaKey()
		metadata.MediaMimetype = sql.NullString{String: stkr.GetMimetype(), Valid: stkr.GetMimetype() != ""}
		metadata.MediaEncSHA256 = stkr.GetFileEncSHA256()
		metadata.MediaSHA256 = stkr.GetFileSHA256()
		metadata.MediaLength = sql.NullInt64{Int64: int64(stkr.GetFileLength()), Valid: true}
		metadata.MediaWidth = sql.NullInt64{Int64: int64(stkr.GetWidth()), Valid: true}
		metadata.MediaHeight = sql.NullInt64{Int64: int64(stkr.GetHeight()), Valid: true}
	} else if ptv := protoMsg.GetPtvMessage(); ptv != nil {
		msgType = "video"; thumb = ptv.GetJPEGThumbnail()
		if ptv.GetCaption() != "" {
			metadata.Caption = sql.NullString{String: ptv.GetCaption(), Valid: true}
		}
		metadata.MediaURL = sql.NullString{String: ptv.GetURL(), Valid: ptv.GetURL() != ""}
		metadata.MediaDirectPath = sql.NullString{String: ptv.GetDirectPath(), Valid: ptv.GetDirectPath() != ""}
		metadata.MediaKey = ptv.GetMediaKey()
		metadata.MediaMimetype = sql.NullString{String: ptv.GetMimetype(), Valid: ptv.GetMimetype() != ""}
		metadata.MediaEncSHA256 = ptv.GetFileEncSHA256()
		metadata.MediaSHA256 = ptv.GetFileSHA256()
		metadata.MediaLength = sql.NullInt64{Int64: int64(ptv.GetFileLength()), Valid: true}
		metadata.MediaWidth = sql.NullInt64{Int64: int64(ptv.GetWidth()), Valid: true}
		metadata.MediaHeight = sql.NullInt64{Int64: int64(ptv.GetHeight()), Valid: true}
	} else if vid := extractVideoMessage(protoMsg); vid != nil {
		msgType = "video"; thumb = vid.GetJPEGThumbnail()
		if vid.GetCaption() != "" {
			metadata.Caption = sql.NullString{String: vid.GetCaption(), Valid: true}
		} else if content != "" && content != "[Video]" {
			metadata.Caption = sql.NullString{String: content, Valid: true}
		}
		metadata.MediaURL = sql.NullString{String: vid.GetURL(), Valid: vid.GetURL() != ""}
		metadata.MediaDirectPath = sql.NullString{String: vid.GetDirectPath(), Valid: vid.GetDirectPath() != ""}
		metadata.MediaKey = vid.GetMediaKey()
		metadata.MediaMimetype = sql.NullString{String: vid.GetMimetype(), Valid: vid.GetMimetype() != ""}
		metadata.MediaEncSHA256 = vid.GetFileEncSHA256()
		metadata.MediaSHA256 = vid.GetFileSHA256()
		metadata.MediaLength = sql.NullInt64{Int64: int64(vid.GetFileLength()), Valid: true}
		metadata.MediaWidth = sql.NullInt64{Int64: int64(vid.GetWidth()), Valid: true}
		metadata.MediaHeight = sql.NullInt64{Int64: int64(vid.GetHeight()), Valid: true}
	} else if doc := extractDocumentMessage(protoMsg); doc != nil {
		msgType = "document"; thumb = doc.GetJPEGThumbnail()
		if doc.GetCaption() != "" {
			metadata.Caption = sql.NullString{String: doc.GetCaption(), Valid: true}
		} else if content != "" && !strings.HasPrefix(content, "[Document") {
			metadata.Caption = sql.NullString{String: content, Valid: true}
		}
		metadata.MediaURL = sql.NullString{String: doc.GetURL(), Valid: doc.GetURL() != ""}
		metadata.MediaDirectPath = sql.NullString{String: doc.GetDirectPath(), Valid: doc.GetDirectPath() != ""}
		metadata.MediaKey = doc.GetMediaKey()
		metadata.MediaMimetype = sql.NullString{String: doc.GetMimetype(), Valid: doc.GetMimetype() != ""}
		metadata.MediaEncSHA256 = doc.GetFileEncSHA256()
		metadata.MediaSHA256 = doc.GetFileSHA256()
		metadata.MediaLength = sql.NullInt64{Int64: int64(doc.GetFileLength()), Valid: true}
	} else if aud := protoMsg.GetAudioMessage(); aud != nil {
		msgType = "audio"
		metadata.MediaURL = sql.NullString{String: aud.GetURL(), Valid: aud.GetURL() != ""}
		metadata.MediaDirectPath = sql.NullString{String: aud.GetDirectPath(), Valid: aud.GetDirectPath() != ""}
		metadata.MediaKey = aud.GetMediaKey()
		metadata.MediaMimetype = sql.NullString{String: aud.GetMimetype(), Valid: aud.GetMimetype() != ""}
		metadata.MediaEncSHA256 = aud.GetFileEncSHA256()
		metadata.MediaSHA256 = aud.GetFileSHA256()
		metadata.MediaLength = sql.NullInt64{Int64: int64(aud.GetFileLength()), Valid: true}
	} else if poll := ms.GetPollCreationMessage(protoMsg); poll != nil {
		msgType = "poll"
		for _, opt := range poll.GetOptions() {
			_ = ms.DB.SavePollOptionTx(tx, msg.Info.ID, opt.GetOptionName())
		}
	}

	// Check if file already exists on disk
	if msgType != "text" {
		ext := ".jpg"
		switch msgType {
		case "sticker": ext = ".webp"
		case "video": ext = ".mp4"
		case "audio": ext = ".ogg"
		case "document": ext = ".bin"
		}
		path := paths.MediaPath(msg.Info.ID + ext)
		if _, err := os.Stat(path); err == nil {
			content = path
		}
	}

	// Only save if it has content or is a known media type
	if content == "" && msgType == "text" {
		return
	}

	if (msgType == "image" || msgType == "sticker" || msgType == "video") && len(thumb) == 0 {
		fmt.Printf("Bridge: Media %s arrived without thumbnail (ID: %s)\n", msgType, msg.Info.ID)
	}

	chatJID := ms.ResolveJID(msg.Info.Chat).String()
	senderJID := ms.ResolveJID(msg.Info.Sender).String()
	
	if tx == nil && strings.HasSuffix(msg.Info.Sender.String(), "@lid") {
		go ms.Contacts.ResolveLIDMapping(msg.Info.Sender.String())
	}
	
	metadata.ID = msg.Info.ID; metadata.ChatJID = chatJID; metadata.SenderJID = senderJID
	metadata.Content = content; metadata.Type = msgType; metadata.Timestamp = msg.Info.Timestamp
	metadata.IsFromMe = msg.Info.IsFromMe; metadata.Thumbnail = thumb
	if metadata.IsFromMe && metadata.Status == "" {
		metadata.Status = "sent"
	}

	// Extract ContextInfo
	if ci := ms.ExtractContextInfo(msg); ci != nil {
		metadata.IsForwarded = ci.GetIsForwarded()
		if ci.GetStanzaID() != "" {
			metadata.QuotedMsgID = sql.NullString{String: ci.GetStanzaID(), Valid: true}
			metadata.QuotedMsgSender = sql.NullString{String: ci.GetParticipant(), Valid: ci.GetParticipant() != ""}
			
			// Extract quoted content
			quotedContent := ""
			if qm := ci.GetQuotedMessage(); qm != nil {
				quotedContent = ms.ExtractContentFromProto(qm)
			}
			metadata.QuotedMsgContent = sql.NullString{String: quotedContent, Valid: quotedContent != ""}
		}
	}
	var err error
	if tx != nil {
		err = ms.DB.SaveMessageTx(tx, metadata)
		_ = ms.DB.UpdateContactTimestampTx(tx, chatJID, msg.Info.Timestamp)
	} else {
		err = ms.DB.SaveMessage(metadata)
		_ = ms.DB.UpdateContactTimestamp(chatJID, msg.Info.Timestamp)
	}
	if err != nil {
		fmt.Printf("Bridge: Error saving message %s: %v\n", metadata.ID, err)
	}
	
	if !msg.Info.IsFromMe {
		pushName := msg.Info.PushName; fullName := ""
		contactInfo, err := ms.Backend.Client.Store.Contacts.GetContact(ms.ctx, msg.Info.Sender)
		if err == nil && contactInfo.Found { if pushName == "" { pushName = contactInfo.PushName }; fullName = contactInfo.FullName }
		if pushName != "" || fullName != "" {
			c := database.Contact{JID: senderJID, SavedName: sql.NullString{String: fullName, Valid: fullName != ""}, PushName: sql.NullString{String: pushName, Valid: pushName != ""}}
			if tx != nil {
				_ = ms.DB.SaveContactTx(tx, c)
			} else {
				_ = ms.DB.SaveContact(c)
			}
		}
	}
}

// PersistMediaMessage is a simplified persistence for pre-downloaded media messages.
func (ms *MessageService) PersistMediaMessage(msg *events.Message, msgType string, path string) {
	protoMsg, isViewOnce := ms.UnwrapMessage(msg.Message)
	chatJID := msg.Info.Chat.ToNonAD().String(); senderJID := msg.Info.Sender.ToNonAD().String()
	var thumb []byte
	var width, height int64
	if img := protoMsg.GetImageMessage(); img != nil { 
		thumb = img.GetJPEGThumbnail()
		width = int64(img.GetWidth())
		height = int64(img.GetHeight())
	} else if stkr := protoMsg.GetStickerMessage(); stkr != nil { 
		thumb = stkr.GetPngThumbnail() 
		width = int64(stkr.GetWidth())
		height = int64(stkr.GetHeight())
	}
	status := ""
	if msg.Info.IsFromMe {
		status = "sent"
	}
	ms.DB.SaveMessage(database.Message{
		ID: msg.Info.ID, ChatJID: chatJID, SenderJID: senderJID, Content: path, Type: msgType, 
		Timestamp: msg.Info.Timestamp, Status: status, IsFromMe: msg.Info.IsFromMe, Thumbnail: thumb,
		MediaWidth: sql.NullInt64{Int64: width, Valid: width > 0},
		MediaHeight: sql.NullInt64{Int64: height, Valid: height > 0},
		IsViewOnce: isViewOnce,
	})
	ms.DB.UpdateContactTimestamp(chatJID, msg.Info.Timestamp)
}

// HandleReaction saves a reaction to the database and updates the UI if the chat is selected.
func (ms *MessageService) HandleReaction(chat, sender types.JID, text, targetID string, timestamp time.Time) {
	ms.HandleReactionTx(nil, chat, sender, text, targetID, timestamp)
}

func (ms *MessageService) HandleReactionTx(tx *sql.Tx, chat, sender types.JID, text, targetID string, timestamp time.Time) {
	react := database.Reaction{
		MessageID: targetID,
		SenderJID: sender.ToNonAD().String(),
		Reaction:  text,
		Timestamp: timestamp,
	}
	if tx != nil {
		_ = ms.DB.SaveReactionTx(tx, react)
	} else {
		_ = ms.DB.SaveReaction(react)
	}
	
	selectedJID := ms.GetSelectedJID()
	if selectedJID != nil && chat.ToNonAD().String() == selectedJID.ToNonAD().String() {
		reactions, _ := ms.DB.GetReactions(targetID)
		ms.App.ChatView.UpdateMessageReactions(targetID, uniqueReactions(reactions))
	}
}

// ProcessPin recursively processes pin/unpin protocol messages.
// Returns true if a pin message was processed.
func (ms *MessageService) ProcessPin(msg *waProto.Message, chat types.JID) bool {
	if pinMsg := msg.GetPinInChatMessage(); pinMsg != nil {
		targetID := pinMsg.GetKey().GetID()
		isPinned := pinMsg.GetType() == waProto.PinInChatMessage_PIN_FOR_ALL
		chatJID := chat.ToNonAD().String()
		ms.DB.UpdateMessagePinned(targetID, chatJID, isPinned)
		
		selectedJID := ms.GetSelectedJID()
		if selectedJID != nil && chatJID == selectedJID.ToNonAD().String() {
			ms.App.ChatView.UpdateMessagePinned(targetID, isPinned)
			if isPinned {
				if m, err := ms.DB.GetMessage(targetID); err == nil {
					ms.App.ChatView.SetPinnedMessage(m.Content)
				}
			} else {
				ms.App.ChatView.SetPinnedMessage("")
			}
		}
		return true
	}

	if msg.GetProtocolMessage() != nil && msg.GetProtocolMessage().GetEditedMessage() != nil {
		return ms.ProcessPin(msg.GetProtocolMessage().GetEditedMessage(), chat)
	}

	return false
}

func (ms *MessageService) HandleEdit(protoMsg *waProto.ProtocolMessage, chat types.JID) {
	if protoMsg.GetEditedMessage() == nil || protoMsg.GetKey() == nil {
		return
	}
	targetID := protoMsg.GetKey().GetID()
	newContent := ms.ExtractContentFromProto(protoMsg.GetEditedMessage())
	if newContent == "" {
		return
	}
	
	chatJID := chat.ToNonAD().String()
	err := ms.DB.UpdateMessageContent(targetID, chatJID, newContent, true)
	if err != nil {
		fmt.Printf("Bridge: Failed to update edited message %s: %v\n", targetID, err)
		return
	}
	
	// Update UI
	chatView := ms.App.GetChatViewForJID(chatJID)
	if chatView != nil {
		chatView.UpdateMessageContent(targetID, newContent, true)
	}
}

func (ms *MessageService) HandleRevoke(protoMsg *waProto.ProtocolMessage, chat types.JID) {
	if protoMsg.GetKey() == nil {
		return
	}
	targetID := protoMsg.GetKey().GetID()
	chatJID := chat.ToNonAD().String()
	_ = ms.DB.MarkMessageRevoked(targetID, chatJID)

	chatView := ms.App.GetChatViewForJID(chatJID)
	if chatView != nil {
		chatView.UpdateMessageContent(targetID, "🚫 Esta mensagem foi apagada", false)
	}
}
