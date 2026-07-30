package ui

import (
	"whats-gtk/internal/database"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

type SearchDialog struct {
	Window *gtk.Window
	Input  *gtk.SearchEntry
	List   *gtk.ListBox
}

func NewSearchDialog(parent *gtk.Window, onSearch func(string), onSelect func(database.Message)) *SearchDialog {
	win := gtk.NewWindow()
	win.SetTitle("Search Messages")
	win.SetTransientFor(parent)
	win.SetModal(true)
	win.SetDefaultSize(400, 500)
	win.SetHideOnClose(true)

	box := gtk.NewBox(gtk.OrientationVertical, 0)
	
	header := gtk.NewHeaderBar()
	win.SetTitlebar(header)

	input := gtk.NewSearchEntry()
	input.SetMarginTop(10)
	input.SetMarginBottom(10)
	input.SetMarginStart(10)
	input.SetMarginEnd(10)
	input.SetPlaceholderText("Search...")
	
	box.Append(input)

	list := gtk.NewListBox()
	list.SetSelectionMode(gtk.SelectionSingle)
	list.SetCSSClasses([]string{"navigation-sidebar"})
	
	scroll := gtk.NewScrolledWindow()
	scroll.SetVExpand(true)
	scroll.SetChild(list)
	
	box.Append(scroll)

	win.SetChild(box)

	input.ConnectSearchChanged(func() {
		text := input.Text()
		if len(text) >= 3 {
			onSearch(text)
		} else if len(text) == 0 {
			onSearch("")
		}
	})

	list.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		// We'll attach the message data using object data, or just lookup by index
	})

	return &SearchDialog{
		Window: win,
		Input:  input,
		List:   list,
	}
}

func (s *SearchDialog) SetLoading(loading bool) {
	for s.List.FirstChild() != nil {
		s.List.Remove(s.List.FirstChild())
	}
	if loading {
		spinner := gtk.NewSpinner()
		spinner.Start()
		spinner.SetMarginTop(20)
		spinner.SetMarginBottom(20)
		row := gtk.NewListBoxRow()
		row.SetChild(spinner)
		row.SetSelectable(false)
		row.SetActivatable(false)
		s.List.Append(row)
	}
}

func (s *SearchDialog) Populate(msgs []database.Message, onSelect func(database.Message)) {
	// Clear list
	for s.List.FirstChild() != nil {
		s.List.Remove(s.List.FirstChild())
	}

	if msgs != nil && len(msgs) == 0 {
		lbl := gtk.NewLabel("No results found")
		lbl.SetMarginTop(20)
		lbl.SetMarginBottom(20)
		lbl.SetCSSClasses([]string{"dim-label"})
		row := gtk.NewListBoxRow()
		row.SetChild(lbl)
		row.SetSelectable(false)
		row.SetActivatable(false)
		s.List.Append(row)
		return
	}

	for _, msg := range msgs {
		m := msg // copy for closure
		
		rowBox := gtk.NewBox(gtk.OrientationVertical, 4)
		rowBox.SetMarginTop(8)
		rowBox.SetMarginBottom(8)
		rowBox.SetMarginStart(12)
		rowBox.SetMarginEnd(12)
		
		content := m.Content
		if content == "" {
			content = m.Caption.String
		}
		if content == "" {
			content = "[Media/Other]"
		}
		
		lbl := gtk.NewLabel(content)
		lbl.SetHAlign(gtk.AlignStart)
		lbl.SetWrap(true)
		lbl.SetWrapMode(pango.WrapWordChar)
		lbl.SetLines(2)
		
		timeLbl := gtk.NewLabel(m.Timestamp.Format("02/01/06 15:04"))
		timeLbl.SetHAlign(gtk.AlignStart)
		timeLbl.SetCSSClasses([]string{"dim-label"})
		
		rowBox.Append(lbl)
		rowBox.Append(timeLbl)
		
		row := gtk.NewListBoxRow()
		row.SetChild(rowBox)
		
		// Event controller for click
		ctrl := gtk.NewGestureClick()
		ctrl.ConnectPressed(func(nPress int, x, y float64) {
			onSelect(m)
			s.Window.Close()
		})
		row.AddController(ctrl)
		
		s.List.Append(row)
	}
}
