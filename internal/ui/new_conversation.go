package ui

import (
	"strings"
	"unicode"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
)

type NewConversationDialog struct {
	Window       *adw.Window
	PhoneEntry   *adw.EntryRow
	MessageEntry *adw.EntryRow
	StartButton  *gtk.Button
	CancelButton *gtk.Button
	StatusLabel  *gtk.Label
	Spinner      *gtk.Spinner
	OnStart      func(phone, message string, done func(err error))
}

// CleanPhoneNumber removes all non-digit characters from the input string.
func CleanPhoneNumber(input string) string {
	var sb strings.Builder
	for _, r := range input {
		if unicode.IsDigit(r) {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func ShowNewConversationDialog(parent *gtk.Window, onStart func(phone, message string, done func(err error))) *NewConversationDialog {
	win := adw.NewWindow()
	win.SetTitle("New Conversation")
	if parent != nil {
		win.SetTransientFor(parent)
	}
	win.SetModal(true)
	win.SetDefaultSize(420, 240)
	win.SetResizable(false)

	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// HeaderBar
	headerBar := adw.NewHeaderBar()

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		win.Destroy()
	})
	headerBar.PackStart(cancelBtn)

	titleLabel := gtk.NewLabel("New Conversation")
	titleLabel.AddCSSClass("heading")
	headerBar.SetTitleWidget(titleLabel)

	startBtn := gtk.NewButtonWithLabel("Start Chat")
	startBtn.AddCSSClass("suggested-action")
	headerBar.PackEnd(startBtn)

	mainBox.Append(headerBar)

	// Content
	contentBox := gtk.NewBox(gtk.OrientationVertical, 12)
	contentBox.SetMarginStart(16)
	contentBox.SetMarginEnd(16)
	contentBox.SetMarginTop(12)
	contentBox.SetMarginBottom(16)

	prefGroup := adw.NewPreferencesGroup()
	prefGroup.SetDescription("Enter the recipient's phone number with country code (e.g. +55 11 99999-9999).")

	phoneRow := adw.NewEntryRow()
	phoneRow.SetTitle("Phone number")
	phoneRow.SetShowApplyButton(false)

	msgRow := adw.NewEntryRow()
	msgRow.SetTitle("Message (optional)")
	msgRow.SetShowApplyButton(false)

	prefGroup.Add(phoneRow)
	prefGroup.Add(msgRow)

	contentBox.Append(prefGroup)

	// Status & Spinner row
	statusBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	statusBox.SetHAlign(gtk.AlignCenter)

	spinner := gtk.NewSpinner()
	spinner.SetVisible(false)
	statusBox.Append(spinner)

	statusLabel := gtk.NewLabel("")
	statusLabel.SetWrap(true)
	statusLabel.AddCSSClass("dim-label")
	statusBox.Append(statusLabel)

	contentBox.Append(statusBox)
	mainBox.Append(contentBox)

	win.SetContent(mainBox)

	dlg := &NewConversationDialog{
		Window:       win,
		PhoneEntry:   phoneRow,
		MessageEntry: msgRow,
		StartButton:  startBtn,
		CancelButton: cancelBtn,
		StatusLabel:  statusLabel,
		Spinner:      spinner,
		OnStart:      onStart,
	}

	submit := func() {
		rawPhone := phoneRow.Text()
		cleanPhone := CleanPhoneNumber(rawPhone)

		if len(cleanPhone) < 8 {
			statusLabel.RemoveCSSClass("dim-label")
			statusLabel.AddCSSClass("error-label")
			statusLabel.SetText("Please enter a valid phone number with country code.")
			return
		}

		// Loading state
		spinner.SetVisible(true)
		spinner.Start()
		startBtn.SetSensitive(false)
		phoneRow.SetSensitive(false)
		msgRow.SetSensitive(false)
		cancelBtn.SetSensitive(false)

		statusLabel.RemoveCSSClass("error-label")
		statusLabel.AddCSSClass("dim-label")
		statusLabel.SetText("Checking number on WhatsApp...")

		msgText := msgRow.Text()

		if dlg.OnStart != nil {
			dlg.OnStart(cleanPhone, msgText, func(err error) {
				glib.IdleAdd(func() {
					if err != nil {
						spinner.Stop()
						spinner.SetVisible(false)
						startBtn.SetSensitive(true)
						phoneRow.SetSensitive(true)
						msgRow.SetSensitive(true)
						cancelBtn.SetSensitive(true)

						statusLabel.RemoveCSSClass("dim-label")
						statusLabel.AddCSSClass("error-label")
						statusLabel.SetText(err.Error())
						return
					}

					win.Destroy()
				})
			})
		}
	}

	startBtn.ConnectClicked(submit)
	phoneRow.ConnectEntryActivated(submit)
	msgRow.ConnectEntryActivated(submit)

	// Key controller for Escape key to close dialog
	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(keyval uint, keycode uint, state gdk.ModifierType) bool {
		if keyval == gdk.KEY_Escape {
			win.Destroy()
			return true
		}
		return false
	})
	win.AddController(keyCtrl)

	win.Present()

	glib.IdleAdd(func() {
		phoneRow.GrabFocus()
	})

	return dlg
}
