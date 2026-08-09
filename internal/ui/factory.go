package ui

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

// LabelOption defines a functional option for configuring a gtk.Label.
type LabelOption func(*gtk.Label)

// WithCSS adds one or more CSS classes to the label.
func WithCSS(classes ...string) LabelOption {
	return func(l *gtk.Label) {
		for _, c := range classes {
			l.AddCSSClass(c)
		}
	}
}

// WithAlign sets horizontal alignment for the label.
func WithAlign(hAlign gtk.Align) LabelOption {
	return func(l *gtk.Label) {
		l.SetHAlign(hAlign)
	}
}

// WithWrap enables text wrapping with WordChar wrap mode.
func WithWrap() LabelOption {
	return func(l *gtk.Label) {
		l.SetWrap(true)
		l.SetWrapMode(pango.WrapWordChar)
	}
}

// WithXAlign sets the x-alignment ratio (0.0 to 1.0).
func WithXAlign(xAlign float32) LabelOption {
	return func(l *gtk.Label) {
		l.SetXAlign(xAlign)
	}
}

// NewLabel constructs a configured gtk.Label cleanly in one line.
func NewLabel(text string, opts ...LabelOption) *gtk.Label {
	l := gtk.NewLabel(text)
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// NewBox constructs a configured gtk.Box with optional CSS classes.
func NewBox(orientation gtk.Orientation, spacing int, cssClasses ...string) *gtk.Box {
	box := gtk.NewBox(orientation, spacing)
	for _, css := range cssClasses {
		box.AddCSSClass(css)
	}
	return box
}

// NewButton constructs a gtk.Button with label and optional CSS classes.
func NewButton(label string, cssClasses ...string) *gtk.Button {
	btn := gtk.NewButtonWithLabel(label)
	for _, css := range cssClasses {
		btn.AddCSSClass(css)
	}
	return btn
}
