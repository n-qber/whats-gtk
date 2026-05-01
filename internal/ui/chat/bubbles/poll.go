package bubbles

import (
	"github.com/gotk3/gotk3/gtk"
)

type PollBubble struct {
	box *gtk.Box
}

func NewPollBubble(question string, options []string, isSelf bool) (*PollBubble, error) {
	box, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 5)
	if err != nil {
		return nil, err
	}

	label, _ := gtk.LabelNew(question)
	label.SetXAlign(0)
	box.PackStart(label, false, false, 2)

	for _, opt := range options {
		btn, _ := gtk.CheckButtonNewWithLabel(opt)
		box.PackStart(btn, false, false, 0)
	}

	voteBtn, _ := gtk.ButtonNewWithLabel("Vote")
	box.PackStart(voteBtn, false, false, 5)

	mainBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if isSelf {
		mainBox.PackEnd(box, false, false, 10)
	} else {
		mainBox.PackStart(box, false, false, 10)
	}

	return &PollBubble{box: mainBox}, nil
}

func (b *PollBubble) Widget() gtk.IWidget {
	return b.box
}
