package sidebar

import (
	"fmt"
	"strings"
	"whats-gtk/internal/database"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
)

type ProfileItem struct {
	ID   int64
	Name string
}

type Sidebar struct {
	Box               *gtk.Box
	ListBox           *gtk.ListBox
	ScrolledWindow    *gtk.ScrolledWindow
	SearchEntry       *gtk.SearchEntry
	ProgressBar       *gtk.ProgressBar
	ProfileCombo      *gtk.ComboBoxText
	NewChatBtn        *gtk.Button
	ManageProfilesBtn *gtk.Button
	OnChatSelected    func(jid string)
	OnSearch          func(text string)
	OnProfileSelected func(profileID int64)
	OnManageProfiles  func()
	OnNewChat         func()

	profileItems []ProfileItem
	chatRows     map[string]*adw.ActionRow
	chatAvatars  map[string]*adw.Avatar
	chatIndices  map[string]*gtk.Label
	isRefreshing bool
	isCycling    bool
	originalJID  string
	pendingJID   string
	syncPulseId  glib.SourceHandle
}

func NewSidebar() (*Sidebar, error) {
	box := gtk.NewBox(gtk.OrientationVertical, 0)

	profileBar := gtk.NewBox(gtk.OrientationHorizontal, 6)
	profileBar.SetMarginTop(6)
	profileBar.SetMarginStart(6)
	profileBar.SetMarginEnd(6)

	profileCombo := gtk.NewComboBoxText()
	profileCombo.SetHExpand(true)
	profileCombo.SetFocusable(false)
	profileBar.Append(profileCombo)

	newChatBtn := gtk.NewButtonFromIconName("chat-new-symbolic")
	newChatBtn.SetTooltipText("New Conversation (Ctrl+N)")
	profileBar.Append(newChatBtn)

	manageBtn := gtk.NewButtonFromIconName("preferences-system-symbolic")
	manageBtn.SetTooltipText("Manage Profiles")
	profileBar.Append(manageBtn)

	box.Append(profileBar)

	searchEntry := gtk.NewSearchEntry()
	searchEntry.SetMarginTop(4)
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
		Box:               box,
		ListBox:           listBox,
		ScrolledWindow:    scrolled,
		SearchEntry:       searchEntry,
		ProgressBar:       progressBar,
		ProfileCombo:      profileCombo,
		NewChatBtn:        newChatBtn,
		ManageProfilesBtn: manageBtn,
		chatRows:          make(map[string]*adw.ActionRow),
		chatAvatars:       make(map[string]*adw.Avatar),
		chatIndices:       make(map[string]*gtk.Label),
	}

	profileCombo.ConnectChanged(func() {
		if s.isRefreshing {
			return
		}
		idx := profileCombo.Active()
		if idx >= 0 && int(idx) < len(s.profileItems) {
			if s.OnProfileSelected != nil {
				s.OnProfileSelected(s.profileItems[int(idx)].ID)
			}
		}
		glib.IdleAdd(func() {
			s.ListBox.GrabFocus()
		})
	})

	newChatBtn.ConnectClicked(func() {
		if s.OnNewChat != nil {
			s.OnNewChat()
		}
	})

	manageBtn.ConnectClicked(func() {
		if s.OnManageProfiles != nil {
			s.OnManageProfiles()
		}
	})

	searchEntry.ConnectSearchChanged(func() {
		if s.OnSearch != nil {
			s.OnSearch(searchEntry.Text())
		}
	})

	searchEntry.ConnectActivate(func() {
		row := s.ListBox.RowAtIndex(0)
		if row != nil {
			s.ListBox.SelectRow(row)
			searchEntry.SetText("")
		}
	})

	listBox.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		if row == nil || s.isRefreshing { return }
		s.isCycling = false
		s.originalJID = ""
		s.pendingJID = ""
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
		// Default to indeterminate pulsing
		s.SetSyncProgress(-1.0)
	} else {
		if s.syncPulseId != 0 {
			glib.SourceRemove(s.syncPulseId)
			s.syncPulseId = 0
		}
	}
}

func (s *Sidebar) SetSyncProgress(fraction float64) {
	if fraction < 0 {
		if s.syncPulseId == 0 {
			s.syncPulseId = glib.TimeoutAdd(100, func() bool {
				s.ProgressBar.Pulse()
				return true
			})
		}
		s.ProgressBar.SetText("Syncing messages...")
	} else {
		if s.syncPulseId != 0 {
			glib.SourceRemove(s.syncPulseId)
			s.syncPulseId = 0
		}
		s.ProgressBar.SetFraction(fraction)
		s.ProgressBar.SetText(fmt.Sprintf("Syncing... %d%%", int(fraction*100)))
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
	s.isCycling = false
	s.originalJID = ""
	s.pendingJID = ""
	row := s.ListBox.RowAtIndex(index)
	if row != nil {
		s.ListBox.SelectRow(row)
		if s.OnChatSelected != nil && row.Name() != "" {
			s.OnChatSelected(row.Name())
		}
		if s.ScrolledWindow != nil {
			glib.IdleAdd(func() {
				_, destY, success := row.TranslateCoordinates(s.ListBox, 0, 0)
				if success {
					adj := s.ScrolledWindow.VAdjustment()
					val := adj.Value()
					pageSize := adj.PageSize()
					h := float64(row.Height())
					if h <= 0 {
						h = 60
					}
					targetY := destY - 10
					if targetY < 0 {
						targetY = 0
					}
					if destY < val {
						adj.SetValue(targetY)
					} else if destY+h > val+pageSize {
						adj.SetValue(destY + h - pageSize + 10)
					}
				}
			})
		}
	}
}

func (s *Sidebar) SelectOffset(offset int) {
	count := 0
	for {
		if s.ListBox.RowAtIndex(count) == nil {
			break
		}
		count++
	}
	if count == 0 {
		return
	}

	selected := s.ListBox.SelectedRow()
	var newIdx int
	if selected == nil {
		if offset > 0 {
			newIdx = 0
		} else {
			newIdx = count - 1
		}
	} else {
		currentIdx := selected.Index()
		if currentIdx == -1 {
			if offset > 0 {
				newIdx = 0
			} else {
				newIdx = count - 1
			}
		} else {
			newIdx = (currentIdx + offset) % count
			if newIdx < 0 {
				newIdx = count + newIdx
			}
		}
	}
	s.SelectIndex(newIdx)
}

func (s *Sidebar) IsCycling() bool {
	return s.isCycling
}

func (s *Sidebar) CycleOffset(offset int) {
	count := 0
	for {
		if s.ListBox.RowAtIndex(count) == nil {
			break
		}
		count++
	}
	if count == 0 {
		return
	}

	selected := s.ListBox.SelectedRow()
	var currentIdx int = -1

	if !s.isCycling {
		s.isCycling = true
		if selected != nil {
			s.originalJID = selected.Name()
			currentIdx = selected.Index()
		} else {
			s.originalJID = ""
		}
	} else if selected != nil {
		currentIdx = selected.Index()
	}

	var newIdx int
	if currentIdx == -1 {
		if offset > 0 {
			newIdx = 0
		} else {
			newIdx = count - 1
		}
	} else {
		newIdx = (currentIdx + offset) % count
		if newIdx < 0 {
			newIdx = count + newIdx
		}
	}

	row := s.ListBox.RowAtIndex(newIdx)
	if row != nil {
		s.pendingJID = row.Name()
		s.isRefreshing = true
		s.ListBox.SelectRow(row)
		s.isRefreshing = false

		if s.ScrolledWindow != nil {
			glib.IdleAdd(func() {
				_, destY, success := row.TranslateCoordinates(s.ListBox, 0, 0)
				if success {
					adj := s.ScrolledWindow.VAdjustment()
					val := adj.Value()
					pageSize := adj.PageSize()
					h := float64(row.Height())
					if h <= 0 {
						h = 60
					}
					targetY := destY - 10
					if targetY < 0 {
						targetY = 0
					}
					if destY < val {
						adj.SetValue(targetY)
					} else if destY+h > val+pageSize {
						adj.SetValue(destY + h - pageSize + 10)
					}
				}
			})
		}
	}
}

func (s *Sidebar) CommitCycle() {
	if !s.isCycling {
		return
	}
	targetJID := s.pendingJID
	s.isCycling = false
	s.originalJID = ""
	s.pendingJID = ""

	if targetJID != "" && s.OnChatSelected != nil {
		s.OnChatSelected(targetJID)
	}
}

func (s *Sidebar) CancelCycle() bool {
	if !s.isCycling {
		return false
	}
	orig := s.originalJID
	s.isCycling = false
	s.originalJID = ""
	s.pendingJID = ""

	if orig != "" {
		s.SelectChat(orig)
	} else {
		s.ClearSelection()
	}
	return true
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

func formatChatTitle(name string, isGroup bool, unreadCount int, isPinned bool) string {
	title := name
	if isGroup {
		title = "[G] " + title
	}
	if unreadCount > 0 {
		title = fmt.Sprintf("(%d) %s", unreadCount, title)
	}
	if isPinned {
		title = "📌 " + title
	}
	return title
}

func (s *Sidebar) UpdateChatRow(jid, name string, isGroup bool, unreadCount int, isPinned bool) {
	if row, exists := s.chatRows[jid]; exists {
		title := formatChatTitle(name, isGroup, unreadCount, isPinned)
		row.SetTitle(glib.MarkupEscapeText(title))
		return
	}
	s.AddChat(jid, name, isGroup, unreadCount, isPinned)
}

func (s *Sidebar) AddChat(jid, name string, isGroup bool, unreadCount int, isPinned bool) {
	if _, exists := s.chatRows[jid]; exists {
		s.UpdateChatRow(jid, name, isGroup, unreadCount, isPinned)
		return
	}

	row := adw.NewActionRow()
	title := formatChatTitle(name, isGroup, unreadCount, isPinned)
	row.SetTitle(glib.MarkupEscapeText(title))
	row.SetName(jid)
	
	avatar := adw.NewAvatar(32, name, true)
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
	if avatar, exists := s.chatAvatars[jid]; exists {
		if tex != nil {
			avatar.SetCustomImage(tex)
		} else {
			avatar.SetCustomImage(nil)
		}
		return
	}

	cleanJID := jid
	if atIdx := strings.Index(jid, "@"); atIdx != -1 {
		user := jid[:atIdx]
		server := jid[atIdx:]
		if dotIdx := strings.Index(user, "."); dotIdx != -1 {
			cleanJID = user[:dotIdx] + server
		}
		if colonIdx := strings.Index(user, ":"); colonIdx != -1 {
			cleanJID = user[:colonIdx] + server
		}
	}

	if avatar, exists := s.chatAvatars[cleanJID]; exists {
		if tex != nil {
			avatar.SetCustomImage(tex)
		} else {
			avatar.SetCustomImage(nil)
		}
		return
	}

	for j, av := range s.chatAvatars {
		if strings.HasPrefix(j, cleanJID) || strings.HasPrefix(cleanJID, j) {
			if tex != nil {
				av.SetCustomImage(tex)
			} else {
				av.SetCustomImage(nil)
			}
			return
		}
	}
}

func (s *Sidebar) SetProfiles(profiles []database.Profile, activeID int64) {
	s.isRefreshing = true
	defer func() { s.isRefreshing = false }()

	s.ProfileCombo.RemoveAll()
	s.profileItems = nil

	// Add Default "All Chats"
	s.profileItems = append(s.profileItems, ProfileItem{ID: database.ProfileAllChatsID, Name: "All Chats"})
	s.ProfileCombo.AppendText("All Chats")

	for _, p := range profiles {
		s.profileItems = append(s.profileItems, ProfileItem{ID: p.ID, Name: p.Name})
		s.ProfileCombo.AppendText(p.Name)
	}

	// Add System "Archived" (always the last profile)
	s.profileItems = append(s.profileItems, ProfileItem{ID: database.ProfileArchivedID, Name: "Archived"})
	s.ProfileCombo.AppendText("Archived")

	activeIdx := 0
	for i, item := range s.profileItems {
		if item.ID == activeID {
			activeIdx = i
			break
		}
	}

	s.ProfileCombo.SetActive(activeIdx)
}

func (s *Sidebar) SelectProfileIndex(index int) {
	if index >= 0 && index < len(s.profileItems) {
		if s.ProfileCombo.Active() == index {
			s.ListBox.GrabFocus()
			return
		}
		s.ProfileCombo.SetActive(index)
	}
}

