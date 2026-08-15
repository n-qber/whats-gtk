package chat

import (
	"fmt"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

type ForwardContactItem struct {
	JID     string
	Name    string
	IsGroup bool
	Avatar  *gdk.Texture
}

type ForwardDialog struct {
	Window         *adw.Window
	HeaderBar      *adw.HeaderBar
	SearchEntry    *gtk.SearchEntry
	ListBox        *gtk.ListBox
	ScrolledWindow *gtk.ScrolledWindow
	SendButton     *gtk.Button
	CancelButton   *gtk.Button

	SelectedJIDs map[string]bool
	AllContacts  []ForwardContactItem
	RowWidgets   map[string]*gtk.CheckButton

	OnForward func(targetJIDs []string)
}

func ShowForwardDialog(parent *gtk.Window, contacts []ForwardContactItem, onForward func(targetJIDs []string)) *ForwardDialog {
	win := adw.NewWindow()
	win.SetTitle("Encaminhar mensagem")
	if parent != nil {
		win.SetTransientFor(parent)
	}
	win.SetModal(true)
	win.SetDefaultSize(440, 580)

	fd := &ForwardDialog{
		Window:       win,
		SelectedJIDs: make(map[string]bool),
		AllContacts:  contacts,
		RowWidgets:   make(map[string]*gtk.CheckButton),
		OnForward:    onForward,
	}

	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)

	headerBar := adw.NewHeaderBar()

	cancelBtn := gtk.NewButtonWithLabel("Cancelar")
	cancelBtn.ConnectClicked(func() {
		win.Destroy()
	})
	headerBar.PackStart(cancelBtn)

	titleLabel := gtk.NewLabel("Encaminhar para...")
	titleLabel.AddCSSClass("heading")
	headerBar.SetTitleWidget(titleLabel)

	sendBtn := gtk.NewButtonWithLabel("Encaminhar")
	sendBtn.AddCSSClass("suggested-action")
	sendBtn.SetSensitive(false)
	sendBtn.ConnectClicked(func() {
		var selected []string
		for jid, isSelected := range fd.SelectedJIDs {
			if isSelected {
				selected = append(selected, jid)
			}
		}
		if len(selected) > 0 && fd.OnForward != nil {
			fd.OnForward(selected)
		}
		win.Destroy()
	})
	headerBar.PackEnd(sendBtn)

	fd.HeaderBar = headerBar
	fd.SendButton = sendBtn
	fd.CancelButton = cancelBtn

	mainBox.Append(headerBar)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 8)
	contentBox.SetMarginStart(12)
	contentBox.SetMarginEnd(12)
	contentBox.SetMarginTop(8)
	contentBox.SetMarginBottom(12)

	searchEntry := gtk.NewSearchEntry()
	searchEntry.SetPlaceholderText("Pesquisar contatos ou grupos...")
	searchEntry.ConnectChanged(func() {
		fd.filterContacts(searchEntry.Text())
	})
	contentBox.Append(searchEntry)
	fd.SearchEntry = searchEntry

	listBox := gtk.NewListBox()
	listBox.AddCSSClass("boxed-list")
	listBox.SetSelectionMode(gtk.SelectionNone)
	fd.ListBox = listBox

	scrolledWindow := gtk.NewScrolledWindow()
	scrolledWindow.SetVExpand(true)
	scrolledWindow.SetChild(listBox)
	contentBox.Append(scrolledWindow)
	fd.ScrolledWindow = scrolledWindow

	mainBox.Append(contentBox)
	win.SetContent(mainBox)

	fd.populateContacts(contacts)
	win.Present()

	return fd
}

func (fd *ForwardDialog) populateContacts(contacts []ForwardContactItem) {
	for fd.ListBox.FirstChild() != nil {
		fd.ListBox.Remove(fd.ListBox.FirstChild())
	}
	fd.RowWidgets = make(map[string]*gtk.CheckButton)

	for _, c := range contacts {
		row := gtk.NewListBoxRow()
		rowBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
		rowBox.SetMarginStart(10)
		rowBox.SetMarginEnd(10)
		rowBox.SetMarginTop(8)
		rowBox.SetMarginBottom(8)

		avatar := adw.NewAvatar(36, c.Name, true)
		if c.Avatar != nil {
			avatar.SetCustomImage(c.Avatar)
		}
		rowBox.Append(avatar)

		infoBox := gtk.NewBox(gtk.OrientationVertical, 2)
		infoBox.SetHExpand(true)
		infoBox.SetVAlign(gtk.AlignCenter)

		nameLabel := gtk.NewLabel(c.Name)
		nameLabel.SetXAlign(0)
		nameLabel.AddCSSClass("body")
		nameLabel.SetEllipsize(pango.EllipsizeEnd)
		infoBox.Append(nameLabel)

		if c.IsGroup {
			subtitle := gtk.NewLabel("Grupo")
			subtitle.SetXAlign(0)
			subtitle.AddCSSClass("dim-label")
			subtitle.AddCSSClass("caption")
			infoBox.Append(subtitle)
		}
		rowBox.Append(infoBox)

		chk := gtk.NewCheckButton()
		chk.SetVAlign(gtk.AlignCenter)
		jStr := c.JID
		chk.SetActive(fd.SelectedJIDs[jStr])
		chk.ConnectToggled(func() {
			fd.SelectedJIDs[jStr] = chk.Active()
			fd.updateSendButton()
		})
		rowBox.Append(chk)
		fd.RowWidgets[jStr] = chk

		row.SetChild(rowBox)

		click := gtk.NewGestureClick()
		click.ConnectPressed(func(n int, x, y float64) {
			chk.SetActive(!chk.Active())
		})
		row.AddController(click)

		fd.ListBox.Append(row)
	}
}

func (fd *ForwardDialog) filterContacts(query string) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		fd.populateContacts(fd.AllContacts)
		return
	}

	var filtered []ForwardContactItem
	for _, c := range fd.AllContacts {
		if strings.Contains(strings.ToLower(c.Name), q) || strings.Contains(strings.ToLower(c.JID), q) {
			filtered = append(filtered, c)
		}
	}
	fd.populateContacts(filtered)
}

func (fd *ForwardDialog) updateSendButton() {
	count := 0
	for _, isSel := range fd.SelectedJIDs {
		if isSel {
			count++
		}
	}
	if count == 0 {
		fd.SendButton.SetSensitive(false)
		fd.SendButton.SetLabel("Encaminhar")
	} else {
		fd.SendButton.SetSensitive(true)
		fd.SendButton.SetLabel(fmt.Sprintf("Encaminhar (%d)", count))
	}
}
