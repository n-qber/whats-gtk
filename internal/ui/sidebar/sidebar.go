package sidebar

import (
	"github.com/gotk3/gotk3/gtk"
)

type Sidebar struct {
	Box            *gtk.Box
	SearchEntry    *gtk.Entry
	ChatList       *gtk.ListBox
	ChatRows       map[string]*gtk.ListBoxRow
	OnChatSelected func(jid string)
	OnSearch       func(text string)
}

func NewSidebar() (*Sidebar, error) {
	box, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	box.SetSizeRequest(300, -1)

	searchEntry, _ := gtk.EntryNew()
	searchEntry.SetPlaceholderText("Search or start new chat")
	box.PackStart(searchEntry, false, false, 5)

	chatList, _ := gtk.ListBoxNew()
	scrolledChat, _ := gtk.ScrolledWindowNew(nil, nil)
	scrolledChat.Add(chatList)
	box.PackStart(scrolledChat, true, true, 0)

	s := &Sidebar{
		Box:         box,
		SearchEntry: searchEntry,
		ChatList:    chatList,
		ChatRows:    make(map[string]*gtk.ListBoxRow),
	}

	chatList.Connect("row-selected", func() {
		row := chatList.GetSelectedRow()
		if row != nil && s.OnChatSelected != nil {
			for jid, r := range s.ChatRows {
				if r.Native() == row.Native() {
					s.OnChatSelected(jid)
					break
				}
			}
		}
	})

	searchEntry.Connect("changed", func() {
		text, _ := searchEntry.GetText()
		if s.OnSearch != nil {
			s.OnSearch(text)
		}
	})

	return s, nil
}

func (s *Sidebar) AddChat(jid, name string) {
	row, _ := gtk.ListBoxRowNew()
	label, _ := gtk.LabelNew(name)
	label.SetXAlign(0)
	label.SetMarginStart(12)
	label.SetMarginEnd(12)
	label.SetMarginTop(6)
	label.SetMarginBottom(6)
	lCtx, _ := label.GetStyleContext()
	lCtx.AddClass("sidebar-name")
	row.Add(label)
	s.ChatList.Add(row)
	s.ChatRows[jid] = row
	row.ShowAll()
}

func (s *Sidebar) MoveChatToTop(jid string) {
	if row, exists := s.ChatRows[jid]; exists {
		s.ChatList.Remove(row)
		s.ChatList.Insert(row, 0)
	}
}

func (s *Sidebar) ClearChats() {
	s.ChatRows = make(map[string]*gtk.ListBoxRow)
	children := s.ChatList.GetChildren()
	children.Foreach(func(item interface{}) {
		s.ChatList.Remove(item.(gtk.IWidget))
	})
	s.ChatList.ShowAll()
}
