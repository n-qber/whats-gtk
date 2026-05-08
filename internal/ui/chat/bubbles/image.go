package bubbles

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type ImageBubble struct {
	*baseBubble
	picture           *gtk.Picture
	placeholder       *gtk.Box
	OnDownloadRequest func()
}

func NewImageBubble(name string, tex, thumb *gdk.Texture, isSelf bool, status, time string, avatar *gdk.Texture, realW, realH int) (*ImageBubble, error) {
	overlay := gtk.NewOverlay()

	picture := gtk.NewPicture()
	picture.SetContentFit(gtk.ContentFitCover)
	picture.SetCanShrink(true)
	picture.AddCSSClass("message-image")

	placeholder := gtk.NewBox(gtk.OrientationVertical, 0)
	placeholder.AddCSSClass("image-placeholder")
	placeholder.SetSizeRequest(200, 150)
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
	overlay.SetHAlign(gtk.AlignStart)
	overlay.SetVAlign(gtk.AlignStart)

	displayTex := tex
	if displayTex == nil && thumb != nil {
		displayTex = thumb
	}

	if displayTex != nil {
		placeholder.Hide()
		picture.SetPaintable(displayTex)

		w, h := float64(displayTex.Width()), float64(displayTex.Height())
		if realW > 0 && realH > 0 {
			w, h = float64(realW), float64(realH)
		}

		if w > 300 || h > 300 {
			ratio := 300.0 / w
			if h > w { ratio = 300.0 / h }
			w *= ratio
			h *= ratio
		}
		picture.SetSizeRequest(int(w), int(h))
	} else {
		picture.Hide()
		placeholder.Show()
	}

	click := gtk.NewGestureClick()
	overlay.AddController(click)

	base, err := newBaseBubble(name, "[Image]", overlay, isSelf, true, status, time, avatar)

	if err != nil {
		return nil, err
	}

	ib := &ImageBubble{baseBubble: base, picture: picture, placeholder: placeholder}

	click.ConnectPressed(func(n int, x, y float64) {
		if ib.OnDownloadRequest != nil {
			ib.OnDownloadRequest()
		}
	})

	return ib, nil
}

func (ib *ImageBubble) UpdateImage(tex *gdk.Texture) {
	if tex != nil {
		ib.placeholder.Hide()
		ib.picture.Show()
		ib.picture.SetPaintable(tex)
		w, h := float64(tex.Width()), float64(tex.Height())
		if w > 300 || h > 300 {
			ratio := 300.0 / w
			if h > w { ratio = 300.0 / h }
			w *= ratio
			h *= ratio
		}
		ib.picture.SetSizeRequest(int(w), int(h))
	}
}
