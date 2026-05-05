package bubbles

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type ImageBubble struct {
	*baseBubble
	picture           *gtk.Picture
	OnDownloadRequest func()
}
func NewImageBubble(name string, tex, thumb *gdk.Texture, isSelf bool, status, time string, avatar *gdk.Texture, realW, realH int) (*ImageBubble, error) {
	picture := gtk.NewPicture()
	picture.SetContentFit(gtk.ContentFitScaleDown)
	picture.SetCanShrink(true)

	displayTex := tex
	if displayTex == nil && thumb != nil {
		displayTex = thumb
	}

	if displayTex != nil {
		picture.SetPaintable(displayTex)

		w, h := float64(displayTex.Width()), float64(displayTex.Height())
		if realW > 0 && realH > 0 {
			w, h = float64(realW), float64(realH)
		}

		if w > 300 {
			h = (300 / w) * h
			w = 300
		}
		picture.SetSizeRequest(int(w), int(h))
	}


	picture.AddCSSClass("message-image")

	click := gtk.NewGestureClick()
	picture.AddController(click)

	base, err := newBaseBubble(name, "[Image]", picture, isSelf, true, status, time, avatar)

	if err != nil {
		return nil, err
	}

	ib := &ImageBubble{baseBubble: base, picture: picture}

	click.ConnectPressed(func(n int, x, y float64) {
		if ib.OnDownloadRequest != nil {
			ib.OnDownloadRequest()
		}
	})

	return ib, nil
}

func (ib *ImageBubble) UpdateImage(tex *gdk.Texture) {
	if tex != nil {
		ib.picture.SetPaintable(tex)
		w, h := float64(tex.Width()), float64(tex.Height())
		if w > 300 {
			h = (300 / w) * h
			w = 300
		}
		ib.picture.SetSizeRequest(int(w), int(h))
	}
}
