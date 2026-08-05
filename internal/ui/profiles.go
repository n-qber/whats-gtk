package ui

import (
	"fmt"
	"strings"
	"whats-gtk/internal/database"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
)

type ProfileManagerDialog struct {
	Window         *adw.Window
	ListBox        *gtk.ListBox
	db             *database.AppDB
	parentWindow   *gtk.Window
	onChanged      func()
	onSelectActive func(id int64)
}

func ShowProfileManagerDialog(parent *gtk.Window, db *database.AppDB, onChanged func(), onSelectActive func(id int64)) {
	win := adw.NewWindow()
	win.SetTitle("Manage Profiles")
	win.SetTransientFor(parent)
	win.SetModal(true)
	win.SetDefaultSize(480, 560)

	box := gtk.NewBox(gtk.OrientationVertical, 0)

	headerBar := adw.NewHeaderBar()
	addBtn := gtk.NewButtonFromIconName("list-add-symbolic")
	addBtn.SetTooltipText("Create New Profile")
	headerBar.PackEnd(addBtn)

	box.Append(headerBar)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetMarginStart(12)
	scrolled.SetMarginEnd(12)
	scrolled.SetMarginTop(12)
	scrolled.SetMarginBottom(12)

	listBox := gtk.NewListBox()
	listBox.AddCSSClass("boxed-list")
	scrolled.SetChild(listBox)

	box.Append(scrolled)

	win.SetContent(box)

	dlg := &ProfileManagerDialog{
		Window:         win,
		ListBox:        listBox,
		db:             db,
		parentWindow:   parent,
		onChanged:      onChanged,
		onSelectActive: onSelectActive,
	}

	addBtn.ConnectClicked(func() {
		dlg.showCreateProfileDialog()
	})

	dlg.Refresh()
	win.Present()
}

func (d *ProfileManagerDialog) Refresh() {
	// Clear listbox
	for {
		child := d.ListBox.FirstChild()
		if child == nil {
			break
		}
		d.ListBox.Remove(child)
	}

	activeID, _ := d.db.GetActiveProfileID()
	profiles, err := d.db.GetProfiles()
	if err != nil {
		fmt.Printf("ProfileManagerDialog: failed to get profiles: %v\n", err)
		return
	}

	// 1. System default "All Chats"
	defaultRow := adw.NewActionRow()
	defaultRow.SetTitle("All Chats")
	defaultRow.SetSubtitle("Default profile (shows all conversations)")

	if activeID == 0 {
		activeBadge := gtk.NewLabel("Active")
		activeBadge.AddCSSClass("accent")
		activeBadge.SetMarginEnd(8)
		defaultRow.AddSuffix(activeBadge)
	} else {
		selectBtn := gtk.NewButtonWithLabel("Select")
		selectBtn.ConnectClicked(func() {
			d.onSelectActive(0)
			d.Refresh()
		})
		defaultRow.AddSuffix(selectBtn)
	}
	d.ListBox.Append(defaultRow)

	// 2. User created profiles
	for _, p := range profiles {
		prof := p // capture loop variable
		row := adw.NewActionRow()
		row.SetTitle(glib.MarkupEscapeText(prof.Name))
		row.SetSubtitle(fmt.Sprintf("%d conversations", prof.ContactCount))

		if prof.ID == activeID {
			activeBadge := gtk.NewLabel("Active")
			activeBadge.AddCSSClass("accent")
			activeBadge.SetMarginEnd(8)
			row.AddSuffix(activeBadge)
		} else {
			selectBtn := gtk.NewButtonWithLabel("Select")
			selectBtn.ConnectClicked(func() {
				d.onSelectActive(prof.ID)
				d.Refresh()
			})
			row.AddSuffix(selectBtn)
		}

		editBtn := gtk.NewButtonFromIconName("document-edit-symbolic")
		editBtn.SetTooltipText("Edit Profile Members")
		editBtn.ConnectClicked(func() {
			ShowEditProfileDialog(&d.Window.Window, d.db, prof, func() {
				d.Refresh()
				if d.onChanged != nil {
					d.onChanged()
				}
			})
		})
		row.AddSuffix(editBtn)

		deleteBtn := gtk.NewButtonFromIconName("user-trash-symbolic")
		deleteBtn.SetTooltipText("Delete Profile")
		deleteBtn.AddCSSClass("destructive-action")
		deleteBtn.ConnectClicked(func() {
			_ = d.db.DeleteProfile(prof.ID)
			d.Refresh()
			if d.onChanged != nil {
				d.onChanged()
			}
		})
		row.AddSuffix(deleteBtn)

		d.ListBox.Append(row)
	}
}

func (d *ProfileManagerDialog) showCreateProfileDialog() {
	win := adw.NewWindow()
	win.SetTitle("New Profile")
	win.SetTransientFor(&d.Window.Window)
	win.SetModal(true)
	win.SetResizable(false)
	win.SetDefaultSize(360, 180)

	box := gtk.NewBox(gtk.OrientationVertical, 12)
	box.SetMarginTop(16)
	box.SetMarginBottom(16)
	box.SetMarginStart(16)
	box.SetMarginEnd(16)

	headerBar := adw.NewHeaderBar()
	box.Append(headerBar)

	label := gtk.NewLabel("Enter a name for the new profile:")
	label.SetHAlign(gtk.AlignStart)
	box.Append(label)

	entry := gtk.NewEntry()
	entry.SetPlaceholderText("e.g. Bateria UFSCar")
	box.Append(entry)

	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	btnBox.SetHAlign(gtk.AlignEnd)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		win.Destroy()
	})
	btnBox.Append(cancelBtn)

	createBtn := gtk.NewButtonWithLabel("Create")
	createBtn.AddCSSClass("suggested-action")

	doCreate := func() {
		name := strings.TrimSpace(entry.Text())
		if name == "" {
			return
		}
		prof, err := d.db.CreateProfile(name)
		win.Destroy()
		if err == nil && prof != nil {
			d.Refresh()
			if d.onChanged != nil {
				d.onChanged()
			}
			// Automatically open edit dialog to select contacts
			ShowEditProfileDialog(&d.Window.Window, d.db, *prof, func() {
				d.Refresh()
				if d.onChanged != nil {
					d.onChanged()
				}
			})
		}
	}

	createBtn.ConnectClicked(doCreate)
	entry.ConnectActivate(doCreate)

	btnBox.Append(createBtn)
	box.Append(btnBox)

	win.SetContent(box)
	win.Present()
}

type EditProfileDialog struct {
	Window       *adw.Window
	ListBox      *gtk.ListBox
	db           *database.AppDB
	profile      database.Profile
	onClose      func()
	jidMap       map[string]bool
	allContacts  []database.Contact
	filterTerm   string
}

func ShowEditProfileDialog(parent *gtk.Window, db *database.AppDB, profile database.Profile, onClose func()) {
	win := adw.NewWindow()
	win.SetTitle(fmt.Sprintf("Edit Profile: %s", profile.Name))
	win.SetTransientFor(parent)
	win.SetModal(true)
	win.SetDefaultSize(520, 640)

	box := gtk.NewBox(gtk.OrientationVertical, 0)

	headerBar := adw.NewHeaderBar()
	box.Append(headerBar)

	// Top controls: Name entry & Search
	topBox := gtk.NewBox(gtk.OrientationVertical, 8)
	topBox.SetMarginStart(12)
	topBox.SetMarginEnd(12)
	topBox.SetMarginTop(12)
	topBox.SetMarginBottom(8)

	nameBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	nameLabel := gtk.NewLabel("Name:")
	nameBox.Append(nameLabel)

	nameEntry := gtk.NewEntry()
	nameEntry.SetText(profile.Name)
	nameEntry.SetHExpand(true)
	nameBox.Append(nameEntry)

	saveNameBtn := gtk.NewButtonWithLabel("Save")
	saveNameBtn.ConnectClicked(func() {
		newName := strings.TrimSpace(nameEntry.Text())
		if newName != "" && newName != profile.Name {
			_ = db.UpdateProfileName(profile.ID, newName)
			profile.Name = newName
			win.SetTitle(fmt.Sprintf("Edit Profile: %s", profile.Name))
			if onClose != nil {
				onClose()
			}
		}
	})
	nameBox.Append(saveNameBtn)

	topBox.Append(nameBox)

	searchEntry := gtk.NewSearchEntry()
	searchEntry.SetPlaceholderText("Search contacts and groups...")
	topBox.Append(searchEntry)

	box.Append(topBox)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetMarginStart(12)
	scrolled.SetMarginEnd(12)
	scrolled.SetMarginBottom(12)

	listBox := gtk.NewListBox()
	listBox.AddCSSClass("boxed-list")
	scrolled.SetChild(listBox)

	box.Append(scrolled)

	win.SetContent(box)

	jidMap, _ := db.GetProfileJIDMap(profile.ID)
	allContacts, _ := db.GetAllContacts(0, 1000)

	dlg := &EditProfileDialog{
		Window:      win,
		ListBox:     listBox,
		db:          db,
		profile:     profile,
		onClose:     onClose,
		jidMap:      jidMap,
		allContacts: allContacts,
	}

	searchEntry.ConnectSearchChanged(func() {
		dlg.filterTerm = strings.ToLower(strings.TrimSpace(searchEntry.Text()))
		dlg.renderRows()
	})

	dlg.renderRows()

	win.Connect("close-request", func() bool {
		if onClose != nil {
			onClose()
		}
		return false
	})

	win.Present()
}

func (e *EditProfileDialog) renderRows() {
	// Clear listbox
	for {
		child := e.ListBox.FirstChild()
		if child == nil {
			break
		}
		e.ListBox.Remove(child)
	}

	for _, c := range e.allContacts {
		contact := c // capture
		dispName := contact.DisplayName()

		if e.filterTerm != "" {
			nameLower := strings.ToLower(dispName)
			jidLower := strings.ToLower(contact.JID)
			if !strings.Contains(nameLower, e.filterTerm) && !strings.Contains(jidLower, e.filterTerm) {
				continue
			}
		}

		row := adw.NewActionRow()

		title := dispName
		if contact.IsGroup.Valid && contact.IsGroup.Bool {
			title = "[G] " + title
		}
		row.SetTitle(glib.MarkupEscapeText(title))
		row.SetSubtitle(glib.MarkupEscapeText(contact.JID))

		avatar := adw.NewAvatar(32, dispName, true)
		row.AddPrefix(avatar)

		check := gtk.NewCheckButton()
		inProfile := e.jidMap[contact.JID] || (contact.LID.Valid && e.jidMap[contact.LID.String])
		check.SetActive(inProfile)

		check.ConnectToggled(func() {
			active := check.Active()
			if active {
				e.jidMap[contact.JID] = true
				_ = e.db.AddContactToProfile(e.profile.ID, contact.JID)
			} else {
				delete(e.jidMap, contact.JID)
				_ = e.db.RemoveContactFromProfile(e.profile.ID, contact.JID)
				if contact.LID.Valid {
					delete(e.jidMap, contact.LID.String)
					_ = e.db.RemoveContactFromProfile(e.profile.ID, contact.LID.String)
				}
			}
		})

		row.AddSuffix(check)
		row.SetActivatableWidget(check)

		e.ListBox.Append(row)
	}
}
