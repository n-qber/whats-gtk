package bubbles

import (
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

type TextBubble struct {
	*baseBubble
}

func NewTextBubble(name string, text string, isSelf bool, status string, time string, avatar *gdk.Pixbuf) (*TextBubble, error) {
	label, err := gtk.LabelNew(text)
	if err != nil {
		return nil, err
	}
	label.SetLineWrap(true)
	label.SetMaxWidthChars(60)
	label.SetXAlign(0)
	label.SetSelectable(true)

	base, err := newBaseBubble(name, label, isSelf, true, status, time, avatar)
	if err != nil {
		return nil, err
	}

	return &TextBubble{baseBubble: base}, nil
}
