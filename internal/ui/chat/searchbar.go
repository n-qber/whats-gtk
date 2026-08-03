package chat

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type SearchBar struct {
	Widget *gtk.SearchBar
	Entry  *gtk.SearchEntry

	OnSearchMessages func(query string)
	OnCancelSearch   func()
}

func NewSearchBar(keyCaptureWidget gtk.Widgetter) *SearchBar {
	searchBar := gtk.NewSearchBar()
	searchEntry := gtk.NewSearchEntry()
	searchBar.ConnectEntry(searchEntry)
	searchBar.SetChild(searchEntry)
	searchBar.SetKeyCaptureWidget(keyCaptureWidget)
	searchBar.SetShowCloseButton(true)

	sb := &SearchBar{
		Widget: searchBar,
		Entry:  searchEntry,
	}

	searchBar.Connect("notify::search-mode-enabled", func() {
		if !searchBar.SearchMode() {
			if sb.OnCancelSearch != nil {
				sb.OnCancelSearch()
			}
		}
	})

	searchEntry.ConnectSearchChanged(func() {
		text := searchEntry.Text()
		if text == "" {
			if sb.OnCancelSearch != nil {
				sb.OnCancelSearch()
			}
		} else {
			if sb.OnSearchMessages != nil {
				sb.OnSearchMessages(text)
			}
		}
	})

	return sb
}

func (sb *SearchBar) Toggle() {
	sb.Widget.SetSearchMode(!sb.Widget.SearchMode())
	if sb.Widget.SearchMode() {
		sb.Entry.GrabFocus()
	}
}

func (sb *SearchBar) Close() {
	sb.Widget.SetSearchMode(false)
}

func (sb *SearchBar) IsActive() bool {
	return sb.Widget.SearchMode()
}
