package chat

import (
	"path/filepath"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type MediaCaptionDialog struct {
	Window    *adw.Window
	Entry     *gtk.Entry
	OnConfirm func(caption string)
}

// ShowMediaCaptionDialog displays a preview and caption dialog before sending media.
func ShowMediaCaptionDialog(parent *gtk.Window, filePath string, tex *gdk.Texture, onConfirm func(caption string)) *MediaCaptionDialog {
	win := adw.NewWindow()
	win.SetTitle("Enviar Mídia")
	if parent != nil {
		win.SetTransientFor(parent)
	}
	win.SetModal(true)
	win.SetDefaultSize(460, 480)

	dialog := &MediaCaptionDialog{
		Window:    win,
		OnConfirm: onConfirm,
	}

	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)

	headerBar := adw.NewHeaderBar()
	cancelBtn := gtk.NewButtonWithLabel("Cancelar")
	cancelBtn.ConnectClicked(func() {
		win.Destroy()
	})
	headerBar.PackStart(cancelBtn)

	sendBtn := gtk.NewButtonWithLabel("Enviar")
	sendBtn.AddCSSClass("suggested-action")
	headerBar.PackEnd(sendBtn)

	mainBox.Append(headerBar)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 12)
	contentBox.SetMarginTop(12)
	contentBox.SetMarginBottom(16)
	contentBox.SetMarginStart(16)
	contentBox.SetMarginEnd(16)
	contentBox.SetVExpand(true)

	// Preview area
	previewBox := gtk.NewBox(gtk.OrientationVertical, 0)
	previewBox.SetVExpand(true)
	previewBox.SetHExpand(true)
	previewBox.SetHAlign(gtk.AlignCenter)
	previewBox.SetVAlign(gtk.AlignCenter)

	if tex != nil {
		pic := gtk.NewPictureForPaintable(tex)
		pic.SetContentFit(gtk.ContentFitContain)
		pic.SetSizeRequest(360, 280)
		previewBox.Append(pic)
	} else if filePath != "" {
		ext := strings.ToLower(filepath.Ext(filePath))
		if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp" || ext == ".gif" {
			file := gio.NewFileForPath(filePath)
			pic := gtk.NewPictureForFile(file)
			pic.SetContentFit(gtk.ContentFitContain)
			pic.SetSizeRequest(360, 280)
			previewBox.Append(pic)
		} else {
			iconBox := gtk.NewBox(gtk.OrientationVertical, 8)
			iconBox.SetHAlign(gtk.AlignCenter)
			iconName := "video-x-generic-symbolic"
			if !strings.Contains(ext, "mp4") && !strings.Contains(ext, "mkv") && !strings.Contains(ext, "webm") && !strings.Contains(ext, "mov") {
				iconName = "text-x-generic-symbolic"
			}
			img := gtk.NewImageFromIconName(iconName)
			img.SetPixelSize(72)
			iconBox.Append(img)

			nameLabel := gtk.NewLabel(filepath.Base(filePath))
			nameLabel.AddCSSClass("heading")
			nameLabel.SetWrap(true)
			nameLabel.SetMaxWidthChars(30)
			iconBox.Append(nameLabel)
			previewBox.Append(iconBox)
		}
	}

	contentBox.Append(previewBox)

	// Caption entry
	entry := gtk.NewEntry()
	entry.SetPlaceholderText("Adicione uma legenda...")
	entry.SetHExpand(true)
	contentBox.Append(entry)
	dialog.Entry = entry

	confirmSend := func() {
		caption := strings.TrimSpace(entry.Text())
		win.Destroy()
		if dialog.OnConfirm != nil {
			dialog.OnConfirm(caption)
		}
	}

	sendBtn.ConnectClicked(confirmSend)
	entry.ConnectActivate(confirmSend)

	mainBox.Append(contentBox)
	win.SetContent(mainBox)
	win.Present()

	entry.GrabFocus()

	return dialog
}
