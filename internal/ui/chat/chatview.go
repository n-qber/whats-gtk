package chat

import (
	"context"
	"whats-gtk/internal/database"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type ChatView struct {
	Box         *gtk.Box
	Stack       *gtk.Stack
	EmptyState  *adw.StatusPage
	TopBar      *TopBar
	SearchBar   *SearchBar
	MessageList *MessageList
	InputBar    *InputBar
	Banner      *adw.Banner

	// High-level Callbacks (consumed by bridge)
	OnReconnect             func()
	OnSendMessage           func(text string, replyToID string)
	OnPasteImage            func(tex *gdk.Texture)
	OnSendFile              func(path string)
	OnSendSticker           func(item database.StickerItem)
	OnSendStickerFile       func(path string)
	OnToggleFavoriteSticker func(item database.StickerItem, isFav bool)
	OnDeleteStickerHistory  func(id string)
	OnSyncFavorites         func()
	IsStickerFavorite       func(id, filePath string) bool
	LoadFavorites           func() []database.StickerItem
	LoadHistory             func() []database.StickerItem
	OnDownloadMedia         func(id string)
	OnOpenImage             func(path string)
	OnSendReaction          func(id, emoji string)
	OnPinMessage            func(id string, pin bool, duration uint32)
	OnDetach                func()
	OnLoadOlder             func()
	OnLoadMessageRequest    func(id string)
	OnSearchMessages        func(query string)
	OnCancelSearch          func()
	OnCancelSearchAndJump   func(id string)
	OnSearchResultClick     func(id string)
	OnMentionClick          func(jid string)
	OnSendPollVote          func(msgID string, senderJID string, isFromMe bool, selectedOptions []string)
	OnHeaderClick           func()
	OnForwardMessages       func(ids []string)
	OnTyping                func()
	OnStopTyping            func()

	// Direct Field Accessors for legacy compatibility
	IsSearching bool

	ctx       context.Context
	cancelCtx context.CancelFunc
}

func NewChatView() (*ChatView, error) {
	ctx, cancel := context.WithCancel(context.Background())

	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.SetName("chat-view-box")

	box.ConnectDestroy(func() {
		cancel()
	})

	cv := &ChatView{
		Box:       box,
		ctx:       ctx,
		cancelCtx: cancel,
	}

	cv.TopBar = NewTopBar()
	cv.SearchBar = NewSearchBar(box)
	cv.MessageList = NewMessageList()
	cv.InputBar = NewInputBar(ctx)

	// Wire TopBar
	cv.TopBar.OnHeaderClick = func() {
		if cv.OnHeaderClick != nil {
			cv.OnHeaderClick()
		}
	}
	cv.TopBar.OnDetach = func() {
		if cv.OnDetach != nil {
			cv.OnDetach()
		}
	}

	// Wire SearchBar
	cv.SearchBar.OnSearchMessages = func(query string) {
		if cv.OnSearchMessages != nil {
			cv.OnSearchMessages(query)
		}
	}
	cv.SearchBar.OnCancelSearch = func() {
		if cv.OnCancelSearch != nil {
			cv.OnCancelSearch()
		}
	}

	// Wire MessageList
	cv.MessageList.OnLoadOlder = func() {
		if cv.OnLoadOlder != nil {
			cv.OnLoadOlder()
		}
	}
	cv.MessageList.OnLoadMessageRequest = func(id string) {
		if cv.OnLoadMessageRequest != nil {
			cv.OnLoadMessageRequest(id)
		}
	}
	cv.MessageList.OnSearchResultClick = func(id string) {
		if cv.OnSearchResultClick != nil {
			cv.OnSearchResultClick(id)
		}
	}
	cv.MessageList.OnDownloadMedia = func(id string) {
		if cv.OnDownloadMedia != nil {
			cv.OnDownloadMedia(id)
		}
	}
	cv.MessageList.OnOpenImage = func(path string) {
		if cv.OnOpenImage != nil {
			cv.OnOpenImage(path)
		}
	}
	cv.MessageList.OnSendReaction = func(id, emoji string) {
		if cv.OnSendReaction != nil {
			cv.OnSendReaction(id, emoji)
		}
	}
	cv.MessageList.OnPinMessage = func(id string, pin bool, duration uint32) {
		if cv.OnPinMessage != nil {
			cv.OnPinMessage(id, pin, duration)
		}
	}
	cv.MessageList.OnMentionClick = func(jid string) {
		if cv.OnMentionClick != nil {
			cv.OnMentionClick(jid)
		}
	}
	cv.MessageList.OnSendPollVote = func(msgID string, senderJID string, isFromMe bool, selectedOptions []string) {
		if cv.OnSendPollVote != nil {
			cv.OnSendPollVote(msgID, senderJID, isFromMe, selectedOptions)
		}
	}
	cv.MessageList.OnReplyRequest = func(id, sender, content string) {
		cv.InputBar.SetReplyTo(id, sender, content)
	}
	cv.MessageList.OnForwardMessagesRequest = func(ids []string) {
		if cv.OnForwardMessages != nil {
			cv.OnForwardMessages(ids)
		}
	}
	cv.MessageList.OnContextMenuClosed = func() {
		cv.FocusEntry()
	}
	cv.MessageList.OnToggleFavoriteSticker = func(msgID, path string, isFav bool) {
		if cv.OnToggleFavoriteSticker != nil {
			cv.OnToggleFavoriteSticker(database.StickerItem{ID: msgID, FilePath: path}, isFav)
		}
	}
	cv.MessageList.IsStickerFavorite = func(id, path string) bool {
		if cv.IsStickerFavorite != nil {
			return cv.IsStickerFavorite(id, path)
		}
		return false
	}
	cv.MessageList.OnSendStickerFile = func(path string) {
		if cv.OnSendStickerFile != nil {
			cv.OnSendStickerFile(path)
		}
	}

	// Wire InputBar
	cv.InputBar.OnTyping = func() {
		if cv.OnTyping != nil {
			cv.OnTyping()
		}
	}
	cv.InputBar.OnStopTyping = func() {
		if cv.OnStopTyping != nil {
			cv.OnStopTyping()
		}
	}
	cv.InputBar.OnSendMessage = func(text, replyToID string) {
		if cv.OnSendMessage != nil {
			cv.OnSendMessage(text, replyToID)
		}
	}
	cv.InputBar.OnSendFile = func(path string) {
		if cv.OnSendFile != nil {
			cv.OnSendFile(path)
		}
	}
	cv.InputBar.OnPasteImage = func(tex *gdk.Texture) {
		if cv.OnPasteImage != nil {
			cv.OnPasteImage(tex)
		}
	}
	cv.InputBar.OnSendSticker = func(item database.StickerItem) {
		if cv.OnSendSticker != nil {
			cv.OnSendSticker(item)
		}
	}
	cv.InputBar.OnSendStickerFile = func(path string) {
		if cv.OnSendStickerFile != nil {
			cv.OnSendStickerFile(path)
		}
	}
	cv.InputBar.OnToggleFavoriteSticker = func(item database.StickerItem, isFav bool) {
		if cv.OnToggleFavoriteSticker != nil {
			cv.OnToggleFavoriteSticker(item, isFav)
		}
	}
	cv.InputBar.OnDeleteStickerHistory = func(id string) {
		if cv.OnDeleteStickerHistory != nil {
			cv.OnDeleteStickerHistory(id)
		}
	}
	cv.InputBar.OnSyncFavorites = func() {
		if cv.OnSyncFavorites != nil {
			cv.OnSyncFavorites()
		}
	}
	cv.InputBar.IsStickerFavorite = func(id, path string) bool {
		if cv.IsStickerFavorite != nil {
			return cv.IsStickerFavorite(id, path)
		}
		return false
	}
	cv.InputBar.LoadFavorites = func() []database.StickerItem {
		if cv.LoadFavorites != nil {
			return cv.LoadFavorites()
		}
		return nil
	}
	cv.InputBar.LoadHistory = func() []database.StickerItem {
		if cv.LoadHistory != nil {
			return cv.LoadHistory()
		}
		return nil
	}
	cv.InputBar.OnSelectMessageUp = cv.MessageList.SelectMessageUp
	cv.InputBar.OnSelectMessageDown = cv.MessageList.SelectMessageDown
	cv.InputBar.OnSelectMessageBlockUp = cv.MessageList.SelectMessageBlockUp
	cv.InputBar.OnSelectMessageBlockDown = cv.MessageList.SelectMessageBlockDown
	cv.InputBar.OnConfirmMessageSelection = cv.MessageList.ConfirmKeyboardReply
	cv.InputBar.OnCancelMessageSelection = func() bool {
		if cv.MessageList.HasKeyboardSelection() {
			cv.MessageList.ClearKeyboardSelection()
			return true
		}
		return false
	}

	// ChatView key controller for message navigation when outside entry
	chatKeyCtrl := gtk.NewEventControllerKey()
	chatKeyCtrl.SetPropagationPhase(gtk.PhaseCapture)
	chatKeyCtrl.ConnectKeyPressed(func(keyval uint, keycode uint, state gdk.ModifierType) bool {
		name := gdk.KeyvalName(keyval)
		isCtrl := state&gdk.ControlMask != 0
		isShift := state&gdk.ShiftMask != 0

		if isCtrl && (keyval == gdk.KEY_Up || keyval == gdk.KEY_KP_Up || name == "Up" || name == "KP_Up") {
			if cv.MessageList.SelectMessageUp() {
				return true
			}
		} else if isCtrl && (keyval == gdk.KEY_Down || keyval == gdk.KEY_KP_Down || name == "Down" || name == "KP_Down") {
			if cv.MessageList.SelectMessageDown() {
				return true
			}
		} else if isCtrl && (keyval == gdk.KEY_Left || keyval == gdk.KEY_KP_Left || name == "Left" || name == "KP_Left") {
			if cv.MessageList.SelectMessageBlockUp() {
				return true
			}
		} else if isCtrl && (keyval == gdk.KEY_Right || keyval == gdk.KEY_KP_Right || name == "Right" || name == "KP_Right") {
			if cv.MessageList.SelectMessageBlockDown() {
				return true
			}
		}
		if !isShift && (keyval == gdk.KEY_Return || keyval == gdk.KEY_KP_Enter || keyval == gdk.KEY_ISO_Enter || name == "Return" || name == "KP_Enter") {
			if cv.MessageList.ConfirmKeyboardReply() {
				cv.FocusEntry()
				return true
			}
		}
		if keyval == gdk.KEY_Escape || name == "Escape" {
			if cv.InputBar.ReplyToID != "" {
				cv.InputBar.CancelReply()
				cv.FocusEntry()
				return true
			}
			if cv.MessageList.HasKeyboardSelection() {
				cv.MessageList.ClearKeyboardSelection()
				cv.FocusEntry()
				return true
			}
		}
		return false
	})
	box.AddController(chatKeyCtrl)

	// Build layout using a Stack for Empty State vs Active Chat View
	cv.Stack = gtk.NewStack()

	// 1. Static Empty View Container (with HeaderBar for close button & window controls)
	emptyBox := gtk.NewBox(gtk.OrientationVertical, 0)
	emptyHeaderBar := adw.NewHeaderBar()

	statusPage := adw.NewStatusPage()
	statusPage.SetTitle("No Conversation")
	statusPage.SetDescription("Select a chat from the sidebar to start messaging")
	statusPage.SetIconName("chat-symbolic")
	statusPage.SetVExpand(true)
	statusPage.SetHExpand(true)
	cv.EmptyState = statusPage

	emptyBox.Append(emptyHeaderBar)
	emptyBox.Append(statusPage)

	// 2. Active Chat Box Container
	chatBox := gtk.NewBox(gtk.OrientationVertical, 0)
	chatBox.Append(cv.TopBar.Header)
	chatBox.Append(cv.SearchBar.Widget)
	chatBox.Append(cv.MessageList.Widget)
	chatBox.Append(cv.InputBar.Box)

	cv.Stack.AddNamed(emptyBox, "empty")
	cv.Stack.AddNamed(chatBox, "chat")

	// Banner for network/reconnection alerts
	cv.Banner = adw.NewBanner("")
	cv.Banner.SetRevealed(false)
	cv.Banner.ConnectButtonClicked(func() {
		if cv.OnReconnect != nil {
			cv.OnReconnect()
		}
	})

	box.Append(cv.Banner)
	box.Append(cv.Stack)

	return cv, nil
}

// SetConnectionStatus updates the contextual connection banner.
// An empty status string hides the banner.
func (cv *ChatView) SetConnectionStatus(status string, showReconnect bool) {
	if cv.Banner == nil {
		return
	}
	if status == "" {
		cv.Banner.SetRevealed(false)
		return
	}
	cv.Banner.SetTitle(status)
	if showReconnect {
		cv.Banner.SetButtonLabel("Reconnect")
	} else {
		cv.Banner.SetButtonLabel("")
	}
	cv.Banner.SetRevealed(true)
}

// Proxies for legacy compatibility

func (cv *ChatView) SetNoConversation() {
	cv.Clear()
	cv.ClearInput()
	if cv.Stack != nil {
		cv.Stack.SetVisibleChildName("empty")
	}
}

func (cv *ChatView) SetHeader(name string, tex *gdk.Texture) {
	if name == "" || name == "WhatsApp GTK" || name == "Select a chat" {
		cv.SetNoConversation()
		return
	}
	if cv.Stack != nil {
		cv.Stack.SetVisibleChildName("chat")
	}
	cv.TopBar.SetInfo(name, tex)
}

func (cv *ChatView) ToggleSearch() {
	cv.SearchBar.Toggle()
}

func (cv *ChatView) FocusEntry() {
	cv.InputBar.GrabFocus()
}

func (cv *ChatView) SetPinnedMessage(content string) {
	cv.MessageList.SetPinnedMessage(content)
}

func (cv *ChatView) Clear() {
	cv.MessageList.Clear()
}

func (cv *ChatView) ScrollToBottom() {
	cv.MessageList.ScrollToBottom()
}

func (cv *ChatView) ScrollToMessage(id string) {
	cv.MessageList.ScrollToMessage(id)
}

func (cv *ChatView) HighlightMessage(id string) {
	cv.MessageList.HighlightMessage(id)
}

func (cv *ChatView) SetLoadingOlder(loading bool) {
	cv.MessageList.SetLoadingOlder(loading)
}

// Passthrough Bubble Adders
func (cv *ChatView) AddSeparator(text string) {
	cv.MessageList.AddSeparator(text)
}

func (cv *ChatView) AddMessage(id, jid, name, text string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string) {
	cv.MessageList.AddMessage(id, jid, name, text, isSelf, isCont, status, tStr, av, qID, qSender, qContent)
}

func (cv *ChatView) AddImage(id, jid, name, text string, tex, thumb *gdk.Texture, path string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string, w, h int) {
	cv.MessageList.AddImage(id, jid, name, text, tex, thumb, path, isSelf, isCont, status, tStr, av, qID, qSender, qContent, w, h)
}

func (cv *ChatView) AddSticker(id, jid, name string, anim *gdkpixbuf.PixbufAnimation, tex, thumb *gdk.Texture, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string, w, h int) {
	cv.MessageList.AddSticker(id, jid, name, anim, tex, thumb, isSelf, isCont, status, tStr, av, qID, qSender, qContent, w, h)
}

func (cv *ChatView) AddAudio(id, jid, name string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string) {
	cv.MessageList.AddAudio(id, jid, name, isSelf, isCont, status, tStr, av, qID, qSender, qContent)
}

func (cv *ChatView) AddVideo(id, jid, name, text string, thumb *gdk.Texture, path string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string, w, h int) {
	cv.MessageList.AddVideo(id, jid, name, text, thumb, path, isSelf, isCont, status, tStr, av, qID, qSender, qContent, w, h)
}

func (cv *ChatView) AddDocument(id, jid, name, fileName string, thumb *gdk.Texture, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string) {
	cv.MessageList.AddDocument(id, jid, name, fileName, thumb, isSelf, isCont, status, tStr, av, qID, qSender, qContent)
}

func (cv *ChatView) AddPoll(id, jid, name, question string, options []string, votes map[string][]string, myJID string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string) {
	cv.MessageList.AddPoll(id, jid, name, question, options, votes, myJID, isSelf, isCont, status, tStr, av, qID, qSender, qContent)
}

// Passthrough Bubble Updaters
func (cv *ChatView) UpdatePollVotes(msgID string, votes map[string][]string, myJID string) {
	cv.MessageList.UpdatePollVotes(msgID, votes, myJID)
}

func (cv *ChatView) UpdateMessageStatus(id, status string) {
	cv.MessageList.UpdateMessageStatus(id, status)
}

func (cv *ChatView) UpdateMessageContent(id, content string, isEdited bool) {
	cv.MessageList.UpdateMessageContent(id, content, isEdited)
}

func (cv *ChatView) UpdateMessageReactions(id string, reactions []string) {
	cv.MessageList.UpdateMessageReactions(id, reactions)
}

func (cv *ChatView) UpdateMessagePinned(id string, pinned bool) {
	cv.MessageList.UpdateMessagePinned(id, pinned)
}

func (cv *ChatView) UpdateMessageForwarded(id string, forwarded bool) {
	cv.MessageList.UpdateMessageForwarded(id, forwarded)
}

func (cv *ChatView) UpdateMessageSticker(id string, anim *gdkpixbuf.PixbufAnimation, tex *gdk.Texture, path string) {
	cv.MessageList.UpdateMessageSticker(id, anim, tex, path)
}

func (cv *ChatView) UpdateMessageImage(id string, tex *gdk.Texture, path string) {
	cv.MessageList.UpdateMessageImage(id, tex, path)
}

func (cv *ChatView) UpdateMessageAudio(id string, path string) {
	cv.MessageList.UpdateMessageAudio(id, path)
}

func (cv *ChatView) UpdateMessageDocument(id string, path string) {
	cv.MessageList.UpdateMessageDocument(id, path)
}

func (cv *ChatView) SetAvatar(jid string, tex *gdk.Texture) {
	if bubbles, exists := cv.MessageList.BubblesByJID[jid]; exists {
		glib.IdleAdd(func() {
			for _, b := range bubbles {
				b.UpdateAvatar(tex)
			}
		})
	}
}

func (cv *ChatView) SetInputText(text string) {
	if cv.InputBar != nil {
		cv.InputBar.SetText(text)
	}
}

func (cv *ChatView) ClearInput() {
	if cv.InputBar != nil {
		cv.InputBar.CancelReply()
		cv.InputBar.SetText("")
	}
}

func (cv *ChatView) StopTyping() {
	if cv.OnStopTyping != nil {
		cv.OnStopTyping()
	}
}

func (cv *ChatView) SetTopBarInfo(name string, tex *gdk.Texture) {
	if cv.TopBar != nil {
		cv.TopBar.SetInfo(name, tex)
	}
}
