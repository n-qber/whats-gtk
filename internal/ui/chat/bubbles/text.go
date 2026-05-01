package bubbles

import (
	"github.com/gotk3/gotk3/gtk"
)

type Bubble interface {
	Widget() gtk.IWidget
}

type TextBubble struct {
	box *gtk.Box
}

func NewTextBubble(text string, isSelf bool) (*TextBubble, error) {
	box, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		return nil, err
	}

	label, _ := gtk.LabelNew(text)
	label.SetLineWrap(true)
	label.SetMaxWidthChars(50)
	
	if isSelf {
		box.PackEnd(label, false, false, 10)
	} else {
		box.PackStart(label, false, false, 10)
	}

	return &TextBubble{box: box}, nil
}

func (b *TextBubble) Widget() gtk.IWidget {
	return b.box
}
