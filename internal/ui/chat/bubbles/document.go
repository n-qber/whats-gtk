package bubbles

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

type DocumentBubble struct {
	*baseBubble
	OnDownloadRequest func()
	downloadBtn       *gtk.Button
	filePath          string
}

func NewDocumentBubble(name, fileName string, isSelf bool, status, time string, avatar *gdk.Texture) (*DocumentBubble, error) {
	box := gtk.NewBox(gtk.OrientationHorizontal, 10)
	box.SetMarginTop(4)
	box.SetMarginBottom(4)
	box.SetMarginStart(4)
	box.SetMarginEnd(4)

	icon := gtk.NewImageFromIconName("document-x-generic-symbolic")
	icon.SetPixelSize(32)
	
	nameLabel := gtk.NewLabel(fileName)
	nameLabel.SetEllipsize(pango.EllipsizeEnd)
	nameLabel.SetMaxWidthChars(30)
	nameLabel.SetHExpand(true)
	nameLabel.SetXAlign(0)
	
	downloadBtn := gtk.NewButtonFromIconName("folder-download-symbolic")
	downloadBtn.SetHasFrame(false)

	box.Append(icon)
	box.Append(nameLabel)
	box.Append(downloadBtn)

	base, err := newBaseBubble(name, "[File: "+fileName+"]", box, isSelf, true, status, time, avatar)
	if err != nil {
		return nil, err
	}

	db := &DocumentBubble{
		baseBubble:  base,
		downloadBtn: downloadBtn,
	}

	downloadBtn.ConnectClicked(func() {
		if db.filePath != "" {
			// [TODO: Open file]
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
	}
}
