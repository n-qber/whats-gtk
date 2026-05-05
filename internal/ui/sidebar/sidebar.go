package sidebar

import (
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
)

type Sidebar struct {
	Box            *gtk.Box
	ListBox        *gtk.ListBox
	SearchEntry    *gtk.SearchEntry
	OnChatSelected func(jid string)
	OnSearch       func(text string)
	
	chatRows       map[string]*adw.ActionRow
	chatAvatars    map[string]*adw.Avatar
}

func NewSidebar() (*Sidebar, error) {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	
	searchEntry := gtk.NewSearchEntry()
	searchEntry.SetMarginTop(6)
	searchEntry.SetMarginBottom(6)
	searchEntry.SetMarginStart(6)
	searchEntry.SetMarginEnd(6)
	
	box.Append(searchEntry)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	
	listBox := gtk.NewListBox()
	listBox.AddCSSClass("navigation-sidebar")
	scrolled.SetChild(listBox)
	
	box.Append(scrolled)

	s := &Sidebar{
		Box:         box,
		ListBox:     listBox,
		SearchEntry: searchEntry,
		chatRows:    make(map[string]*adw.ActionRow),
		chatAvatars: make(map[string]*adw.Avatar),
	}

	searchEntry.ConnectSearchChanged(func() {
		if s.OnSearch != nil {
			s.OnSearch(searchEntry.Text())
		}
	})

	listBox.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		if row == nil { return }
		// Extract JID from row name or other mapping
		// For now we use row.Name() as JID
		if s.OnChatSelected != nil {
			s.OnChatSelected(row.Name())
		}
	})

	return s, nil
}

func (s *Sidebar) AddChat(jid, name string) {
	if _, exists := s.chatRows[jid]; exists {
		return
	}

	row := adw.NewActionRow()
	row.SetTitle(name)
	row.SetName(jid)
	
	// Clean name for initials (remove [G] and unread count)
	cleanName := name
	if idx := strings.Index(name, "] "); idx != -1 { cleanName = name[idx+2:] }
	if idx := strings.Index(cleanName, ") "); idx != -1 { cleanName = cleanName[idx+2:] }

	avatar := adw.NewAvatar(32, cleanName, true)
	row.AddPrefix(avatar)
	
	s.chatRows[jid] = row
	s.chatAvatars[jid] = avatar
	
	// We need to wrap it in a ListBoxRow to set the name for lookup
	lbRow := gtk.NewListBoxRow()
	lbRow.SetChild(row)
	lbRow.SetName(jid)
	
	s.ListBox.Append(lbRow)
}

func (s *Sidebar) ClearChats() {
	s.chatRows = make(map[string]*adw.ActionRow)
	s.chatAvatars = make(map[string]*adw.Avatar)
	for {
		child := s.ListBox.FirstChild()
		if child == nil {
			break
		}
		s.ListBox.Remove(child)
	}
}

func (s *Sidebar) MoveChatToTop(jid string) {
	// [TODO: Implement Move to top logic]
}

func (s *Sidebar) SetAvatar(jid string, tex *gdk.Texture) {
	// Normalize JID for lookup
	jid = strings.Split(jid, ".")[0] // Handle potential .AD suffixes
	if avatar, exists := s.chatAvatars[jid]; exists {
		avatar.SetCustomImage(tex)
	} else {
		// Try search by Name if JID didn't match exactly
		for j, av := range s.chatAvatars {
			if strings.HasPrefix(j, jid) || strings.HasPrefix(jid, j) {
				av.SetCustomImage(tex)
			}
		}
	}
}
