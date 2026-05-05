package bubbles

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type StickerBubble struct {
	*baseBubble
	picture *gtk.Picture
}
func NewStickerBubble(name string, pixbuf, thumb *gdk.Texture, isSelf bool, status, time string, avatar *gdk.Texture, realW, realH int) (*StickerBubble, error) {
	picture := gtk.NewPicture()
	picture.SetContentFit(gtk.ContentFitScaleDown)
	picture.SetCanShrink(true)

	displayTex := pixbuf
	if displayTex == nil {
		displayTex = thumb
	}

	if displayTex != nil {
		picture.SetPaintable(displayTex)

		w, h := float64(displayTex.Width()), float64(displayTex.Height())
		if realW > 0 && realH > 0 {
			w, h = float64(realW), float64(realH)
		}

		if w > 160 || h > 160 {
			ratio := 160.0 / w
			if h > w { ratio = 160.0 / h }
			w *= ratio
			h *= ratio
		}
		picture.SetSizeRequest(int(w), int(h))
	}

	picture.AddCSSClass("message-sticker")


	base, err := newBaseBubble(name, "[Sticker]", picture, isSelf, false, status, time, avatar)
	if err != nil {
		return nil, err
	}
	return &StickerBubble{baseBubble: base, picture: picture}, nil
}

func (sb *StickerBubble) UpdateImage(tex *gdk.Texture) {
	if tex != nil {
		sb.picture.SetPaintable(tex)
		w, h := float64(tex.Width()), float64(tex.Height())
		if w > 160 || h > 160 {
			ratio := 160.0 / w
			if h > w { ratio = 160.0 / h }
			w *= ratio
			h *= ratio
		}
		sb.picture.SetSizeRequest(int(w), int(h))
	}
}
