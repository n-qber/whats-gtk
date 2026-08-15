package chat

import (
	"context"

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

	// High-level Callbacks (consumed by bridge)
	OnSendMessage         func(text string, replyToID string)
	OnPasteImage          func(tex *gdk.Texture)
	OnSendFile            func(path string)
	OnDownloadMedia       func(id string)
	OnOpenImage           func(path string)
	OnSendReaction        func(id, emoji string)
	OnPinMessage          func(id string, pin bool, duration uint32)
	OnDetach              func()
	OnLoadOlder           func()
	OnLoadMessageRequest  func(id string)
	OnSearchMessages      func(query string)
	OnCancelSearch        func()
	OnCancelSearchAndJump func(id string)
	OnSearchResultClick   func(id string)
	OnMentionClick        func(jid string)
	OnSendPollVote        func(msgID string, senderJID string, isFromMe bool, selectedOptions []string)
	OnHeaderClick         func()
	OnForwardMessages     func(ids []string)

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

	// Wire InputBar
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

	// Set initial state to static empty background
	cv.Stack.SetVisibleChildName("empty")

	box.Append(cv.Stack)

	return cv, nil
}

// Proxies for legacy compatibility

func (cv *ChatView) SetNoConversation() {
	cv.Clear()
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
