package bubbles

import (
	"fmt"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type VideoBubble struct {
	*baseBubble
	overlay           *gtk.Overlay
	picture           *gtk.Picture
	placeholder       *gtk.Box
	actionBtn         *gtk.Box
	actionIcon        *gtk.Image
	spinner           *gtk.Spinner
	loadingBar        *gtk.ProgressBar
	captionLabel      *gtk.Label
	OnDownloadRequest func()
	OnOpenRequest     func(path string)
	filePath          string
	isDownloading     bool
	progressTick      glib.SourceHandle
}

func NewVideoBubble(name, text string, thumb *gdk.Texture, path string, isSelf bool, status, time string, avatar *gdk.Texture, realW, realH int) (*VideoBubble, error) {
	mainBox := gtk.NewBox(gtk.OrientationVertical, 5)

	overlay := gtk.NewOverlay()
	overlay.AddCSSClass("video-container")

	picture := gtk.NewPicture()
	picture.SetContentFit(gtk.ContentFitCover)
	picture.SetCanShrink(true)
	picture.AddCSSClass("message-image")

	placeholder := gtk.NewBox(gtk.OrientationVertical, 0)
	placeholder.AddCSSClass("video-placeholder")
	placeholder.SetHAlign(gtk.AlignFill)
	placeholder.SetVAlign(gtk.AlignFill)

	placeholderIcon := gtk.NewImageFromIconName("video-x-generic-symbolic")
	placeholderIcon.SetPixelSize(36)
	placeholderIcon.SetHAlign(gtk.AlignCenter)
	placeholderIcon.SetVAlign(gtk.AlignCenter)
	placeholderIcon.SetHExpand(true)
	placeholderIcon.SetVExpand(true)
	placeholder.Append(placeholderIcon)

	actionBtn := gtk.NewBox(gtk.OrientationHorizontal, 0)
	actionBtn.AddCSSClass("video-action-button")
	actionBtn.SetHAlign(gtk.AlignCenter)
	actionBtn.SetVAlign(gtk.AlignCenter)

	actionIcon := gtk.NewImageFromIconName("folder-download-symbolic")
	actionIcon.SetPixelSize(28)
	actionIcon.SetHAlign(gtk.AlignCenter)
	actionIcon.SetVAlign(gtk.AlignCenter)
	actionBtn.Append(actionIcon)

	spinner := gtk.NewSpinner()
	spinner.SetSizeRequest(28, 28)
	spinner.SetHAlign(gtk.AlignCenter)
	spinner.SetVAlign(gtk.AlignCenter)
	actionBtn.Append(spinner)
	spinner.Hide()

	loadingBar := gtk.NewProgressBar()
	loadingBar.AddCSSClass("video-loading-bar")
	loadingBar.AddCSSClass("media-progress")
	loadingBar.SetVAlign(gtk.AlignEnd)
	loadingBar.SetHAlign(gtk.AlignFill)
	loadingBar.SetHExpand(true)
	loadingBar.Hide()

	// Determine dimensions
	targetW, targetH := 240.0, 160.0
	if realW > 0 && realH > 0 {
		w, h := float64(realW), float64(realH)
		if w > 300 || h > 300 {
			ratio := 300.0 / w
			if h > w {
				ratio = 300.0 / h
			}
			w *= ratio
			h *= ratio
		}
		if w >= 120 && h >= 120 {
			targetW, targetH = w, h
		}
	}

	if thumb != nil {
		picture.SetPaintable(thumb)
		picture.SetSizeRequest(int(targetW), int(targetH))
		picture.Show()
		placeholder.Hide()
		overlay.SetChild(picture)
	} else {
		picture.Hide()
		placeholder.Show()
		placeholder.SetSizeRequest(int(targetW), int(targetH))
		overlay.SetChild(placeholder)
	}

	overlay.AddOverlay(actionBtn)
	overlay.AddOverlay(loadingBar)

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

	click := gtk.NewGestureClick()
	overlay.AddController(click)

	base, err := newBaseBubble(name, "[Video]", mainBox, isSelf, true, status, time, avatar)
	if err != nil {
		return nil, err
	}

	vb := &VideoBubble{
		baseBubble:   base,
		overlay:      overlay,
		picture:      picture,
		placeholder:  placeholder,
		actionBtn:    actionBtn,
		actionIcon:   actionIcon,
		spinner:      spinner,
		loadingBar:   loadingBar,
		captionLabel: captionLabel,
		filePath:     path,
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

	if path != "" {
		actionIcon.SetFromIconName("media-playback-start-symbolic")
		actionIcon.Show()
		spinner.Hide()
		loadingBar.Hide()
	} else if status == "pending" {
		actionIcon.Hide()
		spinner.Show()
		spinner.Start()
		loadingBar.Show()
		vb.startPulsing()
	} else {
		actionIcon.SetFromIconName("folder-download-symbolic")
		actionIcon.Show()
		spinner.Hide()
		loadingBar.Hide()
	}

	click.ConnectPressed(func(n int, x, y float64) {
		if vb.filePath != "" {
			if vb.OnOpenRequest != nil {
				vb.OnOpenRequest(vb.filePath)
			}
		} else {
			if !vb.isDownloading {
				vb.isDownloading = true
				vb.actionIcon.Hide()
				vb.spinner.Show()
				vb.spinner.Start()
				vb.loadingBar.Show()
				vb.startPulsing()
				if vb.OnDownloadRequest != nil {
					vb.OnDownloadRequest()
				}
			}
		}
	})

	return vb, nil
}

func (vb *VideoBubble) startPulsing() {
	if vb.progressTick == 0 {
		vb.progressTick = glib.TimeoutAdd(50, func() bool {
			if vb.loadingBar != nil && vb.loadingBar.Visible() {
				vb.loadingBar.Pulse()
				return true
			}
			vb.progressTick = 0
			return false
		})
	}
}

func (vb *VideoBubble) stopPulsing() {
	if vb.progressTick != 0 {
		glib.SourceRemove(vb.progressTick)
		vb.progressTick = 0
	}
}

func (vb *VideoBubble) SetProgress(fraction float64) {
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1.0 {
		fraction = 1.0
	}
	glib.IdleAdd(func() {
		vb.stopPulsing()
		if vb.loadingBar != nil {
			vb.loadingBar.Show()
			vb.loadingBar.SetFraction(fraction)
		}
		if fraction > 0 && fraction < 1.0 {
			if vb.actionIcon != nil {
				vb.actionIcon.Hide()
			}
			if vb.spinner != nil {
				vb.spinner.Show()
				vb.spinner.Start()
			}
		}
		if vb.StatusLabel != nil {
			percent := int(fraction * 100)
			vb.StatusLabel.SetText(fmt.Sprintf("🕒 %d%%", percent))
		}
	})
}

func (vb *VideoBubble) SetStatus(status string) {
	vb.baseBubble.SetStatus(status)
	glib.IdleAdd(func() {
		if status == "pending" {
			vb.loadingBar.Show()
			if vb.actionIcon != nil {
				vb.actionIcon.Hide()
			}
			if vb.spinner != nil {
				vb.spinner.Show()
				vb.spinner.Start()
			}
			vb.startPulsing()
		} else if status == "failed" {
			vb.isDownloading = false
			vb.stopPulsing()
			if vb.loadingBar != nil {
				vb.loadingBar.Hide()
			}
			if vb.spinner != nil {
				vb.spinner.Stop()
				vb.spinner.Hide()
			}
			if vb.actionIcon != nil {
				vb.actionIcon.SetFromIconName("folder-download-symbolic")
				vb.actionIcon.Show()
			}
		} else {
			if vb.filePath != "" {
				vb.isDownloading = false
				vb.stopPulsing()
				if vb.loadingBar != nil {
					vb.loadingBar.Hide()
				}
				if vb.spinner != nil {
					vb.spinner.Stop()
					vb.spinner.Hide()
				}
				if vb.actionIcon != nil {
					vb.actionIcon.SetFromIconName("media-playback-start-symbolic")
					vb.actionIcon.Show()
				}
			}
		}
	})
}

func (vb *VideoBubble) UpdateVideo(path string) {
	glib.IdleAdd(func() {
		if path != "" {
			vb.filePath = path
		}
		vb.isDownloading = false
		vb.stopPulsing()
		if vb.loadingBar != nil {
			vb.loadingBar.Hide()
		}
		if vb.spinner != nil {
			vb.spinner.Stop()
			vb.spinner.Hide()
		}
		if vb.actionIcon != nil {
			vb.actionIcon.SetFromIconName("media-playback-start-symbolic")
			vb.actionIcon.Show()
		}
	})
}

func (vb *VideoBubble) UpdateImage(tex *gdk.Texture, path string) {
	glib.IdleAdd(func() {
		if path != "" {
			vb.UpdateVideo(path)
		}
		if tex != nil {
			vb.placeholder.Hide()
			vb.picture.Show()
			vb.picture.SetPaintable(tex)
			vb.overlay.SetChild(vb.picture)
		}
	})
}

func (vb *VideoBubble) SetFilePath(path string) {
	vb.filePath = path
	if path != "" {
		vb.UpdateVideo(path)
	}
}

func (vb *VideoBubble) MediaPath() string {
	return vb.filePath
}
