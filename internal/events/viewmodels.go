package events

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
)

// UIMessage represents a formatted, UI-agnostic message ready to be rendered.
type UIMessage struct {
	ID              string
	JID             string
	SenderName      string
	IsFromMe        bool
	IsContinuation  bool
	TimeString      string
	DateSeparator   string // If set, UI should render a date separator before this message.

	Type            string // "text", "image", "video", "audio", "document", "sticker", "poll", "revoked", "view_once"
	Content         string

	IsViewOnce      bool
	IsForwarded     bool
	IsEdited        bool
	IsPinned        bool
	Status          string // "sent", "delivered", "read", "error"

	// Reactions
	Reactions       []string

	// Media properties
	MediaData       []byte
	Thumbnail       []byte
	AudioPath       string
	DocumentPath    string
	Mimetype        string
	Duration        uint32

	// Rendered GTK objects (optional, could be decoupled further but helpful for GTK for now)
	Avatar          *gdk.Texture
	StickerAnim     *gdkpixbuf.PixbufAnimation

	// Reply/Quote properties
	QuotedMsgID     string
	QuotedMsgSender string
	QuotedMsgText   string

	// Poll properties
	PollOptions     []string
	PollVotes       map[string][]string
	MyJID           string
	MediaWidth      int
	MediaHeight     int
}

// ChatMessagesPayload is the payload for EventChatMessagesLoaded.
type ChatMessagesPayload struct {
	JID      string
	Messages []UIMessage
	Append   bool // If true, these messages are appended/prepended (e.g., loading older). If false, clear chat.
}

// LiveUIMessagePayload is the payload for EventLiveMessageReceived.
type LiveUIMessagePayload struct {
	JID       string
	Message   UIMessage
	IsSyncing bool
}

// SidebarItem represents a chat in the sidebar.
type SidebarItem struct {
	JID         string
	Name        string
	IsGroup     bool
	UnreadCount int
	IsPinned    bool
	Avatar      *gdk.Texture
}
