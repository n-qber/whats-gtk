package sidebar

import (
	"fmt"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
)

type Sidebar struct {
	Box            *gtk.Box
	ListBox        *gtk.ListBox
	SearchEntry    *gtk.SearchEntry
	ProgressBar    *gtk.ProgressBar
	OnChatSelected func(jid string)
	OnSearch       func(text string)
	
	chatRows       map[string]*adw.ActionRow
	chatAvatars    map[string]*adw.Avatar
	chatIndices    map[string]*gtk.Label
	isRefreshing   bool
	syncPulseId    glib.SourceHandle
}

func NewSidebar() (*Sidebar, error) {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	
	searchEntry := gtk.NewSearchEntry()
	searchEntry.SetMarginTop(6)
	searchEntry.SetMarginBottom(6)
	searchEntry.SetMarginStart(6)
	searchEntry.SetMarginEnd(6)
	
	box.Append(searchEntry)

	progressBar := gtk.NewProgressBar()
	progressBar.SetVisible(false)
	progressBar.SetShowText(true)
	progressBar.SetText("Syncing messages...")
	box.Append(progressBar)

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
		ProgressBar: progressBar,
		chatRows:    make(map[string]*adw.ActionRow),
		chatAvatars: make(map[string]*adw.Avatar),
		chatIndices: make(map[string]*gtk.Label),
	}

	searchEntry.ConnectSearchChanged(func() {
		if s.OnSearch != nil {
			s.OnSearch(searchEntry.Text())
		}
	})

	listBox.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		if row == nil || s.isRefreshing { return }
		if s.OnChatSelected != nil {
			s.OnChatSelected(row.Name())
		}
	})

	return s, nil
}

func (s *Sidebar) SetRefreshing(refreshing bool) {
	s.isRefreshing = refreshing
}

func (s *Sidebar) ShowSyncing(syncing bool) {
	s.ProgressBar.SetVisible(syncing)
	if syncing {
		if s.syncPulseId == 0 {
			s.syncPulseId = glib.TimeoutAdd(100, func() bool {
				s.ProgressBar.Pulse()
				return true
			})
		}
	} else {
		if s.syncPulseId != 0 {
			glib.SourceRemove(s.syncPulseId)
			s.syncPulseId = 0
		}
	}
}

func (s *Sidebar) SelectChat(jid string) {
	if row, ok := s.chatRows[jid]; ok {
		if lbRow, ok := row.Parent().(*gtk.ListBoxRow); ok {
			s.isRefreshing = true
			s.ListBox.SelectRow(lbRow)
			s.isRefreshing = false
		}
	}
}

func (s *Sidebar) SelectIndex(index int) {
	row := s.ListBox.RowAtIndex(index)
	if row != nil {
		s.ListBox.SelectRow(row)
	}
}

func (s *Sidebar) SelectOffset(offset int) {
	selected := s.ListBox.SelectedRow()
	var newIdx int
	if selected == nil {
		if offset > 0 {
			newIdx = 0
		} else {
			return
		}
	} else {
		count := 0
		currentIdx := -1
		
		// In GTK4, we can iterate rows more easily or just use RowAtIndex in a loop
		for {
			row := s.ListBox.RowAtIndex(count)
			if row == nil {
				break
			}
			if row == selected {
				currentIdx = count
			}
			count++
		}

		if currentIdx == -1 {
			return
		}
		newIdx = (currentIdx + offset) % count
		if newIdx < 0 {
			newIdx = count + newIdx
		}
	}
	s.SelectIndex(newIdx)
}

func (s *Sidebar) ClearSelection() {
	s.isRefreshing = true
	s.ListBox.SelectRow(nil)
	s.isRefreshing = false
}

func (s *Sidebar) ShowIndices(show bool) {
	for _, label := range s.chatIndices {
		if show {
			label.Show()
		} else {
			label.Hide()
		}
	}
}

func (s *Sidebar) AddChat(jid, name string) {
	if _, exists := s.chatRows[jid]; exists {
		return
	}

	row := adw.NewActionRow()
	row.SetTitle(glib.MarkupEscapeText(name))
	row.SetName(jid)
	
	// Clean name for initials (remove [G] and unread count)
	cleanName := name
	if idx := strings.Index(name, "] "); idx != -1 { cleanName = name[idx+2:] }
	if idx := strings.Index(cleanName, ") "); idx != -1 { cleanName = cleanName[idx+2:] }

	avatar := adw.NewAvatar(32, cleanName, true)
	row.AddPrefix(avatar)
	
	// Add index label
	idx := len(s.chatRows) + 1
	idxLabel := gtk.NewLabel(fmt.Sprintf("%d", idx))
	if idx > 9 {
		idxLabel.SetText("") // Only 1-9 are supported for now
	}
	idxLabel.AddCSSClass("chat-index-label")
	idxLabel.SetOpacity(0.5)
	idxLabel.Hide()
	row.AddSuffix(idxLabel)
	
	s.chatRows[jid] = row
	s.chatAvatars[jid] = avatar
	s.chatIndices[jid] = idxLabel
	
	// We need to wrap it in a ListBoxRow to set the name for lookup
	lbRow := gtk.NewListBoxRow()
	lbRow.SetChild(row)
	lbRow.SetName(jid)
	
	s.ListBox.Append(lbRow)
}

func (s *Sidebar) ClearChats() {
	s.chatRows = make(map[string]*adw.ActionRow)
	s.chatAvatars = make(map[string]*adw.Avatar)
	s.chatIndices = make(map[string]*gtk.Label)
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
		if tex != nil {
			avatar.SetCustomImage(tex)
		} else {
			avatar.SetCustomImage(nil)
		}
	} else {
		// Try search by Name if JID didn't match exactly
		for j, av := range s.chatAvatars {
			if strings.HasPrefix(j, jid) || strings.HasPrefix(jid, j) {
				if tex != nil {
					av.SetCustomImage(tex)
				} else {
					av.SetCustomImage(nil)
				}
			}
		}
	}
}
