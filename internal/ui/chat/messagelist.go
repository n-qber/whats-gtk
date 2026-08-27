package chat

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"whats-gtk/internal/ui/chat/bubbles"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

type MessageList struct {
	Widget             *gtk.Box
	ListBox            *gtk.ListBox
	ScrolledWindow     *gtk.ScrolledWindow
	PinnedMessageBar   *gtk.Box
	PinnedMessageLabel *gtk.Label
	LoadingSpinner     *gtk.Spinner

	MessageRows     map[string]bubbles.Bubble
	MessageListRows map[string]*gtk.ListBoxRow
	RowToMessageID  map[*gtk.ListBoxRow]string
	BubblesByJID    map[string][]bubbles.Bubble
	InsertIndex     int

	AudioPlayer *AudioPlayer

	// State
	IsLoadingOlder     bool
	NoMoreOlder        bool
	IsSearching        bool
	KeyboardSelectedID string

	// Selection mode state
	IsSelectionMode     bool
	SelectedIDs         map[string]bool
	SelectionBar        *gtk.Box
	SelectionCountLabel *gtk.Label
	SelectionCancelBtn  *gtk.Button
	SelectionForwardBtn *gtk.Button

	// Callbacks
	OnLoadOlder              func()
	OnLoadMessageRequest     func(id string)
	OnSearchResultClick      func(id string)
	OnDownloadMedia          func(id string)
	OnOpenImage              func(path string)
	OnSendReaction           func(id, emoji string)
	OnPinMessage             func(id string, pin bool, duration uint32)
	OnMentionClick           func(jid string)
	OnSendPollVote           func(msgID string, senderJID string, isFromMe bool, selectedOptions []string)
	OnReplyRequest           func(id, sender, content string)
	OnForwardMessagesRequest func(ids []string)
	OnContextMenuClosed      func()
}

func NewMessageList() *MessageList {
	ml := &MessageList{
		Widget:          gtk.NewBox(gtk.OrientationVertical, 0),
		MessageRows:     make(map[string]bubbles.Bubble),
		MessageListRows: make(map[string]*gtk.ListBoxRow),
		RowToMessageID:  make(map[*gtk.ListBoxRow]string),
		BubblesByJID:    make(map[string][]bubbles.Bubble),
		SelectedIDs:     make(map[string]bool),
		InsertIndex:     -1,
		AudioPlayer:     NewAudioPlayer(),
	}

	// Pinned Message Bar
	ml.PinnedMessageBar = gtk.NewBox(gtk.OrientationHorizontal, 5)
	ml.PinnedMessageBar.AddCSSClass("pinned-message-bar")
	pinnedIcon := gtk.NewImageFromIconName("pin-symbolic")
	ml.PinnedMessageLabel = gtk.NewLabel("Pinned Message")
	ml.PinnedMessageLabel.SetXAlign(0)
	ml.PinnedMessageLabel.SetEllipsize(pango.EllipsizeEnd)
	ml.PinnedMessageBar.Append(pinnedIcon)
	ml.PinnedMessageBar.Append(ml.PinnedMessageLabel)
	ml.PinnedMessageBar.Hide()
	ml.Widget.Append(ml.PinnedMessageBar)

	// Selection Bar
	ml.SelectionBar = gtk.NewBox(gtk.OrientationHorizontal, 10)
	ml.SelectionBar.AddCSSClass("selection-bar")

	cancelBtn := gtk.NewButtonFromIconName("window-close-symbolic")
	cancelBtn.SetTooltipText("Cancelar seleção")
	cancelBtn.ConnectClicked(func() {
		ml.ExitSelectionMode()
	})
	ml.SelectionBar.Append(cancelBtn)
	ml.SelectionCancelBtn = cancelBtn

	countLabel := gtk.NewLabel("1 mensagem selecionada")
	countLabel.AddCSSClass("heading")
	countLabel.SetHExpand(true)
	countLabel.SetXAlign(0)
	ml.SelectionBar.Append(countLabel)
	ml.SelectionCountLabel = countLabel

	forwardBtn := gtk.NewButtonWithLabel("Encaminhar")
	forwardBtn.AddCSSClass("suggested-action")
	forwardBtn.ConnectClicked(func() {
		var selected []string
		for id, sel := range ml.SelectedIDs {
			if sel {
				selected = append(selected, id)
			}
		}
		if len(selected) > 0 && ml.OnForwardMessagesRequest != nil {
			ml.OnForwardMessagesRequest(selected)
		}
		ml.ExitSelectionMode()
	})
	ml.SelectionBar.Append(forwardBtn)
	ml.SelectionForwardBtn = forwardBtn

	ml.SelectionBar.Hide()
	ml.Widget.Append(ml.SelectionBar)

	// Loading Spinner
	ml.LoadingSpinner = gtk.NewSpinner()
	ml.LoadingSpinner.SetMarginTop(10)
	ml.LoadingSpinner.SetMarginBottom(10)
	ml.LoadingSpinner.SetHAlign(gtk.AlignCenter)
	ml.LoadingSpinner.Hide()
	ml.Widget.Append(ml.LoadingSpinner)

	// List Box
	ml.ListBox = gtk.NewListBox()
	ml.ListBox.SetName("message-list")
	ml.ListBox.SetSelectionMode(gtk.SelectionNone)

	// Scrolled Window
	ml.ScrolledWindow = gtk.NewScrolledWindow()
	ml.ScrolledWindow.SetVExpand(true)
	ml.ScrolledWindow.SetChild(ml.ListBox)
	ml.Widget.Append(ml.ScrolledWindow)

	// Events
	ml.ListBox.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		if ml.IsSearching {
			for id, r := range ml.MessageListRows {
				if r == row {
					if ml.OnSearchResultClick != nil {
						ml.OnSearchResultClick(id)
					}
					break
				}
			}
		}
	})

	ml.ScrolledWindow.VAdjustment().ConnectValueChanged(func() {
		adj := ml.ScrolledWindow.VAdjustment()
		if len(ml.MessageRows) > 0 && adj.Value() <= 50.0 && !ml.IsLoadingOlder && !ml.NoMoreOlder && ml.OnLoadOlder != nil {
			ml.SetLoadingOlder(true)
			ml.OnLoadOlder()
		}
	})

	return ml
}

func (ml *MessageList) SetLoadingOlder(loading bool) {
	ml.IsLoadingOlder = loading
	if loading {
		ml.LoadingSpinner.Show()
		ml.LoadingSpinner.Start()
	} else {
		ml.LoadingSpinner.Stop()
		ml.LoadingSpinner.Hide()
	}
}

func (ml *MessageList) SetPinnedMessage(content string) {
	if content == "" {
		ml.PinnedMessageBar.Hide()
	} else {
		ml.PinnedMessageLabel.SetText(content)
		ml.PinnedMessageBar.Show()
	}
}

func (ml *MessageList) Clear() {
	ml.ClearKeyboardSelection()
	ml.ExitSelectionMode()
	ml.SetLoadingOlder(false)
	ml.NoMoreOlder = false
	for ml.ListBox.FirstChild() != nil {
		ml.ListBox.Remove(ml.ListBox.FirstChild())
	}
	ml.MessageRows = make(map[string]bubbles.Bubble)
	ml.MessageListRows = make(map[string]*gtk.ListBoxRow)
	ml.RowToMessageID = make(map[*gtk.ListBoxRow]string)
	ml.BubblesByJID = make(map[string][]bubbles.Bubble)
	ml.InsertIndex = -1
}

func (ml *MessageList) ScrollToBottom() {
	attempts := 0
	var prevUpper float64 = -1
	var tryScroll func() bool
	tryScroll = func() bool {
		attempts++
		adj := ml.ScrolledWindow.VAdjustment()
		upper := adj.Upper()
		pageSize := adj.PageSize()
		target := upper - pageSize
		if target < 0 {
			target = 0
		}
		adj.SetValue(target)

		if upper != prevUpper && attempts < 5 {
			prevUpper = upper
			return true
		}
		return false
	}
	glib.TimeoutAdd(20, tryScroll)
}

func (ml *MessageList) HighlightMessage(id string) {
	if row, ok := ml.MessageListRows[id]; ok {
		glib.IdleAdd(func() {
			row.AddCSSClass("highlighted-row")
			glib.TimeoutAdd(1500, func() bool {
				row.RemoveCSSClass("highlighted-row")
				return false
			})
		})
	}
}

func (ml *MessageList) ScrollToMessage(id string) {
	if row, ok := ml.MessageListRows[id]; ok {
		attempts := 0
		var tryScroll func() bool
		tryScroll = func() bool {
			attempts++
			_, destY, success := row.TranslateCoordinates(ml.ListBox, 0, 0)
			if success && destY >= 0 {
				adj := ml.ScrolledWindow.VAdjustment()
				adj.SetValue(destY)
				ml.HighlightMessage(id)
				return false
			}
			if attempts >= 10 {
				return false
			}
			return true
		}
		glib.TimeoutAdd(50, tryScroll)
	} else if ml.OnLoadMessageRequest != nil {
		ml.OnLoadMessageRequest(id)
	}
}

func (ml *MessageList) AddSeparator(text string) {
	row := gtk.NewListBoxRow()
	row.SetFocusable(false)
	row.SetSelectable(false)
	row.AddCSSClass("message-row")
	row.AddCSSClass("message-row-connected")

	label := gtk.NewLabel(text)
	label.AddCSSClass("date-separator")
	row.SetChild(label)
	row.SetHAlign(gtk.AlignCenter)

	if ml.InsertIndex >= 0 {
		ml.ListBox.Insert(row, ml.InsertIndex)
		ml.InsertIndex++
	} else {
		ml.ListBox.Append(row)
	}
}

func (ml *MessageList) EnterSelectionMode(initialMsgID string) {
	ml.IsSelectionMode = true
	ml.SelectedIDs = make(map[string]bool)
	if initialMsgID != "" {
		ml.SelectedIDs[initialMsgID] = true
	}

	for id, row := range ml.MessageListRows {
		if ml.SelectedIDs[id] {
			row.AddCSSClass("message-row-selected")
		} else {
			row.RemoveCSSClass("message-row-selected")
		}
	}

	count := len(ml.SelectedIDs)
	if count == 1 {
		ml.SelectionCountLabel.SetText("1 mensagem selecionada")
	} else {
		ml.SelectionCountLabel.SetText(fmt.Sprintf("%d mensagens selecionadas", count))
	}
	ml.SelectionBar.Show()
}

func (ml *MessageList) ToggleMessageSelection(id string) {
	if !ml.IsSelectionMode {
		return
	}
	if ml.SelectedIDs[id] {
		delete(ml.SelectedIDs, id)
		if row, ok := ml.MessageListRows[id]; ok {
			row.RemoveCSSClass("message-row-selected")
		}
	} else {
		ml.SelectedIDs[id] = true
		if row, ok := ml.MessageListRows[id]; ok {
			row.AddCSSClass("message-row-selected")
		}
	}

	count := len(ml.SelectedIDs)
	if count == 0 {
		ml.ExitSelectionMode()
		return
	}

	if count == 1 {
		ml.SelectionCountLabel.SetText("1 mensagem selecionada")
	} else {
		ml.SelectionCountLabel.SetText(fmt.Sprintf("%d mensagens selecionadas", count))
	}
}

func (ml *MessageList) ExitSelectionMode() {
	ml.IsSelectionMode = false
	ml.SelectedIDs = make(map[string]bool)
	if ml.SelectionBar != nil {
		ml.SelectionBar.Hide()
	}
	for _, row := range ml.MessageListRows {
		row.RemoveCSSClass("message-row-selected")
	}
}

// GetOrderedMessageIDs returns the message IDs in visual order (top to bottom).
func (ml *MessageList) GetOrderedMessageIDs() []string {
	var ids []string
	for i := 0; ; i++ {
		row := ml.ListBox.RowAtIndex(i)
		if row == nil {
			break
		}
		if id, ok := ml.RowToMessageID[row]; ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// HasKeyboardSelection reports whether a message is currently selected via keyboard.
func (ml *MessageList) HasKeyboardSelection() bool {
	return ml.KeyboardSelectedID != ""
}

// SelectMessageUp moves the keyboard selection up (older message) or selects the bottom-most message.
func (ml *MessageList) SelectMessageUp() bool {
	ids := ml.GetOrderedMessageIDs()
	if len(ids) == 0 {
		return false
	}

	if ml.KeyboardSelectedID == "" {
		ml.setKeyboardSelection(ids[len(ids)-1])
		return true
	}

	currIdx := -1
	for i, id := range ids {
		if id == ml.KeyboardSelectedID {
			currIdx = i
			break
		}
	}

	if currIdx > 0 {
		ml.setKeyboardSelection(ids[currIdx-1])
		return true
	} else if currIdx == 0 {
		// Keep top message selected
		return true
	}

	ml.setKeyboardSelection(ids[len(ids)-1])
	return true
}

// SelectMessageDown moves the keyboard selection down (newer message). If moving past the bottom, deselects.
func (ml *MessageList) SelectMessageDown() bool {
	if ml.KeyboardSelectedID == "" {
		return false
	}

	ids := ml.GetOrderedMessageIDs()
	if len(ids) == 0 {
		ml.ClearKeyboardSelection()
		return false
	}

	currIdx := -1
	for i, id := range ids {
		if id == ml.KeyboardSelectedID {
			currIdx = i
			break
		}
	}

	if currIdx >= 0 && currIdx < len(ids)-1 {
		ml.setKeyboardSelection(ids[currIdx+1])
		return true
	}

	// Reached after the newest message: clear selection
	ml.ClearKeyboardSelection()
	return true
}

// ConfirmKeyboardReply initiates a reply to the currently keyboard-selected message.
func (ml *MessageList) ConfirmKeyboardReply() bool {
	if ml.KeyboardSelectedID == "" {
		return false
	}

	id := ml.KeyboardSelectedID
	bubble, ok := ml.MessageRows[id]
	if !ok {
		ml.ClearKeyboardSelection()
		return false
	}

	sender := bubble.Sender()
	if sender == "" {
		sender = "Unknown"
	}
	content := bubble.Content()

	ml.ClearKeyboardSelection()
	if ml.OnReplyRequest != nil {
		ml.OnReplyRequest(id, sender, content)
		return true
	}
	return false
}

// ClearKeyboardSelection clears any active keyboard selection highlight.
func (ml *MessageList) ClearKeyboardSelection() {
	if ml.KeyboardSelectedID != "" {
		if row, ok := ml.MessageListRows[ml.KeyboardSelectedID]; ok {
			row.RemoveCSSClass("message-row-reply-target")
		}
		ml.KeyboardSelectedID = ""
	}
}

func (ml *MessageList) setKeyboardSelection(id string) {
	if ml.KeyboardSelectedID != "" && ml.KeyboardSelectedID != id {
		if prevRow, ok := ml.MessageListRows[ml.KeyboardSelectedID]; ok {
			prevRow.RemoveCSSClass("message-row-reply-target")
		}
	}

	ml.KeyboardSelectedID = id
	if row, ok := ml.MessageListRows[id]; ok {
		row.AddCSSClass("message-row-reply-target")
		ml.scrollToKeyboardSelected(row)
	}
}

func (ml *MessageList) scrollToKeyboardSelected(row *gtk.ListBoxRow) {
	attempts := 0
	var tryScroll func() bool
	tryScroll = func() bool {
		attempts++
		_, destY, success := row.TranslateCoordinates(ml.ListBox, 0, 0)
		if success && destY >= 0 {
			adj := ml.ScrolledWindow.VAdjustment()
			val := adj.Value()
			pageSize := adj.PageSize()
			h := float64(row.Height())
			if h <= 0 {
				h = 40
			}

			if destY < val {
				adj.SetValue(destY - 20)
			} else if destY+h > val+pageSize {
				adj.SetValue(destY + h - pageSize + 20)
			}
			return false
		}
		if attempts >= 5 {
			return false
		}
		return true
	}
	glib.TimeoutAdd(20, tryScroll)
}

func (ml *MessageList) showContextMenu(id string, b bubbles.Bubble) {
	popover := gtk.NewPopover()
	popover.ConnectClosed(func() {
		if ml.OnContextMenuClosed != nil {
			ml.OnContextMenuClosed()
		}
		glib.IdleAdd(func() {
			popover.Unparent()
		})
	})
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	
	fwdBtn := gtk.NewButtonWithLabel("Encaminhar")
	fwdBtn.SetHasFrame(false)
	fwdBtn.ConnectClicked(func() {
		popover.Popdown()
		if ml.OnForwardMessagesRequest != nil {
			ml.OnForwardMessagesRequest([]string{id})
		}
	})
	box.Append(fwdBtn)

	selBtn := gtk.NewButtonWithLabel("Selecionar")
	selBtn.SetHasFrame(false)
	selBtn.ConnectClicked(func() {
		popover.Popdown()
		ml.EnterSelectionMode(id)
	})
	box.Append(selBtn)

	box.Append(gtk.NewSeparator(gtk.OrientationHorizontal))

	pinBtn := gtk.NewButtonWithLabel("Pin Message")
	pinBtn.SetHasFrame(false)
	pinBtn.ConnectClicked(func() {
		popover.Popdown()
		durationPopover := gtk.NewPopover()
		dBox := gtk.NewBox(gtk.OrientationVertical, 0)
		durations := []struct {
			Label string
			Secs  uint32
		}{
			{"24 Hours", 86400},
			{"7 Days", 604800},
			{"30 Days", 2592000},
		}
		for _, d := range durations {
			btn := gtk.NewButtonWithLabel(d.Label)
			btn.SetHasFrame(false)
			dSecs := d.Secs
			btn.ConnectClicked(func() {
				if ml.OnPinMessage != nil {
					ml.OnPinMessage(id, true, dSecs)
				}
				durationPopover.Popdown()
			})
			dBox.Append(btn)
		}
		durationPopover.SetChild(dBox)
		durationPopover.SetParent(b.Widget().(gtk.Widgetter))
		durationPopover.Popup()
	})
	box.Append(pinBtn)

	unpinBtn := gtk.NewButtonWithLabel("Unpin Message")
	unpinBtn.SetHasFrame(false)
	unpinBtn.ConnectClicked(func() {
		if ml.OnPinMessage != nil {
			ml.OnPinMessage(id, false, 0)
		}
		popover.Popdown()
	})
	box.Append(unpinBtn)

	if mPath := b.MediaPath(); mPath != "" {
		if absPath, err := filepath.Abs(mPath); err == nil {
			mPath = absPath
		}
		box.Append(gtk.NewSeparator(gtk.OrientationHorizontal))

		openBtn := gtk.NewButtonWithLabel("Open")
		openBtn.SetHasFrame(false)
		openBtn.ConnectClicked(func() {
			exec.Command("xdg-open", mPath).Start()
			popover.Popdown()
		})
		box.Append(openBtn)

		folderBtn := gtk.NewButtonWithLabel("Show in Folder")
		folderBtn.SetHasFrame(false)
		folderBtn.ConnectClicked(func() {
			uri := "file://" + mPath
			cmd := exec.Command("dbus-send", "--session", "--dest=org.freedesktop.FileManager1",
				"--type=method_call", "/org/freedesktop/FileManager1",
				"org.freedesktop.FileManager1.ShowItems",
				"array:string:"+uri, "string:")
			if err := cmd.Run(); err != nil {
				exec.Command("xdg-open", filepath.Dir(mPath)).Start()
			}
			popover.Popdown()
		})
		box.Append(folderBtn)

		copyPathBtn := gtk.NewButtonWithLabel("Copy Path")
		copyPathBtn.SetHasFrame(false)
		copyPathBtn.ConnectClicked(func() {
			gdk.DisplayGetDefault().Clipboard().SetText(mPath)
			popover.Popdown()
		})
		box.Append(copyPathBtn)
	}

	popover.SetChild(box)
	popover.SetParent(b.Widget().(gtk.Widgetter))
	popover.Popup()
}

func (ml *MessageList) addBubble(id string, b bubbles.Bubble, isCont bool) {
	row := gtk.NewListBoxRow()
	row.SetFocusable(false)
	row.AddCSSClass("message-row")
	if isCont {
		row.AddCSSClass("message-row-connected")
	}

	row.SetChild(b.Widget())

	click := gtk.NewGestureClick()
	click.SetButton(3)
	click.ConnectPressed(func(n int, x, y float64) {
		ml.showContextMenu(id, b)
	})
	row.AddController(click)

	leftClick := gtk.NewGestureClick()
	leftClick.SetButton(1)
	leftClick.SetPropagationPhase(gtk.PhaseCapture)
	leftClick.ConnectPressed(func(n int, x, y float64) {
		if ml.IsSelectionMode {
			if id != "" {
				ml.ToggleMessageSelection(id)
			}
			return
		}
		if ml.IsSearching && id != "" {
			if ml.OnSearchResultClick != nil {
				ml.OnSearchResultClick(id)
			}
		}
	})
	row.AddController(leftClick)

	if id != "" {
		ml.MessageListRows[id] = row
		ml.RowToMessageID[row] = id
	}

	if ml.InsertIndex >= 0 {
		ml.ListBox.Insert(row, ml.InsertIndex)
		ml.InsertIndex++
	} else {
		ml.ListBox.Append(row)
	}
}

func (ml *MessageList) registerBubble(id, jid string, bubble bubbles.Bubble, isCont bool) {
	if id != "" {
		ml.MessageRows[id] = bubble
	}
	if jid != "" {
		ml.BubblesByJID[jid] = append(ml.BubblesByJID[jid], bubble)
	}
	
	bubble.SetOnQuotedClick(func(qid string) {
		ml.ScrollToMessage(qid)
	})
	bubble.SetOnReplyRequest(func() {
		sender := bubble.Sender()
		if sender == "" { sender = "Unknown" }
		if ml.OnReplyRequest != nil {
			ml.OnReplyRequest(id, sender, bubble.Content())
		}
	})
	bubble.SetOnReactionRequest(func(emoji string) {
		if ml.OnSendReaction != nil {
			ml.OnSendReaction(id, emoji)
		}
	})
	bubble.SetOnMentionClick(func(mjid string) {
		if ml.OnMentionClick != nil {
			ml.OnMentionClick(mjid)
		}
	})
	bubble.SetOnBubbleClick(func() {
		if ml.IsSearching && id != "" {
			if ml.OnSearchResultClick != nil {
				ml.OnSearchResultClick(id)
			}
		}
	})

	ml.addBubble(id, bubble, isCont)
}

func (ml *MessageList) AddMessage(id, jid, name, text string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string) {
	bubble, err := bubbles.NewTextBubble(name, text, isSelf, status, tStr, av)
	if err == nil {
		bubble.SetQuotedMessage(qID, qSender, qContent)
		ml.registerBubble(id, jid, bubble, isCont)
	}
}

func (ml *MessageList) AddImage(id, jid, name, text string, tex, thumb *gdk.Texture, path string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string, w, h int) {
	bubble, err := bubbles.NewImageBubble(name, text, tex, thumb, isSelf, status, tStr, av, w, h)
	if err == nil {
		bubble.SetFilePath(path)
		bubble.OnDownloadRequest = func() {
			if ml.OnDownloadMedia != nil {
				ml.OnDownloadMedia(id)
			}
		}
		bubble.OnOpenRequest = func(path string) {
			if ml.OnOpenImage != nil {
				ml.OnOpenImage(path)
			}
		}
		bubble.SetQuotedMessage(qID, qSender, qContent)
		ml.registerBubble(id, jid, bubble, isCont)
	}
}

func (ml *MessageList) AddSticker(id, jid, name string, anim *gdkpixbuf.PixbufAnimation, tex, thumb *gdk.Texture, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string, w, h int) {
	bubble, err := bubbles.NewStickerBubble(name, anim, tex, thumb, isSelf, status, tStr, av, w, h)
	if err == nil {
		bubble.OnDownloadRequest = func() {
			if ml.OnDownloadMedia != nil {
				ml.OnDownloadMedia(id)
			}
		}
		bubble.SetQuotedMessage(qID, qSender, qContent)
		ml.registerBubble(id, jid, bubble, isCont)
	}
}

func (ml *MessageList) AddAudio(id, jid, name string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string) {
	bubble, err := bubbles.NewAudioBubble(name, isSelf, status, tStr, av)
	if err == nil {
		bubble.OnDownloadRequest = func() {
			if ml.OnDownloadMedia != nil {
				ml.OnDownloadMedia(id)
			}
		}
		bubble.OnPlayRequest = func() {
			go func() {
				err := ml.AudioPlayer.Play(bubble.AudioPath(), func() {
					glib.IdleAdd(func() {
						bubble.SetPlaying(false)
						bubble.SetProgress(0)
					})
				}, func(current, total time.Duration) {
					if total > 0 {
						progress := float64(current) / float64(total) * 100
						glib.IdleAdd(func() {
							bubble.SetProgress(progress)
						})
					}
				})
				if err != nil {
					fmt.Printf("MessageList: Audio play error: %v\n", err)
					glib.IdleAdd(func() {
						bubble.SetPlaying(false)
					})
					return
				}
				glib.IdleAdd(func() {
					bubble.SetPlaying(true)
				})
			}()
		}
		bubble.OnStopRequest = func() {
			ml.AudioPlayer.Stop()
		}
		bubble.OnSeekRequest = func(percent float64) {
			ml.AudioPlayer.Seek(percent)
		}
		bubble.SetQuotedMessage(qID, qSender, qContent)
		ml.registerBubble(id, jid, bubble, isCont)
	}
}

func (ml *MessageList) AddVideo(id, jid, name, text string, thumb *gdk.Texture, path string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string, w, h int) {
	bubble, err := bubbles.NewImageBubble(name, text, nil, thumb, isSelf, status, tStr, av, w, h)
	if err == nil {
		bubble.SetFilePath(path)
		bubble.OnDownloadRequest = func() {
			if ml.OnDownloadMedia != nil {
				ml.OnDownloadMedia(id)
			}
		}
		bubble.OnOpenRequest = func(path string) {
			if ml.OnOpenImage != nil {
				ml.OnOpenImage(path)
			}
		}
		bubble.SetQuotedMessage(qID, qSender, qContent)
		ml.registerBubble(id, jid, bubble, isCont)
	}
}

func (ml *MessageList) AddDocument(id, jid, name, fileName string, thumb *gdk.Texture, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string) {
	bubble, err := bubbles.NewDocumentBubble(name, fileName, thumb, isSelf, status, tStr, av)
	if err == nil {
		bubble.OnDownloadRequest = func() {
			if ml.OnDownloadMedia != nil {
				ml.OnDownloadMedia(id)
			}
		}
		bubble.SetQuotedMessage(qID, qSender, qContent)
		ml.registerBubble(id, jid, bubble, isCont)
	}
}

func (ml *MessageList) AddPoll(id, jid, name, question string, options []string, votes map[string][]string, myJID string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string) {
	bubble, err := bubbles.NewPollBubble(id, name, question, options, isSelf, status, tStr, av)
	if err == nil {
		if votes != nil {
			bubble.UpdateVotes(votes, myJID)
		}
		bubble.SetOnVote(func(selected []string) {
			if ml.OnSendPollVote != nil {
				ml.OnSendPollVote(id, jid, isSelf, selected)
			}
		})
		bubble.SetQuotedMessage(qID, qSender, qContent)
		ml.registerBubble(id, jid, bubble, isCont)
	}
}

func (ml *MessageList) UpdatePollVotes(msgID string, votes map[string][]string, myJID string) {
	if bubble, ok := ml.MessageRows[msgID]; ok {
		if pb, isPoll := bubble.(*bubbles.PollBubble); isPoll {
			pb.UpdateVotes(votes, myJID)
		}
	}
}

func (ml *MessageList) UpdateMessageStatus(id, status string) {
	if bubble, exists := ml.MessageRows[id]; exists {
		glib.IdleAdd(func() { bubble.SetStatus(status) })
	}
}

func (ml *MessageList) UpdateMessageContent(id, content string, isEdited bool) {
	if bubble, exists := ml.MessageRows[id]; exists {
		glib.IdleAdd(func() {
			bubble.SetContentText(content)
			bubble.SetEdited(isEdited)
		})
	}
}

func (ml *MessageList) UpdateMessageReactions(id string, reactions []string) {
	if bubble, exists := ml.MessageRows[id]; exists {
		glib.IdleAdd(func() { bubble.SetReactions(reactions) })
	}
}

func (ml *MessageList) UpdateMessagePinned(id string, pinned bool) {
	if bubble, exists := ml.MessageRows[id]; exists {
		glib.IdleAdd(func() { bubble.SetPinned(pinned) })
	}
}

func (ml *MessageList) UpdateMessageForwarded(id string, forwarded bool) {
	if bubble, exists := ml.MessageRows[id]; exists {
		glib.IdleAdd(func() { bubble.SetForwarded(forwarded) })
	}
}

func (ml *MessageList) UpdateMessageSticker(id string, anim *gdkpixbuf.PixbufAnimation, tex *gdk.Texture, path string) {
	if bubble, exists := ml.MessageRows[id]; exists {
		if sb, ok := bubble.(*bubbles.StickerBubble); ok {
			sb.UpdateStickerImage(anim, tex, path)
		}
	}
}

func (ml *MessageList) UpdateMessageImage(id string, tex *gdk.Texture, path string) {
	if bubble, exists := ml.MessageRows[id]; exists {
		glib.IdleAdd(func() { bubble.UpdateImage(tex, path) })
	}
}

func (ml *MessageList) UpdateMessageAudio(id string, path string) {
	if bubble, exists := ml.MessageRows[id]; exists {
		if ab, ok := bubble.(*bubbles.AudioBubble); ok {
			ab.SetAudioPath(path)
		}
	}
}

func (ml *MessageList) UpdateMessageDocument(id string, path string) {
	if bubble, exists := ml.MessageRows[id]; exists {
		if db, ok := bubble.(*bubbles.DocumentBubble); ok {
			db.UpdateDocument(path)
		}
	}
}
