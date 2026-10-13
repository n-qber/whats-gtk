package sidebar

import (
	"fmt"
	"strings"
	"time"
	"whats-gtk/internal/database"
	"whats-gtk/internal/events"

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
	PreferencesBtn    *gtk.Button
	OnChatSelected    func(jid string)
	OnSearch          func(text string)
	OnProfileSelected func(profileID int64)
	OnManageProfiles  func()
	OnOpenPreferences func()
	OnNewChat         func()
	OnNewGroup        func()
	OnNewCommunity    func()
	OnPinChat         func(jid string, pin bool)
	OnArchiveChat     func(jid string, archive bool)
	OnMuteChat        func(jid string, duration time.Duration)
	OnMarkUnread      func(jid string, unread bool)
	OnDetachChat      func(jid string)
	OnClearChat       func(jid string)

	profileItems []ProfileItem
	chatRows        map[string]*adw.ActionRow
	chatListBoxRows map[string]*gtk.ListBoxRow
	chatAvatars     map[string]*adw.Avatar
	chatIndices  map[string]*gtk.Label
	chatPinned   map[string]bool
	chatUnread   map[string]bool
	chatMuted    map[string]bool

	showGroupTag           bool
	showCommunityTag       bool
	showGroupUnread        bool
	showLastMessagePreview bool

	chatNames          map[string]string
	chatIsGroup        map[string]bool
	chatIsCommunity    map[string]bool
	chatUnreadCounts   map[string]int
	chatLastMessages   map[string]string
	chatTypingPresence map[string]string

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
	newChatBtn.SetTooltipText("Nova Conversa (Ctrl+N)")
	profileBar.Append(newChatBtn)

	manageBtn := gtk.NewButtonFromIconName("preferences-system-symbolic")
	manageBtn.SetTooltipText("Manage Profiles")
	profileBar.Append(manageBtn)

	prefBtn := gtk.NewButtonFromIconName("emblem-system-symbolic")
	prefBtn.SetTooltipText("Preferências (Ctrl+,)")
	profileBar.Append(prefBtn)

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
		PreferencesBtn:    prefBtn,
		showGroupTag:           true,
		showCommunityTag:       true,
		showGroupUnread:        true,
		showLastMessagePreview: true,
		chatRows:          make(map[string]*adw.ActionRow),
		chatListBoxRows:   make(map[string]*gtk.ListBoxRow),
		chatAvatars:       make(map[string]*adw.Avatar),
		chatIndices:       make(map[string]*gtk.Label),
		chatPinned:        make(map[string]bool),
		chatUnread:        make(map[string]bool),
		chatMuted:         make(map[string]bool),
		chatNames:          make(map[string]string),
		chatIsGroup:        make(map[string]bool),
		chatIsCommunity:    make(map[string]bool),
		chatUnreadCounts:   make(map[string]int),
		chatLastMessages:   make(map[string]string),
		chatTypingPresence: make(map[string]string),
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
		popover := gtk.NewPopover()
		popover.ConnectClosed(func() {
			glib.IdleAdd(func() {
				popover.Unparent()
			})
		})

		menuBox := gtk.NewBox(gtk.OrientationVertical, 2)
		menuBox.SetMarginTop(4)
		menuBox.SetMarginBottom(4)
		menuBox.SetMarginStart(4)
		menuBox.SetMarginEnd(4)

		makeMenuItem := func(iconName, text string, onClick func()) *gtk.Button {
			btn := gtk.NewButton()
			btn.SetHasFrame(false)
			btn.AddCSSClass("flat")

			itemBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
			itemBox.SetMarginStart(8)
			itemBox.SetMarginEnd(12)
			itemBox.SetMarginTop(6)
			itemBox.SetMarginBottom(6)

			icon := gtk.NewImageFromIconName(iconName)
			label := gtk.NewLabel(text)
			label.SetXAlign(0)
			label.SetHExpand(true)

			itemBox.Append(icon)
			itemBox.Append(label)
			btn.SetChild(itemBox)

			btn.ConnectClicked(func() {
				popover.Popdown()
				if onClick != nil {
					onClick()
				}
			})
			return btn
		}

		menuBox.Append(makeMenuItem("chat-symbolic", "Nova Conversa", func() {
			if s.OnNewChat != nil {
				s.OnNewChat()
			}
		}))

		menuBox.Append(makeMenuItem("system-users-symbolic", "Novo Grupo", func() {
			if s.OnNewGroup != nil {
				s.OnNewGroup()
			}
		}))

		menuBox.Append(makeMenuItem("folder-publicshare-symbolic", "Nova Comunidade", func() {
			if s.OnNewCommunity != nil {
				s.OnNewCommunity()
			}
		}))

		popover.SetChild(menuBox)
		popover.SetParent(newChatBtn)
		popover.Popup()
	})

	manageBtn.ConnectClicked(func() {
		if s.OnManageProfiles != nil {
			s.OnManageProfiles()
		}
	})

	prefBtn.ConnectClicked(func() {
		if s.OnOpenPreferences != nil {
			s.OnOpenPreferences()
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

func (s *Sidebar) formatChatTitle(name string, isGroup bool, isCommunity bool, unreadCount int, isPinned bool, isMuted ...bool) string {
	title := name
	if isCommunity {
		if s.showCommunityTag {
			title = "[C] " + title
		}
	} else if isGroup {
		if s.showGroupTag {
			title = "[G] " + title
		}
	}
	if unreadCount > 0 {
		if !isGroup || s.showGroupUnread {
			title = fmt.Sprintf("(%d) %s", unreadCount, title)
		}
	}
	if len(isMuted) > 0 && isMuted[0] {
		title = "🔕 " + title
	}
	if isPinned {
		title = "📌 " + title
	}
	return title
}

func formatChatTitle(name string, isGroup bool, isCommunity bool, unreadCount int, isPinned bool, isMuted ...bool) string {
	title := name
	if isCommunity {
		title = "[C] " + title
	} else if isGroup {
		title = "[G] " + title
	}
	if unreadCount > 0 {
		title = fmt.Sprintf("(%d) %s", unreadCount, title)
	}
	if len(isMuted) > 0 && isMuted[0] {
		title = "🔕 " + title
	}
	if isPinned {
		title = "📌 " + title
	}
	return title
}

func (s *Sidebar) SetShowGroupTag(show bool) {
	s.showGroupTag = show
	s.RefreshAllTitles()
}
func (s *Sidebar) IsShowGroupTag() bool { return s.showGroupTag }

func (s *Sidebar) SetShowCommunityTag(show bool) {
	s.showCommunityTag = show
	s.RefreshAllTitles()
}
func (s *Sidebar) IsShowCommunityTag() bool { return s.showCommunityTag }

func (s *Sidebar) SetShowGroupUnread(show bool) {
	s.showGroupUnread = show
	s.RefreshAllTitles()
}
func (s *Sidebar) IsShowGroupUnread() bool { return s.showGroupUnread }

func (s *Sidebar) SetShowLastMessagePreview(show bool) {
	s.showLastMessagePreview = show
	for jid := range s.chatRows {
		s.updateRowSubtitle(jid)
	}
}
func (s *Sidebar) IsShowLastMessagePreview() bool { return s.showLastMessagePreview }

func (s *Sidebar) RefreshAllTitles() {
	for jid, row := range s.chatRows {
		if row != nil {
			name := s.chatNames[jid]
			isGrp := s.chatIsGroup[jid]
			isComm := s.chatIsCommunity[jid]
			unread := s.chatUnreadCounts[jid]
			isPinned := s.chatPinned[jid]
			isMuted := s.chatMuted[jid]
			title := s.formatChatTitle(name, isGrp, isComm, unread, isPinned, isMuted)
			row.SetTitle(glib.MarkupEscapeText(title))
		}
	}
}

func (s *Sidebar) UpdateChatRow(jid, name string, isGroup bool, unreadCount int, isPinned bool, isMuted ...bool) {
	isCommunity := false
	if s.chatIsCommunity != nil {
		isCommunity = s.chatIsCommunity[jid]
	}
	s.UpdateChatRowFull(jid, name, isGroup, isCommunity, unreadCount, isPinned, isMuted...)
}

func (s *Sidebar) UpdateChatRowFull(jid, name string, isGroup bool, isCommunity bool, unreadCount int, isPinned bool, isMuted ...bool) {
	muted := false
	if len(isMuted) > 0 {
		muted = isMuted[0]
	} else if s.chatMuted != nil {
		muted = s.chatMuted[jid]
	}
	if s.chatPinned != nil {
		s.chatPinned[jid] = isPinned
	}
	if s.chatUnread != nil {
		s.chatUnread[jid] = unreadCount > 0
	}
	if s.chatMuted != nil {
		s.chatMuted[jid] = muted
	}
	if s.chatNames != nil {
		s.chatNames[jid] = name
	}
	if s.chatIsGroup != nil {
		s.chatIsGroup[jid] = isGroup
	}
	if s.chatIsCommunity != nil {
		s.chatIsCommunity[jid] = isCommunity
	}
	if s.chatUnreadCounts != nil {
		s.chatUnreadCounts[jid] = unreadCount
	}
	if row, exists := s.chatRows[jid]; exists {
		title := s.formatChatTitle(name, isGroup, isCommunity, unreadCount, isPinned, muted)
		row.SetTitle(glib.MarkupEscapeText(title))
		s.updateRowSubtitle(jid)
		return
	}
	s.AddChatFull(jid, name, isGroup, isCommunity, unreadCount, isPinned, muted)
}

func (s *Sidebar) SetChatSubtitle(jid, subtitle string) {
	if s.chatTypingPresence != nil {
		s.chatTypingPresence[jid] = subtitle
	}
	s.updateRowSubtitle(jid)
}

func (s *Sidebar) SetChatLastMessage(jid, lastMessage string) {
	if s.chatLastMessages != nil {
		s.chatLastMessages[jid] = lastMessage
	}
	s.updateRowSubtitle(jid)
}

func (s *Sidebar) updateRowSubtitle(jid string) {
	row, exists := s.chatRows[jid]
	if !exists || row == nil {
		return
	}
	if s.chatTypingPresence != nil && s.chatTypingPresence[jid] != "" {
		row.SetSubtitle(s.chatTypingPresence[jid])
		return
	}
	if s.showLastMessagePreview && s.chatLastMessages != nil && s.chatLastMessages[jid] != "" {
		row.SetSubtitle(glib.MarkupEscapeText(s.chatLastMessages[jid]))
		return
	}
	row.SetSubtitle("")
}

func (s *Sidebar) AddChat(jid, name string, isGroup bool, unreadCount int, isPinned bool, isMuted ...bool) {
	s.AddChatFull(jid, name, isGroup, false, unreadCount, isPinned, isMuted...)
}

func (s *Sidebar) AddChatFull(jid, name string, isGroup bool, isCommunity bool, unreadCount int, isPinned bool, isMuted ...bool) {
	muted := false
	if len(isMuted) > 0 {
		muted = isMuted[0]
	} else if s.chatMuted != nil {
		muted = s.chatMuted[jid]
	}
	if _, exists := s.chatRows[jid]; exists {
		s.UpdateChatRowFull(jid, name, isGroup, isCommunity, unreadCount, isPinned, muted)
		return
	}

	if s.chatPinned != nil {
		s.chatPinned[jid] = isPinned
	}
	if s.chatUnread != nil {
		s.chatUnread[jid] = unreadCount > 0
	}
	if s.chatMuted != nil {
		s.chatMuted[jid] = muted
	}
	if s.chatNames != nil {
		s.chatNames[jid] = name
	}
	if s.chatIsGroup != nil {
		s.chatIsGroup[jid] = isGroup
	}
	if s.chatIsCommunity != nil {
		s.chatIsCommunity[jid] = isCommunity
	}
	if s.chatUnreadCounts != nil {
		s.chatUnreadCounts[jid] = unreadCount
	}

	row := adw.NewActionRow()
	title := s.formatChatTitle(name, isGroup, isCommunity, unreadCount, isPinned, muted)
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
	
	// Right click context menu
	rightClick := gtk.NewGestureClick()
	rightClick.SetButton(3)
	rightClick.ConnectPressed(func(n int, x, y float64) {
		s.showChatContextMenu(jid, lbRow)
	})
	lbRow.AddController(rightClick)

	s.ListBox.Append(lbRow)
	if s.chatListBoxRows != nil {
		s.chatListBoxRows[jid] = lbRow
	}
	s.updateRowSubtitle(jid)
}

func (s *Sidebar) RemoveChat(jid string) {
	if s.chatListBoxRows != nil {
		if lbRow, ok := s.chatListBoxRows[jid]; ok {
			s.ListBox.Remove(lbRow)
			delete(s.chatListBoxRows, jid)
		}
	}
	delete(s.chatRows, jid)
	delete(s.chatAvatars, jid)
	delete(s.chatIndices, jid)
	delete(s.chatPinned, jid)
	delete(s.chatUnread, jid)
	delete(s.chatMuted, jid)
	delete(s.chatNames, jid)
	delete(s.chatIsGroup, jid)
	delete(s.chatIsCommunity, jid)
	delete(s.chatUnreadCounts, jid)
	delete(s.chatLastMessages, jid)
	delete(s.chatTypingPresence, jid)
}

func (s *Sidebar) SyncChats(items []events.SidebarItem) {
	if len(s.chatRows) == 0 {
		for _, item := range items {
			if item.LastMessage != "" && s.chatLastMessages != nil {
				s.chatLastMessages[item.JID] = item.LastMessage
			}
			s.AddChatFull(item.JID, item.Name, item.IsGroup, item.IsCommunity, item.UnreadCount, item.IsPinned, item.IsMuted)
			if item.Avatar != nil {
				s.SetAvatar(item.JID, item.Avatar)
			}
			s.updateRowSubtitle(item.JID)
		}
		return
	}

	validJIDs := make(map[string]bool, len(items))
	for _, item := range items {
		validJIDs[item.JID] = true
	}

	for jid := range s.chatRows {
		if !validJIDs[jid] {
			s.RemoveChat(jid)
		}
	}

	for _, item := range items {
		if item.LastMessage != "" && s.chatLastMessages != nil {
			s.chatLastMessages[item.JID] = item.LastMessage
		}
		if _, exists := s.chatRows[item.JID]; exists {
			s.UpdateChatRowFull(item.JID, item.Name, item.IsGroup, item.IsCommunity, item.UnreadCount, item.IsPinned, item.IsMuted)
			if item.Avatar != nil {
				s.SetAvatar(item.JID, item.Avatar)
			}
			s.updateRowSubtitle(item.JID)
		} else {
			s.AddChatFull(item.JID, item.Name, item.IsGroup, item.IsCommunity, item.UnreadCount, item.IsPinned, item.IsMuted)
			if item.Avatar != nil {
				s.SetAvatar(item.JID, item.Avatar)
			}
			s.updateRowSubtitle(item.JID)
		}
	}
}

func (s *Sidebar) ClearChats() {
	s.chatRows = make(map[string]*adw.ActionRow)
	s.chatListBoxRows = make(map[string]*gtk.ListBoxRow)
	s.chatAvatars = make(map[string]*adw.Avatar)
	s.chatIndices = make(map[string]*gtk.Label)
	s.chatPinned = make(map[string]bool)
	s.chatUnread = make(map[string]bool)
	s.chatMuted = make(map[string]bool)
	s.chatNames = make(map[string]string)
	s.chatIsGroup = make(map[string]bool)
	s.chatIsCommunity = make(map[string]bool)
	s.chatUnreadCounts = make(map[string]int)
	s.chatLastMessages = make(map[string]string)
	s.chatTypingPresence = make(map[string]string)
	for {
		child := s.ListBox.FirstChild()
		if child == nil {
			break
		}
		s.ListBox.Remove(child)
	}
}

func (s *Sidebar) showChatContextMenu(jid string, targetWidget gtk.Widgetter) {
	popover := gtk.NewPopover()
	box := gtk.NewBox(gtk.OrientationVertical, 0)

	isPinned := false
	if s.chatPinned != nil {
		isPinned = s.chatPinned[jid]
	}
	isUnread := false
	if s.chatUnread != nil {
		isUnread = s.chatUnread[jid]
	}
	isMuted := false
	if s.chatMuted != nil {
		isMuted = s.chatMuted[jid]
	}

	// 1. Pin / Unpin
	pinLabel := "📌 Fixar conversa"
	if isPinned {
		pinLabel = "📌 Desafixar conversa"
	}
	pinBtn := gtk.NewButtonWithLabel(pinLabel)
	pinBtn.SetHasFrame(false)
	pinBtn.ConnectClicked(func() {
		popover.Popdown()
		if s.OnPinChat != nil {
			s.OnPinChat(jid, !isPinned)
		}
	})
	box.Append(pinBtn)

	// 2. Mark as read / unread
	readLabel := "Marcar como não lida"
	if isUnread {
		readLabel = "Marcar como lida"
	}
	readBtn := gtk.NewButtonWithLabel(readLabel)
	readBtn.SetHasFrame(false)
	readBtn.ConnectClicked(func() {
		popover.Popdown()
		if s.OnMarkUnread != nil {
			s.OnMarkUnread(jid, !isUnread)
		}
	})
	box.Append(readBtn)

	// 3. Mute notifications
	muteLabel := "Silenciar..."
	if isMuted {
		muteLabel = "🔕 Silenciado (configurar...)"
	}
	muteBtn := gtk.NewButtonWithLabel(muteLabel)
	muteBtn.SetHasFrame(false)
	muteBtn.ConnectClicked(func() {
		popover.Popdown()
		mutePopover := gtk.NewPopover()
		mBox := gtk.NewBox(gtk.OrientationVertical, 0)

		durations := []struct {
			label string
			d     time.Duration
		}{
			{"8 horas", 8 * time.Hour},
			{"1 semana", 7 * 24 * time.Hour},
			{"Sempre", -1},
			{"Reativar notificações", 0},
		}

		for _, item := range durations {
			d := item.d
			btn := gtk.NewButtonWithLabel(item.label)
			btn.SetHasFrame(false)
			btn.ConnectClicked(func() {
				mutePopover.Popdown()
				if s.OnMuteChat != nil {
					s.OnMuteChat(jid, d)
				}
			})
			mBox.Append(btn)
		}

		mutePopover.SetChild(mBox)
		mutePopover.SetParent(targetWidget)
		mutePopover.Popup()
	})
	box.Append(muteBtn)

	// 4. Archive chat
	archiveBtn := gtk.NewButtonWithLabel("Arquivar conversa")
	archiveBtn.SetHasFrame(false)
	archiveBtn.ConnectClicked(func() {
		popover.Popdown()
		if s.OnArchiveChat != nil {
			s.OnArchiveChat(jid, true)
		}
	})
	box.Append(archiveBtn)

	// 5. Detach into separate window
	detachBtn := gtk.NewButtonWithLabel("Abrir em nova janela")
	detachBtn.SetHasFrame(false)
	detachBtn.ConnectClicked(func() {
		popover.Popdown()
		if s.OnDetachChat != nil {
			s.OnDetachChat(jid)
		}
	})
	box.Append(detachBtn)

	box.Append(gtk.NewSeparator(gtk.OrientationHorizontal))

	// 6. Clear chat messages
	clearBtn := gtk.NewButtonWithLabel("Limpar mensagens")
	clearBtn.SetHasFrame(false)
	clearBtn.AddCSSClass("destructive-action")
	clearBtn.ConnectClicked(func() {
		popover.Popdown()
		if s.OnClearChat != nil {
			s.OnClearChat(jid)
		}
	})
	box.Append(clearBtn)

	popover.SetChild(box)
	popover.SetParent(targetWidget)
	popover.Popup()
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

