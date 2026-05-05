package bubbles

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type PollBubble struct {
	*baseBubble
}

func NewPollBubble(name, question string, options []string, isSelf bool, status, time string, avatar *gdk.Texture) (*PollBubble, error) {
	label := gtk.NewLabel("[Poll] " + question)
	base, err := newBaseBubble(name, question, label, isSelf, true, status, time, avatar)
	if err != nil {
		return nil, err
	}
	return &PollBubble{baseBubble: base}, nil
}
