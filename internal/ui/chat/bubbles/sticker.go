package bubbles

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type StickerBubble struct {
	*baseBubble
	picture           *gtk.Picture
	placeholder       *gtk.Box
	OnDownloadRequest func()
	animIter          *gdkpixbuf.PixbufAnimationIter
	hasTickCallback   bool
}

func NewStickerBubble(name string, anim *gdkpixbuf.PixbufAnimation, pixbuf, thumb *gdk.Texture, isSelf bool, status, time string, avatar *gdk.Texture, realW, realH int) (*StickerBubble, error) {
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

	if anim != nil && !anim.IsStaticImage() {
		placeholder.Hide()
		picture.Show()
		if pb := anim.Iter(nil).Pixbuf(); pb != nil {
			picture.SetPaintable(gdk.NewTextureForPixbuf(pb))
			w, h := float64(pb.Width()), float64(pb.Height())
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
		}
	} else if displayTex != nil {
		placeholder.Hide()
		picture.Show()
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

	if anim != nil && !anim.IsStaticImage() {
		sb.animIter = anim.Iter(nil)
		sb.hasTickCallback = true
		sb.picture.AddTickCallback(func(widget gtk.Widgetter, frameClock gdk.FrameClocker) bool {
			if sb.animIter != nil {
				if sb.animIter.Advance(nil) {
					if pb := sb.animIter.Pixbuf(); pb != nil {
						sb.picture.SetPaintable(gdk.NewTextureForPixbuf(pb))
					}
				}
				return true
			}
			return false
		})
	}

	click.ConnectPressed(func(n int, x, y float64) {
		if sb.OnDownloadRequest != nil {
			sb.OnDownloadRequest()
		}
	})

	return sb, nil
}

func (sb *StickerBubble) UpdateImage(tex *gdk.Texture, path string) {
	sb.UpdateStickerImage(nil, tex, path)
}

func (sb *StickerBubble) UpdateStickerImage(anim *gdkpixbuf.PixbufAnimation, tex *gdk.Texture, path string) {
	if anim != nil && !anim.IsStaticImage() {
		sb.animIter = anim.Iter(nil)
		sb.placeholder.Hide()
		sb.picture.Show()
		pb := sb.animIter.Pixbuf()
		if pb != nil {
			sb.picture.SetPaintable(gdk.NewTextureForPixbuf(pb))
		}
		
		if !sb.hasTickCallback {
			sb.hasTickCallback = true
			sb.picture.AddTickCallback(func(widget gtk.Widgetter, frameClock gdk.FrameClocker) bool {
				if sb.animIter != nil {
					if sb.animIter.Advance(nil) {
						if pb := sb.animIter.Pixbuf(); pb != nil {
							sb.picture.SetPaintable(gdk.NewTextureForPixbuf(pb))
						}
					}
					return true
				}
				return false
			})
		}
		
		// Update size for animation first frame
		if pb != nil {
			w, h := float64(pb.Width()), float64(pb.Height())
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
	} else if tex != nil {
		sb.animIter = nil // stop ticking
		sb.hasTickCallback = false // next time it becomes animated, add it
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
