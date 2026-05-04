package bubbles

import (
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

type PollBubble struct {
	*baseBubble
}

func NewPollBubble(name string, question string, options []string, isSelf bool, status string, time string, avatar *gdk.Pixbuf) (*PollBubble, error) {
	box, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 5)
	if err != nil {
		return nil, err
	}

	label, _ := gtk.LabelNew(question)
	label.SetXAlign(0)
	label.SetMarkup("<b>" + question + "</b>")
	box.PackStart(label, false, false, 2)

	for _, opt := range options {
		btn, _ := gtk.CheckButtonNewWithLabel(opt)
		box.PackStart(btn, false, false, 0)
	}

	voteBtn, _ := gtk.ButtonNewWithLabel("Vote")
	box.PackStart(voteBtn, false, false, 5)

	base, err := newBaseBubble(name, question, box, isSelf, true, status, time, avatar)
	if err != nil {
		return nil, err
	}

	return &PollBubble{baseBubble: base}, nil
}

