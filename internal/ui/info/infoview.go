package info

import (
	"fmt"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type ParticipantModel struct {
	Name    string
	JID     string
	IsAdmin bool
	Avatar  *gdk.Texture
}

type InfoView struct {
	Box           *gtk.Box
	Scrolled      *gtk.ScrolledWindow
	Avatar        *adw.Avatar
	NameLabel     *gtk.Label
	JIDLabel      *gtk.Label

	GroupDescGroup *adw.PreferencesGroup
	GroupDescRow   *adw.ActionRow

	ParticipantsGroup *adw.PreferencesGroup
	ParticipantsList  *gtk.ListBox

	OptionsGroup *adw.PreferencesGroup
	ArchiveRow   *adw.ActionRow
	ArchiveSwitch *gtk.Switch
	ExitRow      *adw.ActionRow

	OnArchiveToggled func(archived bool)
	isArchiving      bool
}

func NewInfoView() *InfoView {
	iv := &InfoView{}

	mainBox := gtk.NewBox(gtk.OrientationVertical, 16)
	mainBox.SetMarginTop(24)
	mainBox.SetMarginBottom(24)
	mainBox.SetMarginStart(24)
	mainBox.SetMarginEnd(24)

	iv.Avatar = adw.NewAvatar(120, "", true)
	iv.Avatar.SetHAlign(gtk.AlignCenter)
	mainBox.Append(iv.Avatar)

	iv.NameLabel = gtk.NewLabel("")
	iv.NameLabel.SetWrap(true)
	iv.NameLabel.SetJustify(gtk.JustifyCenter)
	iv.NameLabel.AddCSSClass("title-2")
	mainBox.Append(iv.NameLabel)

	iv.JIDLabel = gtk.NewLabel("")
	iv.JIDLabel.SetWrap(true)
	iv.JIDLabel.SetJustify(gtk.JustifyCenter)
	iv.JIDLabel.AddCSSClass("dim-label")
	mainBox.Append(iv.JIDLabel)

	// Description
	iv.GroupDescGroup = adw.NewPreferencesGroup()
	iv.GroupDescGroup.SetTitle("Description")
	iv.GroupDescRow = adw.NewActionRow()
	iv.GroupDescRow.SetSubtitleLines(0) // Wrap to multiple lines
	iv.GroupDescGroup.Add(iv.GroupDescRow)
	mainBox.Append(iv.GroupDescGroup)

	// Participants
	iv.ParticipantsGroup = adw.NewPreferencesGroup()
	iv.ParticipantsGroup.SetTitle("Participants")
	iv.ParticipantsList = gtk.NewListBox()
	iv.ParticipantsList.AddCSSClass("content")
	iv.ParticipantsList.SetSelectionMode(gtk.SelectionNone)
	iv.ParticipantsGroup.Add(iv.ParticipantsList)
	mainBox.Append(iv.ParticipantsGroup)

	// Options
	iv.OptionsGroup = adw.NewPreferencesGroup()
	iv.OptionsGroup.SetTitle("Options")
	
	muteRow := adw.NewActionRow()
	muteRow.SetTitle("Mute Notifications")
	muteSwitch := gtk.NewSwitch()
	muteSwitch.SetVAlign(gtk.AlignCenter)
	muteRow.AddSuffix(muteSwitch)
	iv.OptionsGroup.Add(muteRow)

	iv.ArchiveRow = adw.NewActionRow()
	iv.ArchiveRow.SetTitle("Archive Chat")
	iv.ArchiveSwitch = gtk.NewSwitch()
	iv.ArchiveSwitch.SetVAlign(gtk.AlignCenter)
	iv.ArchiveRow.AddSuffix(iv.ArchiveSwitch)
	iv.OptionsGroup.Add(iv.ArchiveRow)

	iv.ArchiveSwitch.ConnectStateSet(func(state bool) bool {
		if iv.isArchiving {
			return false
		}
		if iv.OnArchiveToggled != nil {
			iv.OnArchiveToggled(state)
		}
		return false
	})
	
	iv.ExitRow = adw.NewActionRow()
	iv.ExitRow.SetTitle("Exit Group")
	iv.ExitRow.SetTitleLines(1)
	iv.OptionsGroup.Add(iv.ExitRow)

	mainBox.Append(iv.OptionsGroup)

	iv.Scrolled = gtk.NewScrolledWindow()
	iv.Scrolled.SetChild(mainBox)
	iv.Scrolled.SetPropagateNaturalWidth(true)
	iv.Scrolled.SetPropagateNaturalHeight(true)
	iv.Scrolled.SetSizeRequest(320, -1)
	
	iv.Box = gtk.NewBox(gtk.OrientationVertical, 0)
	iv.Box.SetVExpand(true)
	iv.Box.Append(iv.Scrolled)
	iv.Box.AddCSSClass("info-view-box")

	return iv
}

func (iv *InfoView) SetInfo(name string, jid string, tex *gdk.Texture) {
	iv.NameLabel.SetText(name)
	iv.JIDLabel.SetText(jid)
	
	if tex != nil {
		iv.Avatar.SetCustomImage(tex)
	} else {
		iv.Avatar.SetCustomImage(nil)
		iv.Avatar.SetText(name)
	}
	
	iv.GroupDescGroup.Hide()
	iv.ParticipantsGroup.Hide()
	iv.ExitRow.Hide()
	iv.OptionsGroup.Show()
}

func (iv *InfoView) SetArchived(archived bool) {
	iv.isArchiving = true
	iv.ArchiveSwitch.SetActive(archived)
	if archived {
		iv.ArchiveRow.SetTitle("Archived Chat")
	} else {
		iv.ArchiveRow.SetTitle("Archive Chat")
	}
	iv.isArchiving = false
}

func (iv *InfoView) SetGroupDetails(desc string, participants []ParticipantModel) {
	if desc != "" {
		iv.GroupDescRow.SetSubtitle(desc)
		iv.GroupDescGroup.Show()
	} else {
		iv.GroupDescGroup.Hide()
	}

	for iv.ParticipantsList.FirstChild() != nil {
		iv.ParticipantsList.Remove(iv.ParticipantsList.FirstChild())
	}

	for _, p := range participants {
		row := adw.NewActionRow()
		row.SetTitle(p.Name)
		row.SetSubtitle(p.JID)
		
		avatar := adw.NewAvatar(40, p.Name, true)
		if p.Avatar != nil {
			avatar.SetCustomImage(p.Avatar)
		}
		row.AddPrefix(avatar)

		if p.IsAdmin {
			adminLabel := gtk.NewLabel("Admin")
			adminLabel.AddCSSClass("dim-label")
			adminLabel.SetVAlign(gtk.AlignCenter)
			row.AddSuffix(adminLabel)
		}

		iv.ParticipantsList.Append(row)
	}

	iv.ParticipantsGroup.SetTitle(fmt.Sprintf("Participants (%d)", len(participants)))
	iv.ParticipantsGroup.Show()
	iv.OptionsGroup.Show()
}
