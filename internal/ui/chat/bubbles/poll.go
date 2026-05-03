package bubbles

import (
	"github.com/gotk3/gotk3/gtk"
)

type PollBubble struct {
	*baseBubble
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

	base, err := newBaseBubble("", box, isSelf, true, "", "", nil)
	if err != nil {
		return nil, err
	}

	return &PollBubble{baseBubble: base}, nil
}
