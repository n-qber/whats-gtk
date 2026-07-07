package bubbles

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type PollBubble struct {
	*baseBubble
	msgID        string
	options      []string
	checkButtons map[string]*gtk.CheckButton
	voteLabels   map[string]*gtk.Label
	onVote       func(selected []string)
	ignoreToggle bool
}

func NewPollBubble(msgID, name, question string, options []string, isSelf bool, status, timeStr string, avatar *gdk.Texture) (*PollBubble, error) {
	pb := &PollBubble{
		msgID:        msgID,
		options:      options,
		checkButtons: make(map[string]*gtk.CheckButton),
		voteLabels:   make(map[string]*gtk.Label),
	}

	box := gtk.NewBox(gtk.OrientationVertical, 4)
	
	qLabel := gtk.NewLabel(question)
	qLabel.SetXAlign(0)
	qLabel.SetWrap(true)
	qLabel.AddCSSClass("message-content")
	qLabel.AddCSSClass("poll-question")
	box.Append(qLabel)

	for _, opt := range options {
		optHash := hashOption(opt)
		
		optBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
		optBox.SetMarginTop(4)
		
		chk := gtk.NewCheckButtonWithLabel(opt)
		chk.SetHExpand(true)
		
		// Capture opt for the closure
		capturedOpt := opt
		chk.ConnectToggled(func() {
			if pb.ignoreToggle {
				return
			}
			if pb.onVote != nil {
				// We just send the single selected vote for now (or collect all selected)
				var selected []string
				for o, c := range pb.checkButtons {
					if c.Active() {
						selected = append(selected, o)
					}
				}
				// If we just toggled, and we need to immediately vote:
				// It might be better to just re-vote with the current state of all checkboxes.
				pb.onVote(selected)
			}
		})
		
		vLabel := gtk.NewLabel("")
		vLabel.AddCSSClass("poll-vote-count")
		
		pb.checkButtons[capturedOpt] = chk
		pb.voteLabels[optHash] = vLabel
		
		optBox.Append(chk)
		optBox.Append(vLabel)
		box.Append(optBox)
	}

	base, err := newBaseBubble(name, "", box, isSelf, true, status, timeStr, avatar)
	if err != nil {
		return nil, err
	}
	pb.baseBubble = base

	return pb, nil
}

func (pb *PollBubble) SetOnVote(f func(selected []string)) {
	pb.onVote = f
}

func (pb *PollBubble) UpdateVotes(votes map[string][]string, myJID string) {
	pb.ignoreToggle = true
	defer func() { pb.ignoreToggle = false }()

	for _, opt := range pb.options {
		optHash := hashOption(opt)
		
		voters := votes[optHash]
		count := len(voters)
		
		// Update label
		if count > 0 {
			pb.voteLabels[optHash].SetText(fmt.Sprintf("%d", count))
		} else {
			pb.voteLabels[optHash].SetText("")
		}
		
		// Update checkbox state (if I voted for this)
		iVoted := false
		for _, v := range voters {
			if v == myJID {
				iVoted = true
				break
			}
		}
		pb.checkButtons[opt].SetActive(iVoted)
	}
}

func hashOption(opt string) string {
	h := sha256.Sum256([]byte(opt))
	return hex.EncodeToString(h[:])
}
