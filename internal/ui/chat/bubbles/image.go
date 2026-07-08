package bubbles

import (
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type ImageBubble struct {
	*baseBubble
	picture           *gtk.Picture
	placeholder       *gtk.Box
	captionLabel      *gtk.Label
	OnDownloadRequest func()
	OnOpenRequest     func(path string)
	filePath          string
}

func NewImageBubble(name, text string, tex, thumb *gdk.Texture, isSelf bool, status, time string, avatar *gdk.Texture, realW, realH int) (*ImageBubble, error) {
	mainBox := gtk.NewBox(gtk.OrientationVertical, 5)

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

	mainBox.Append(overlay)

	var captionLabel *gtk.Label
	if text != "" {
		captionLabel = gtk.NewLabel("")
		captionLabel.SetMarkup(text)
		captionLabel.SetWrap(true)
		captionLabel.SetXAlign(0)
		captionLabel.AddCSSClass("image-caption")
		mainBox.Append(captionLabel)
	}

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

	content := "[Image]"
	if text != "" {
		content = text
	}
	base, err := newBaseBubble(name, content, mainBox, isSelf, true, status, time, avatar)

	if err != nil {
		return nil, err
	}

	ib := &ImageBubble{
		baseBubble: base,
		picture:    picture,
		placeholder: placeholder,
		captionLabel: captionLabel,
	}

	if captionLabel != nil {
		captionLabel.ConnectActivateLink(func(uri string) bool {
			if strings.HasPrefix(uri, "mention:") {
				jid := strings.TrimPrefix(uri, "mention:")
				if base.onMentionClick != nil {
					base.onMentionClick(jid)
				}
				return true
			}
			return false
		})
	}

	click.ConnectPressed(func(n int, x, y float64) {
		if ib.filePath != "" {
			if ib.OnOpenRequest != nil {
				ib.OnOpenRequest(ib.filePath)
			}
		} else {
			if ib.OnDownloadRequest != nil {
				ib.OnDownloadRequest()
			}
		}
	})

	return ib, nil
}

func (ib *ImageBubble) UpdateImage(tex *gdk.Texture, path string) {
	if path != "" {
		ib.filePath = path
	}
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

func (ib *ImageBubble) SetFilePath(path string) {
	ib.filePath = path
}

func (ib *ImageBubble) MediaPath() string {
	return ib.filePath
}
