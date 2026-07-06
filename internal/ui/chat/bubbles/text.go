package bubbles

import (
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

type TextBubble struct {
	*baseBubble
}

func NewTextBubble(name, text string, isSelf bool, status, time string, avatar *gdk.Texture) (*TextBubble, error) {
	label := gtk.NewLabel("")
	label.SetMarkup(text)
	label.SetWrap(true)
	label.SetWrapMode(pango.WrapWordChar)
	label.SetMaxWidthChars(60)
	label.SetXAlign(0)
	label.SetSelectable(true)

	base, err := newBaseBubble(name, text, label, isSelf, true, status, time, avatar)
	if err != nil {
		return nil, err
	}

	label.ConnectActivateLink(func(uri string) bool {
		if strings.HasPrefix(uri, "mention:") {
			jid := strings.TrimPrefix(uri, "mention:")
			if base.onMentionClick != nil {
				base.onMentionClick(jid)
			}
			return true
		}
		return false
	})

	return &TextBubble{baseBubble: base}, nil
}
