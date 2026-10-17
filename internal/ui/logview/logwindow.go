package logview

import (
	"strings"
	"sync"

	"whats-gtk/internal/logger"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const defaultMaxVisibleLines = 100

// LogWindow represents the system log viewer window.
type LogWindow struct {
	Window          *adw.Window
	TextView        *gtk.TextView
	TextBuffer      *gtk.TextBuffer
	ScrolledWindow  *gtk.ScrolledWindow
	AutoScrollBtn   *gtk.ToggleButton
	SearchEntry     *gtk.SearchEntry
	LineLimitCombo  *gtk.DropDown
	buffer          *logger.RingBuffer
	unregister      func()
	endMark         *gtk.TextMark
	maxVisibleLines int

	mu           sync.Mutex
	pendingLines []string
	timerSource  glib.SourceHandle
}

// NewLogWindow creates a new fast, bounded LogWindow instance.
func NewLogWindow(parent *gtk.Window, buffer *logger.RingBuffer) *LogWindow {
	win := adw.NewWindow()
	win.SetTitle("Logs do Sistema")
	win.SetDefaultSize(820, 520)
	if parent != nil {
		win.SetTransientFor(parent)
	}

	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// Header Bar
	headerBar := adw.NewHeaderBar()

	// Title Widget
	titleWidget := adw.NewWindowTitle("Logs do Sistema", "Diagnóstico e eventos em tempo real")
	headerBar.SetTitleWidget(titleWidget)

	// Copy Button
	copyBtn := gtk.NewButtonFromIconName("edit-copy-symbolic")
	copyBtn.SetTooltipText("Copiar logs para a área de transferência")
	headerBar.PackStart(copyBtn)

	// Clear Button
	clearBtn := gtk.NewButtonFromIconName("user-trash-symbolic")
	clearBtn.SetTooltipText("Limpar histórico de logs")
	headerBar.PackStart(clearBtn)

	// Auto-scroll toggle
	autoScrollBtn := gtk.NewToggleButton()
	autoScrollBtn.SetIconName("go-bottom-symbolic")
	autoScrollBtn.SetTooltipText("Rolar automaticamente para novos logs")
	autoScrollBtn.SetActive(true)
	headerBar.PackEnd(autoScrollBtn)

	// Line limit dropdown (100, 250, 500)
	limitStrings := []string{"100 linhas", "250 linhas", "500 linhas"}
	limitDropdown := gtk.NewDropDownFromStrings(limitStrings)
	limitDropdown.SetTooltipText("Quantidade máxima de linhas renderizadas na tela")
	limitDropdown.SetSelected(0) // Default: 100 lines
	headerBar.PackEnd(limitDropdown)

	mainBox.Append(headerBar)

	// Search / Filter Entry Bar
	searchBarBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	searchBarBox.SetMarginTop(6)
	searchBarBox.SetMarginBottom(6)
	searchBarBox.SetMarginStart(10)
	searchBarBox.SetMarginEnd(10)

	searchEntry := gtk.NewSearchEntry()
	searchEntry.SetPlaceholderText("Filtrar logs por palavra-chave...")
	searchEntry.SetHExpand(true)
	searchBarBox.Append(searchEntry)

	mainBox.Append(searchBarBox)

	// Text View & Scrolled Window
	textBuffer := gtk.NewTextBuffer(nil)
	textView := gtk.NewTextViewWithBuffer(textBuffer)
	textView.SetMonospace(true)
	textView.SetEditable(false)
	textView.SetCursorVisible(false)
	textView.SetWrapMode(gtk.WrapNone)
	textView.SetLeftMargin(10)
	textView.SetRightMargin(10)
	textView.SetTopMargin(8)
	textView.SetBottomMargin(8)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetHExpand(true)
	scrolled.SetChild(textView)

	mainBox.Append(scrolled)
	win.SetContent(mainBox)

	// End mark for scrolling
	endIter := textBuffer.EndIter()
	endMark := textBuffer.CreateMark("end-mark", endIter, false)

	lw := &LogWindow{
		Window:          win,
		TextView:        textView,
		TextBuffer:      textBuffer,
		ScrolledWindow:  scrolled,
		AutoScrollBtn:   autoScrollBtn,
		SearchEntry:     searchEntry,
		LineLimitCombo:  limitDropdown,
		buffer:          buffer,
		endMark:         endMark,
		maxVisibleLines: defaultMaxVisibleLines,
	}

	// Line limit selection changed
	limitDropdown.Connect("notify::selected", func() {
		switch limitDropdown.Selected() {
		case 0:
			lw.maxVisibleLines = 100
		case 1:
			lw.maxVisibleLines = 250
		case 2:
			lw.maxVisibleLines = 500
		default:
			lw.maxVisibleLines = 100
		}
		lw.reloadText(searchEntry.Text())
	})

	// Populate initial bounded lines
	lw.reloadText(searchEntry.Text())

	// Connect search changes
	searchEntry.ConnectSearchChanged(func() {
		lw.reloadText(searchEntry.Text())
	})

	// Copy to clipboard
	copyBtn.ConnectClicked(func() {
		lines := buffer.Lines()
		if len(lines) == 0 {
			start := textBuffer.StartIter()
			end := textBuffer.EndIter()
			lines = []string{textBuffer.Text(start, end, false)}
		}
		allText := strings.Join(lines, "\n")
		if disp := gdk.DisplayGetDefault(); disp != nil {
			disp.Clipboard().SetText(allText)
			copyBtn.SetTooltipText("Copiado!")
			glib.TimeoutAdd(2000, func() bool {
				copyBtn.SetTooltipText("Copiar logs para a área de transferência")
				return false
			})
		}
	})

	// Clear logs
	clearBtn.ConnectClicked(func() {
		buffer.Clear()
		lw.mu.Lock()
		lw.pendingLines = nil
		lw.mu.Unlock()
		textBuffer.SetText("")
	})

	// Register live listener to queue lines
	lw.unregister = buffer.RegisterListener(func(line string) {
		lw.mu.Lock()
		lw.pendingLines = append(lw.pendingLines, line)
		lw.mu.Unlock()
	})

	// Throttled UI flush timer (runs at most 4 times a second: every 250ms)
	lw.timerSource = glib.TimeoutAdd(250, func() bool {
		lw.flushPendingLines()
		return true
	})

	win.ConnectDestroy(func() {
		if lw.timerSource != 0 {
			glib.SourceRemove(lw.timerSource)
			lw.timerSource = 0
		}
		if lw.unregister != nil {
			lw.unregister()
			lw.unregister = nil
		}
	})

	return lw
}

// flushPendingLines processes accumulated log lines and maintains maxVisibleLines.
func (lw *LogWindow) flushPendingLines() {
	lw.mu.Lock()
	if len(lw.pendingLines) == 0 {
		lw.mu.Unlock()
		return
	}
	linesToProcess := lw.pendingLines
	lw.pendingLines = nil
	lw.mu.Unlock()

	filter := strings.ToLower(lw.SearchEntry.Text())
	var sb strings.Builder

	for _, line := range linesToProcess {
		if filter != "" && !strings.Contains(strings.ToLower(line), filter) {
			continue
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	newText := sb.String()
	if newText == "" {
		return
	}

	endIter := lw.TextBuffer.EndIter()
	lw.TextBuffer.Insert(endIter, newText)

	// Enforce maxVisibleLines in the GtkTextBuffer
	currentLineCount := lw.TextBuffer.LineCount()
	if currentLineCount > lw.maxVisibleLines {
		toRemove := currentLineCount - lw.maxVisibleLines
		startIter := lw.TextBuffer.StartIter()
		if deleteToIter, ok := lw.TextBuffer.IterAtLine(toRemove); ok {
			lw.TextBuffer.Delete(startIter, deleteToIter)
		}
	}

	if lw.AutoScrollBtn.Active() {
		lw.TextView.ScrollToMark(lw.endMark, 0.0, false, 0.0, 1.0)
	}
}

func (lw *LogWindow) reloadText(filter string) {
	lw.mu.Lock()
	lw.pendingLines = nil
	lw.mu.Unlock()

	lines := lw.buffer.Lines()
	filter = strings.ToLower(filter)

	var filtered []string
	for _, l := range lines {
		if filter == "" || strings.Contains(strings.ToLower(l), filter) {
			filtered = append(filtered, l)
		}
	}

	// Only take the last maxVisibleLines!
	if len(filtered) > lw.maxVisibleLines {
		filtered = filtered[len(filtered)-lw.maxVisibleLines:]
	}

	var sb strings.Builder
	for _, l := range filtered {
		sb.WriteString(l)
		sb.WriteString("\n")
	}
	lw.TextBuffer.SetText(sb.String())

	if lw.AutoScrollBtn.Active() {
		glib.IdleAdd(func() {
			lw.TextView.ScrollToMark(lw.endMark, 0.0, false, 0.0, 1.0)
		})
	}
}

// Show presents the log window.
func (lw *LogWindow) Show() {
	if lw.Window != nil {
		lw.Window.Present()
	}
}
