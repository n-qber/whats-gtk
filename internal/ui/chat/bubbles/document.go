package bubbles

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

type DocumentBubble struct {
	*baseBubble
	OnDownloadRequest func()
	icon              *gtk.Image
	nameLabel         *gtk.Label
	downloadBtn       *gtk.Button
	filePath          string
}

func NewDocumentBubble(name, fileName string, thumb *gdk.Texture, isSelf bool, status, time string, avatar *gdk.Texture) (*DocumentBubble, error) {
	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)
	mainBox.AddCSSClass("document-bubble-main")

	var icon *gtk.Image
	if thumb != nil {
		icon = gtk.NewImageFromPaintable(thumb)
		icon.SetPixelSize(-1)
		icon.AddCSSClass("document-preview-image")
		icon.SetVExpand(true)
		icon.SetOverflow(gtk.OverflowHidden)
	} else {
		icon = gtk.NewImageFromIconName("document-x-generic-symbolic")
		icon.SetPixelSize(48)
		icon.SetMarginTop(10)
		icon.SetMarginBottom(10)
	}

	infoBox := gtk.NewBox(gtk.OrientationHorizontal, 10)
	infoBox.AddCSSClass("document-info-box")
	infoBox.SetMarginTop(6)
	infoBox.SetMarginBottom(6)
	infoBox.SetMarginStart(10)
	infoBox.SetMarginEnd(10)

	nameLabel := gtk.NewLabel(fileName)
	nameLabel.SetEllipsize(pango.EllipsizeEnd)
	nameLabel.SetMaxWidthChars(30)
	nameLabel.SetHExpand(true)
	nameLabel.SetXAlign(0)
	
	downloadBtn := gtk.NewButtonFromIconName("folder-download-symbolic")
	downloadBtn.SetHasFrame(false)

	infoBox.Append(nameLabel)
	infoBox.Append(downloadBtn)

	mainBox.Append(icon)
	mainBox.Append(infoBox)

	base, err := newBaseBubble(name, "[File: "+fileName+"]", mainBox, isSelf, true, status, time, avatar)
	if err != nil {
		return nil, err
	}

	db := &DocumentBubble{
		baseBubble:  base,
		icon:        icon,
		nameLabel:   nameLabel,
		downloadBtn: downloadBtn,
	}

	downloadBtn.ConnectClicked(func() {
		if db.filePath != "" {
			exec.Command("xdg-open", db.filePath).Start()
			return
		}
		if db.OnDownloadRequest != nil {
			db.OnDownloadRequest()
		}
	})

	return db, nil
}

func (db *DocumentBubble) UpdateDocument(path string) {
	db.filePath = path
	if path != "" {
		db.downloadBtn.SetIconName("document-open-symbolic")
		// Update filename from path if it was 'file'
		if db.nameLabel.Text() == "file" {
			base := filepath.Base(path)
			if idx := strings.Index(base, "_"); idx != -1 {
				db.nameLabel.SetText(base[idx+1:])
			} else {
				db.nameLabel.SetText(base)
			}
		}
	}
}

