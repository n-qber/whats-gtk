package chat

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"whats-gtk/internal/ui/chat/bubbles"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
)

type ChatView struct {
	Box                   *gtk.Box
	MessageList           *gtk.ListBox
	MessageScrolledWindow *gtk.ScrolledWindow
	MessageInput          *gtk.TextView
	ChatHeaderLabel       *gtk.Label
	ChatHeaderImage       *adw.Avatar
	MessageRows           map[string]bubbles.Bubble
	MessageListRows       map[string]*gtk.ListBoxRow
	BubblesByJID          map[string][]bubbles.Bubble
	OnSendMessage         func(text string, replyToID string)
	OnPasteImage          func(tex *gdk.Texture)
	OnSendFile            func(path string)
	OnDownloadMedia       func(id string)
	OnOpenImage           func(path string)
	OnSendReaction        func(id, emoji string)
	OnPinMessage          func(id string, pin bool, duration uint32)
	OnDetach              func()
	AudioPlayer           *AudioPlayer
	ReplyToID             string
	ReplyToSender         string
	ReplyToContent        string
	ReplyPreviewBox       *gtk.Box
	ReplyPreviewLabel     *gtk.Label
	PinnedMessageBar      *gtk.Box
	PinnedMessageLabel    *gtk.Label
	OnLoadOlder           func()
	IsLoadingOlder        bool
	InsertIndex           int
	SearchBar             *gtk.SearchBar
	SearchEntry           *gtk.SearchEntry
	OnSearchMessages      func(query string)
	OnCancelSearch        func()
	IsSearching           bool
	OnMentionClick        func(jid string)
	OnSendPollVote        func(msgID string, selectedOptions []string)
}

func NewChatView() (*ChatView, error) {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.SetName("chat-view-box")

	header := adw.NewHeaderBar()
	
	headerLabel := gtk.NewLabel("Select a chat")
	headerLabel.AddCSSClass("chat-header-name")
	header.SetTitleWidget(headerLabel)
	
	headerAvatar := adw.NewAvatar(32, "", true)
	header.PackStart(headerAvatar)
	
	detachBtn := gtk.NewButtonFromIconName("window-new-symbolic")
	header.PackEnd(detachBtn)

	box.Append(header)

	searchBar := gtk.NewSearchBar()
	searchEntry := gtk.NewSearchEntry()
	searchBar.ConnectEntry(searchEntry)
	searchBar.SetChild(searchEntry)
	searchBar.SetKeyCaptureWidget(box)
	searchBar.SetShowCloseButton(true)
	box.Append(searchBar)

	pinnedMessageBar := gtk.NewBox(gtk.OrientationHorizontal, 5)
	pinnedMessageBar.AddCSSClass("pinned-message-bar")
	pinnedIcon := gtk.NewImageFromIconName("pin-symbolic")
	pinnedLabel := gtk.NewLabel("Pinned Message")
	pinnedLabel.SetXAlign(0)
	pinnedLabel.SetEllipsize(pango.EllipsizeEnd)
	pinnedMessageBar.Append(pinnedIcon)
	pinnedMessageBar.Append(pinnedLabel)
	pinnedMessageBar.Hide()
	box.Append(pinnedMessageBar)

	messageList := gtk.NewListBox()
	messageList.SetName("message-list")
	messageList.SetSelectionMode(gtk.SelectionNone)

	scrolledMsg := gtk.NewScrolledWindow()
	scrolledMsg.SetVExpand(true)
	scrolledMsg.SetChild(messageList)
	box.Append(scrolledMsg)

	replyPreviewBox := gtk.NewBox(gtk.OrientationHorizontal, 5)
	replyPreviewBox.AddCSSClass("reply-preview")
	replyLabel := gtk.NewLabel("")
	replyLabel.SetXAlign(0)
	replyLabel.SetEllipsize(pango.EllipsizeEnd)
	closeReplyBtn := gtk.NewButtonFromIconName("window-close-symbolic")
	replyPreviewBox.Append(replyLabel)
	replyPreviewBox.Append(closeReplyBtn)
	replyPreviewBox.Hide()
	box.Append(replyPreviewBox)

	inputBox := gtk.NewBox(gtk.OrientationHorizontal, 5)
	inputBox.SetMarginTop(6)
	inputBox.SetMarginBottom(6)
	inputBox.SetMarginStart(6)
	inputBox.SetMarginEnd(6)
	
	inputScrolled := gtk.NewScrolledWindow()
	inputScrolled.SetMinContentHeight(36)
	inputScrolled.SetMaxContentHeight(200)
	inputScrolled.SetPropagateNaturalHeight(true)
	inputScrolled.SetHExpand(true)
	inputScrolled.AddCSSClass("message-input-scrolled")

	messageInput := gtk.NewTextView()
	messageInput.SetWrapMode(gtk.WrapWordChar)
	messageInput.SetAcceptsTab(false)
	messageInput.AddCSSClass("message-input-view")
	inputScrolled.SetChild(messageInput)

	fileButton := gtk.NewButtonFromIconName("mail-attachment-symbolic")
	fileButton.SetVAlign(gtk.AlignEnd)

	sendButton := gtk.NewButtonWithLabel("Send")
	sendButton.SetVAlign(gtk.AlignEnd)

	inputBox.Append(fileButton)
	inputBox.Append(inputScrolled)
	inputBox.Append(sendButton)
	box.Append(inputBox)

	cv := &ChatView{
		Box:                   box,
		MessageList:           messageList,
		MessageScrolledWindow: scrolledMsg,
		MessageInput:          messageInput,
		ChatHeaderLabel:       headerLabel,
		ChatHeaderImage:       headerAvatar,
		MessageRows:           make(map[string]bubbles.Bubble),
		MessageListRows:       make(map[string]*gtk.ListBoxRow),
		BubblesByJID:          make(map[string][]bubbles.Bubble),
		AudioPlayer:           NewAudioPlayer(),
		ReplyPreviewBox:       replyPreviewBox,
		ReplyPreviewLabel:     replyLabel,
		PinnedMessageBar:      pinnedMessageBar,
		PinnedMessageLabel:    pinnedLabel,
		InsertIndex:           -1,
		SearchBar:             searchBar,
		SearchEntry:           searchEntry,
	}

	searchBar.Connect("notify::search-mode-enabled", func() {
		if !searchBar.SearchMode() {
			if cv.OnCancelSearch != nil {
				cv.OnCancelSearch()
			}
		}
	})

	searchEntry.ConnectSearchChanged(func() {
		text := searchEntry.Text()
		if text == "" {
			if cv.OnCancelSearch != nil {
				cv.OnCancelSearch()
			}
		} else {
			if cv.OnSearchMessages != nil {
				cv.OnSearchMessages(text)
			}
		}
	})

	scrolledMsg.VAdjustment().ConnectValueChanged(func() {
		adj := scrolledMsg.VAdjustment()
		if adj.Value() <= 50.0 && !cv.IsLoadingOlder && cv.OnLoadOlder != nil {
			cv.IsLoadingOlder = true
			cv.OnLoadOlder()
		}
	})

	detachBtn.ConnectClicked(func() {
		if cv.OnDetach != nil {
			cv.OnDetach()
		}
	})

	closeReplyBtn.ConnectClicked(func() {
		cv.CancelReply()
	})

	sendMsg := func() {
		buffer := messageInput.Buffer()
		start, end := buffer.Bounds()
		text := buffer.Text(start, end, false)
		if text != "" && cv.OnSendMessage != nil {
			cv.OnSendMessage(text, cv.ReplyToID)
			cv.CancelReply()
			buffer.SetText("")
		}
	}

	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(keyval uint, keycode uint, state gdk.ModifierType) bool {
		if keyval == gdk.KEY_Return && (state&gdk.ShiftMask == 0) {
			sendMsg()
			return true
		}
		if keyval == gdk.KEY_v && (state&gdk.ControlMask != 0) {
			clipboard := gdk.DisplayGetDefault().Clipboard()
			if clipboard.Formats().ContainGType(gdk.GTypeTexture) {
				clipboard.ReadTextureAsync(context.TODO(), func(res gio.AsyncResulter) {
					tex, err := clipboard.ReadTextureFinish(res)
					if err == nil && cv.OnPasteImage != nil {
						cv.OnPasteImage(gdk.BaseTexture(tex))
					}
				})
				return true
			}
		}
		return false
	})
	messageInput.AddController(keyCtrl)

	fileButton.ConnectClicked(func() {
		dialog := gtk.NewFileDialog()
		
		var window *gtk.Window
		if root := cv.Box.Root(); root != nil {
			if win, ok := root.Cast().(*gtk.Window); ok {
				window = win
			}
		}

		dialog.Open(context.TODO(), window, func(res gio.AsyncResulter) {
			file, err := dialog.OpenFinish(res)
			if err == nil {
				if cv.OnSendFile != nil {
					cv.OnSendFile(file.Path())
				}
			}
		})
	})

	sendButton.ConnectClicked(sendMsg)

	return cv, nil
}

func (cv *ChatView) SetHeader(name string, tex *gdk.Texture) {
	if cv == nil { return }
	if cv.ChatHeaderLabel != nil {
		cv.ChatHeaderLabel.SetText(name)
	}
	if cv.ChatHeaderImage != nil {
		cv.ChatHeaderImage.SetText(name)
		if tex != nil {
			cv.ChatHeaderImage.SetCustomImage(tex)
		} else {
			// Explicitly pass untyped nil to avoid interface-with-nil-pointer panic
			cv.ChatHeaderImage.SetCustomImage(nil)
		}
	}
}

func (cv *ChatView) ToggleSearch() {
	if cv == nil || cv.SearchBar == nil { return }
	cv.SearchBar.SetSearchMode(!cv.SearchBar.SearchMode())
	if cv.SearchBar.SearchMode() {
		cv.SearchEntry.GrabFocus()
	}
}

func (cv *ChatView) SetReplyTo(id, sender, content string) {
	cv.ReplyToID = id
	cv.ReplyToSender = sender
	cv.ReplyToContent = content

	markupSender := sender
	if !strings.Contains(sender, "<span") {
		markupSender = glib.MarkupEscapeText(sender)
	}

	cv.ReplyPreviewLabel.SetMarkup("Replying to <b>" + markupSender + "</b>: " + glib.MarkupEscapeText(content))
	cv.ReplyPreviewBox.Show()
	cv.FocusEntry()
}

func (cv *ChatView) CancelReply() {
	cv.ReplyToID = ""
	cv.ReplyToSender = ""
	cv.ReplyToContent = ""
	cv.ReplyPreviewBox.Hide()
}

func (cv *ChatView) SetPinnedMessage(content string) {
	if content == "" {
		cv.PinnedMessageBar.Hide()
	} else {
		cv.PinnedMessageLabel.SetText(content)
		cv.PinnedMessageBar.Show()
	}
}

func (cv *ChatView) AddSeparator(text string) {
	row := gtk.NewListBoxRow()
	row.SetFocusable(false)
	row.SetSelectable(false)
	row.AddCSSClass("message-row")
	row.AddCSSClass("message-row-connected") // Make it tighter

	label := gtk.NewLabel(text)
	label.AddCSSClass("date-separator")
	row.SetChild(label)
	row.SetHAlign(gtk.AlignCenter)

	if cv.InsertIndex >= 0 {
		cv.MessageList.Insert(row, cv.InsertIndex)
		cv.InsertIndex++
	} else {
		cv.MessageList.Append(row)
	}
}

func (cv *ChatView) AddMessage(id, jid, name, text string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string) {
	bubble, err := bubbles.NewTextBubble(name, text, isSelf, status, tStr, av)
	if err == nil {
		bubble.SetQuotedMessage(qID, qSender, qContent)
		cv.registerBubble(id, jid, bubble, isCont)
	}
}

func (cv *ChatView) AddImage(id, jid, name, text string, tex, thumb *gdk.Texture, path string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string, w, h int) {
	bubble, err := bubbles.NewImageBubble(name, text, tex, thumb, isSelf, status, tStr, av, w, h)
	if err == nil {
		bubble.SetFilePath(path)
		bubble.OnDownloadRequest = func() {
			if cv.OnDownloadMedia != nil {
				cv.OnDownloadMedia(id)
			}
		}
		bubble.OnOpenRequest = func(path string) {
			if cv.OnOpenImage != nil {
				cv.OnOpenImage(path)
			}
		}
		bubble.SetQuotedMessage(qID, qSender, qContent)
		cv.registerBubble(id, jid, bubble, isCont)
	}
}

func (cv *ChatView) AddSticker(id, jid, name string, anim *gdkpixbuf.PixbufAnimation, tex, thumb *gdk.Texture, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string, w, h int) {
	bubble, err := bubbles.NewStickerBubble(name, anim, tex, thumb, isSelf, status, tStr, av, w, h)
	if err == nil {
		bubble.OnDownloadRequest = func() {
			if cv.OnDownloadMedia != nil {
				cv.OnDownloadMedia(id)
			}
		}
		bubble.SetQuotedMessage(qID, qSender, qContent)
		cv.registerBubble(id, jid, bubble, isCont)
	}
}

func (cv *ChatView) AddAudio(id, jid, name string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string) {
	bubble, err := bubbles.NewAudioBubble(name, isSelf, status, tStr, av)
	if err == nil {
		bubble.OnDownloadRequest = func() {
			if cv.OnDownloadMedia != nil {
				cv.OnDownloadMedia(id)
			}
		}
		bubble.OnPlayRequest = func() {
			go func() {
				err := cv.AudioPlayer.Play(bubble.AudioPath(), func() {
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
					fmt.Printf("ChatView: Audio play error: %v\n", err)
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
			cv.AudioPlayer.Stop()
		}
		bubble.OnSeekRequest = func(percent float64) {
			cv.AudioPlayer.Seek(percent)
		}
		bubble.SetQuotedMessage(qID, qSender, qContent)
		cv.registerBubble(id, jid, bubble, isCont)
	}
}

func (cv *ChatView) AddVideo(id, jid, name, text string, thumb *gdk.Texture, path string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string, w, h int) {
	// For now, video uses image bubble with thumbnail
	bubble, err := bubbles.NewImageBubble(name, text, nil, thumb, isSelf, status, tStr, av, w, h)
	if err == nil {
		bubble.SetFilePath(path)
		bubble.OnDownloadRequest = func() {
			if cv.OnDownloadMedia != nil {
				cv.OnDownloadMedia(id)
			}
		}
		bubble.OnOpenRequest = func(path string) {
			if cv.OnOpenImage != nil {
				cv.OnOpenImage(path)
			}
		}
		bubble.SetQuotedMessage(qID, qSender, qContent)
		cv.registerBubble(id, jid, bubble, isCont)
	}
}

func (cv *ChatView) AddDocument(id, jid, name, fileName string, thumb *gdk.Texture, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string) {
	bubble, err := bubbles.NewDocumentBubble(name, fileName, thumb, isSelf, status, tStr, av)
	if err == nil {
		bubble.OnDownloadRequest = func() {
			if cv.OnDownloadMedia != nil {
				cv.OnDownloadMedia(id)
			}
		}
		bubble.SetQuotedMessage(qID, qSender, qContent)
		cv.registerBubble(id, jid, bubble, isCont)
	}
}

func (cv *ChatView) AddPoll(id, jid, name, question string, options []string, votes map[string][]string, myJID string, isSelf, isCont bool, status, tStr string, av *gdk.Texture, qID, qSender, qContent string) {
	bubble, err := bubbles.NewPollBubble(id, name, question, options, isSelf, status, tStr, av)
	if err == nil {
		if votes != nil {
			bubble.UpdateVotes(votes, myJID)
		}
		bubble.SetOnVote(func(selected []string) {
			if cv.OnSendPollVote != nil {
				cv.OnSendPollVote(id, selected)
			}
		})
		bubble.SetQuotedMessage(qID, qSender, qContent)
		cv.registerBubble(id, jid, bubble, isCont)
	}
}

func (cv *ChatView) UpdatePollVotes(msgID string, votes map[string][]string, myJID string) {
	if bubble, ok := cv.MessageRows[msgID]; ok {
		if pb, isPoll := bubble.(*bubbles.PollBubble); isPoll {
			pb.UpdateVotes(votes, myJID)
		}
	}
}

func (cv *ChatView) registerBubble(id, jid string, bubble bubbles.Bubble, isCont bool) {
	if id != "" {
		cv.MessageRows[id] = bubble
	}
	if jid != "" {
		cv.BubblesByJID[jid] = append(cv.BubblesByJID[jid], bubble)
	}
	
	bubble.SetOnQuotedClick(func(qid string) {
		cv.ScrollToMessage(qid)
	})
	bubble.SetOnReplyRequest(func() {
		sender := bubble.Sender()
		if sender == "" { sender = "Unknown" }
		cv.SetReplyTo(id, sender, bubble.Content())
	})
	bubble.SetOnReactionRequest(func(emoji string) {
		if cv.OnSendReaction != nil {
			cv.OnSendReaction(id, emoji)
		}
	})
	bubble.SetOnMentionClick(func(mjid string) {
		if cv.OnMentionClick != nil {
			cv.OnMentionClick(mjid)
		}
	})

	cv.addBubble(id, bubble, isCont)
}

func (cv *ChatView) ScrollToMessage(id string) {
	if row, ok := cv.MessageListRows[id]; ok {
		glib.IdleAdd(func() {
			adj := cv.MessageScrolledWindow.VAdjustment()
			// In GTK4 we can use row.TranslateCoordinates to get position
			// or just use SelectRow and let the adjustment handle it if possible.
			// Better way:
			cv.MessageList.SelectRow(row)
			
			// Force scroll to row
			row.GrabFocus()
			
			// Get root coordinates of the row relative to the listbox
			_, y, _ := row.TranslateCoordinates(cv.MessageList, 0, 0)
			adj.SetValue(y)
		})
	}
}
func (cv *ChatView) showContextMenu(id string, b bubbles.Bubble) {
	popover := gtk.NewPopover()
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	
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
				if cv.OnPinMessage != nil {
					cv.OnPinMessage(id, true, dSecs)
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
		if cv.OnPinMessage != nil {
			cv.OnPinMessage(id, false, 0)
		}
		popover.Popdown()
	})
	box.Append(unpinBtn)

	if mPath := b.MediaPath(); mPath != "" {
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
				// Fallback to xdg-open on directory
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

func (cv *ChatView) addBubble(id string, b bubbles.Bubble, isCont bool) {
	row := gtk.NewListBoxRow()
	row.SetFocusable(false)
	row.AddCSSClass("message-row")
	if isCont {
		row.AddCSSClass("message-row-connected")
	}

	row.SetChild(b.Widget())

	click := gtk.NewGestureClick()
	click.SetButton(3) // Right click
	click.ConnectPressed(func(n int, x, y float64) {
		cv.showContextMenu(id, b)
	})
	row.AddController(click)

	if id != "" {
		cv.MessageListRows[id] = row
	}

	if cv.InsertIndex >= 0 {
		cv.MessageList.Insert(row, cv.InsertIndex)
		cv.InsertIndex++
	} else {
		cv.MessageList.Append(row)
		glib.IdleAdd(func() {
			cv.ScrollToBottom()
		})
	}
}

func (cv *ChatView) ScrollToBottom() {
	glib.TimeoutAdd(50, func() bool {
		adj := cv.MessageScrolledWindow.VAdjustment()
		adj.SetValue(adj.Upper() - adj.PageSize())
		return false
	})
}

func (cv *ChatView) UpdateMessageStatus(id, status string) {
	if bubble, exists := cv.MessageRows[id]; exists {
		glib.IdleAdd(func() { bubble.SetStatus(status) })
	}
}

func (cv *ChatView) UpdateMessageContent(id, content string, isEdited bool) {
	if bubble, exists := cv.MessageRows[id]; exists {
		glib.IdleAdd(func() {
			bubble.SetContentText(content)
			bubble.SetEdited(isEdited)
		})
	}
}

func (cv *ChatView) UpdateMessageReactions(id string, reactions []string) {
	if bubble, exists := cv.MessageRows[id]; exists {
		glib.IdleAdd(func() { bubble.SetReactions(reactions) })
	}
}

func (cv *ChatView) UpdateMessagePinned(id string, pinned bool) {
	if bubble, exists := cv.MessageRows[id]; exists {
		glib.IdleAdd(func() { bubble.SetPinned(pinned) })
	}
}

func (cv *ChatView) UpdateMessageSticker(id string, anim *gdkpixbuf.PixbufAnimation, tex *gdk.Texture, path string) {
	if bubble, exists := cv.MessageRows[id]; exists {
		if sb, ok := bubble.(*bubbles.StickerBubble); ok {
			sb.UpdateStickerImage(anim, tex, path)
		}
	}
}

func (cv *ChatView) UpdateMessageImage(id string, tex *gdk.Texture, path string) {
	if bubble, exists := cv.MessageRows[id]; exists {
		glib.IdleAdd(func() { bubble.UpdateImage(tex, path) })
	}
}

func (cv *ChatView) UpdateMessageDocument(id, path string) {
	if bubble, exists := cv.MessageRows[id]; exists {
		glib.IdleAdd(func() { bubble.UpdateDocument(path) })
	}
}

func (cv *ChatView) UpdateMessageAudio(id, path string) {
	if b, ok := cv.MessageRows[id]; ok {
		if ab, ok := b.(*bubbles.AudioBubble); ok {
			ab.SetAudioPath(path)
		}
	}
}

func (cv *ChatView) SetAvatar(jid string, tex *gdk.Texture) {
	if bubbles, exists := cv.BubblesByJID[jid]; exists {
		glib.IdleAdd(func() {
			for _, b := range bubbles {
				b.UpdateAvatar(tex)
			}
		})
	}
}

func (cv *ChatView) Clear() {
	cv.MessageRows = make(map[string]bubbles.Bubble)
	cv.BubblesByJID = make(map[string][]bubbles.Bubble)
	cv.MessageListRows = make(map[string]*gtk.ListBoxRow)
	cv.SetPinnedMessage("")
	
	for {
		child := cv.MessageList.FirstChild()
		if child == nil {
			break
		}
		cv.MessageList.Remove(child)
	}
}

func (cv *ChatView) FocusEntry() {
	glib.IdleAdd(func() {
		if cv.MessageInput != nil {
			cv.MessageInput.GrabFocus()
		}
	})
}

