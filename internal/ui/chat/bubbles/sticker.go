package bubbles

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type StickerBubble struct {
	*baseBubble
	picture           *gtk.Picture
	placeholder       *gtk.Box
	OnDownloadRequest func()
}

func NewStickerBubble(name string, pixbuf, thumb *gdk.Texture, isSelf bool, status, time string, avatar *gdk.Texture, realW, realH int) (*StickerBubble, error) {
	overlay := gtk.NewOverlay()
	
	picture := gtk.NewPicture()
	picture.SetContentFit(gtk.ContentFitCover)
	picture.SetCanShrink(true)
	picture.AddCSSClass("message-sticker")

	placeholder := gtk.NewBox(gtk.OrientationVertical, 0)
	placeholder.AddCSSClass("sticker-placeholder")
	placeholder.SetSizeRequest(160, 160)
	placeholder.SetHAlign(gtk.AlignCenter)
	placeholder.SetVAlign(gtk.AlignCenter)
	placeholder.SetHExpand(false)
	placeholder.SetVExpand(false)
	
	downloadIcon := gtk.NewImageFromIconName("folder-download-symbolic")
	downloadIcon.SetPixelSize(48)
	downloadIcon.SetVAlign(gtk.AlignCenter)
	downloadIcon.SetHAlign(gtk.AlignCenter)
	placeholder.Append(downloadIcon)

	overlay.SetChild(picture)
	overlay.AddOverlay(placeholder)
	
	// Alignment depends on whether it's in a bubble or not
	overlay.SetHAlign(gtk.AlignStart)
	overlay.SetVAlign(gtk.AlignStart)
	overlay.SetHExpand(false)
	overlay.SetVExpand(false)

	displayTex := pixbuf
	if displayTex == nil {
		displayTex = thumb
	}

	if displayTex != nil {
		placeholder.Hide()
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
		overlay.SetSizeRequest(int(w), int(h))
	} else {
		picture.Hide()
		placeholder.Show()
		overlay.SetSizeRequest(160, 160)
	}

	click := gtk.NewGestureClick()
	overlay.AddController(click)

	base, err := newBaseBubble(name, "[Sticker]", overlay, isSelf, false, status, time, avatar)
	if err != nil {
		return nil, err
	}

	sb := &StickerBubble{baseBubble: base, picture: picture, placeholder: placeholder}

	click.ConnectPressed(func(n int, x, y float64) {
		if sb.OnDownloadRequest != nil {
			sb.OnDownloadRequest()
		}
	})

	return sb, nil
}

func (sb *StickerBubble) UpdateImage(tex *gdk.Texture) {
	if tex != nil {
		sb.placeholder.Hide()
		sb.picture.Show()
		sb.picture.SetPaintable(tex)
		w, h := float64(tex.Width()), float64(tex.Height())
		if w > 160 || h > 160 {
			ratio := 160.0 / w
			if h > w { ratio = 160.0 / h }
			w *= ratio
			h *= ratio
		}
		sb.picture.SetSizeRequest(int(w), int(h))
		if widget, ok := sb.picture.Parent().(*gtk.Widget); ok {
			widget.SetSizeRequest(int(w), int(h))
		}
	}
}
