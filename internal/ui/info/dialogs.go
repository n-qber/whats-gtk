package info

import (
	"fmt"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type ContactItem struct {
	JID    string
	Name   string
	Avatar *gdk.Texture
}

// ShowAddParticipantsDialog displays a dialog to select multiple contacts to add to a group.
func ShowAddParticipantsDialog(parent *gtk.Window, contacts []ContactItem, onAdd func(targetJIDs []string)) {
	win := adw.NewWindow()
	win.SetTitle("Adicionar participantes")
	if parent != nil {
		win.SetTransientFor(parent)
	}
	win.SetModal(true)
	win.SetDefaultSize(420, 520)

	selected := make(map[string]bool)

	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)

	headerBar := adw.NewHeaderBar()
	cancelBtn := gtk.NewButtonWithLabel("Cancelar")
	cancelBtn.ConnectClicked(func() {
		win.Destroy()
	})
	headerBar.PackStart(cancelBtn)

	titleLabel := gtk.NewLabel("Adicionar participantes")
	titleLabel.AddCSSClass("heading")
	headerBar.SetTitleWidget(titleLabel)

	addBtn := gtk.NewButtonWithLabel("Adicionar")
	addBtn.AddCSSClass("suggested-action")
	addBtn.SetSensitive(false)
	headerBar.PackEnd(addBtn)

	mainBox.Append(headerBar)

	searchEntry := gtk.NewSearchEntry()
	searchEntry.SetMarginTop(8)
	searchEntry.SetMarginBottom(8)
	searchEntry.SetMarginStart(12)
	searchEntry.SetMarginEnd(12)
	searchEntry.SetPlaceholderText("Buscar contatos...")
	mainBox.Append(searchEntry)

	listBox := gtk.NewListBox()
	listBox.AddCSSClass("content")
	listBox.SetSelectionMode(gtk.SelectionNone)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetChild(listBox)
	mainBox.Append(scrolled)

	updateAddButton := func() {
		count := 0
		for _, v := range selected {
			if v {
				count++
			}
		}
		if count > 0 {
			addBtn.SetLabel("Adicionar (" + string(rune('0'+count)) + ")")
			addBtn.SetSensitive(true)
		} else {
			addBtn.SetLabel("Adicionar")
			addBtn.SetSensitive(false)
		}
	}

	populateList := func(filter string) {
		for listBox.FirstChild() != nil {
			listBox.Remove(listBox.FirstChild())
		}

		filter = strings.ToLower(filter)
		for _, c := range contacts {
			if filter != "" && !strings.Contains(strings.ToLower(c.Name), filter) && !strings.Contains(strings.ToLower(c.JID), filter) {
				continue
			}

			contact := c
			row := adw.NewActionRow()
			row.SetTitle(contact.Name)
			row.SetSubtitle(contact.JID)

			avatar := adw.NewAvatar(36, contact.Name, true)
			if contact.Avatar != nil {
				avatar.SetCustomImage(contact.Avatar)
			}
			row.AddPrefix(avatar)

			check := gtk.NewCheckButton()
			check.SetActive(selected[contact.JID])
			check.SetVAlign(gtk.AlignCenter)
			check.ConnectToggled(func() {
				if check.Active() {
					selected[contact.JID] = true
				} else {
					delete(selected, contact.JID)
				}
				updateAddButton()
			})
			row.AddSuffix(check)
			row.SetActivatableWidget(check)

			listBox.Append(row)
		}
	}

	searchEntry.ConnectSearchChanged(func() {
		populateList(searchEntry.Text())
	})

	addBtn.ConnectClicked(func() {
		var result []string
		for jid, isSel := range selected {
			if isSel {
				result = append(result, jid)
			}
		}
		win.Destroy()
		if len(result) > 0 && onAdd != nil {
			onAdd(result)
		}
	})

	populateList("")
	win.SetContent(mainBox)
	win.Present()
}

// ShowCreateCommunityDialog prompts for name and description to create a new community.
func ShowCreateCommunityDialog(parent *gtk.Window, onCreate func(name, desc string)) {
	win := adw.NewWindow()
	win.SetTitle("Nova Comunidade")
	if parent != nil {
		win.SetTransientFor(parent)
	}
	win.SetModal(true)
	win.SetDefaultSize(420, 280)
	win.SetResizable(false)

	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)

	headerBar := adw.NewHeaderBar()
	cancelBtn := gtk.NewButtonWithLabel("Cancelar")
	cancelBtn.ConnectClicked(func() {
		win.Destroy()
	})
	headerBar.PackStart(cancelBtn)

	titleLabel := gtk.NewLabel("Nova Comunidade")
	titleLabel.AddCSSClass("heading")
	headerBar.SetTitleWidget(titleLabel)

	createBtn := gtk.NewButtonWithLabel("Criar")
	createBtn.AddCSSClass("suggested-action")
	createBtn.SetSensitive(false)
	headerBar.PackEnd(createBtn)

	mainBox.Append(headerBar)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 12)
	contentBox.SetMarginStart(16)
	contentBox.SetMarginEnd(16)
	contentBox.SetMarginTop(16)
	contentBox.SetMarginBottom(16)

	prefGroup := adw.NewPreferencesGroup()
	prefGroup.SetDescription("Crie uma comunidade para organizar grupos relacionados e enviar avisos.")

	nameRow := adw.NewEntryRow()
	nameRow.SetTitle("Nome da comunidade")
	nameRow.SetShowApplyButton(false)

	descRow := adw.NewEntryRow()
	descRow.SetTitle("Descrição (opcional)")
	descRow.SetShowApplyButton(false)

	prefGroup.Add(nameRow)
	prefGroup.Add(descRow)
	contentBox.Append(prefGroup)

	nameRow.ConnectChanged(func() {
		t := strings.TrimSpace(nameRow.Text())
		createBtn.SetSensitive(t != "")
	})

	createBtn.ConnectClicked(func() {
		name := strings.TrimSpace(nameRow.Text())
		desc := strings.TrimSpace(descRow.Text())
		if name == "" {
			return
		}
		win.Destroy()
		if onCreate != nil {
			onCreate(name, desc)
		}
	})

	mainBox.Append(contentBox)
	win.SetContent(mainBox)
	win.Present()
}

// ShowCreateSubGroupDialog prompts for a name to create a new sub-group in a community.
func ShowCreateSubGroupDialog(parent *gtk.Window, communityName string, onCreate func(name string)) {
	win := adw.NewWindow()
	win.SetTitle("Novo Grupo na Comunidade")
	if parent != nil {
		win.SetTransientFor(parent)
	}
	win.SetModal(true)
	win.SetDefaultSize(400, 220)
	win.SetResizable(false)

	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)

	headerBar := adw.NewHeaderBar()
	cancelBtn := gtk.NewButtonWithLabel("Cancelar")
	cancelBtn.ConnectClicked(func() {
		win.Destroy()
	})
	headerBar.PackStart(cancelBtn)

	titleLabel := gtk.NewLabel("Novo Grupo")
	titleLabel.AddCSSClass("heading")
	headerBar.SetTitleWidget(titleLabel)

	createBtn := gtk.NewButtonWithLabel("Criar")
	createBtn.AddCSSClass("suggested-action")
	createBtn.SetSensitive(false)
	headerBar.PackEnd(createBtn)

	mainBox.Append(headerBar)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 12)
	contentBox.SetMarginStart(16)
	contentBox.SetMarginEnd(16)
	contentBox.SetMarginTop(16)
	contentBox.SetMarginBottom(16)

	prefGroup := adw.NewPreferencesGroup()
	if communityName != "" {
		prefGroup.SetDescription("Este grupo será adicionado à comunidade: " + communityName)
	}

	nameRow := adw.NewEntryRow()
	nameRow.SetTitle("Nome do grupo")
	nameRow.SetShowApplyButton(false)

	prefGroup.Add(nameRow)
	contentBox.Append(prefGroup)

	nameRow.ConnectChanged(func() {
		t := strings.TrimSpace(nameRow.Text())
		createBtn.SetSensitive(t != "")
	})

	createBtn.ConnectClicked(func() {
		name := strings.TrimSpace(nameRow.Text())
		if name == "" {
			return
		}
		win.Destroy()
		if onCreate != nil {
			onCreate(name)
		}
	})

	mainBox.Append(contentBox)
	win.SetContent(mainBox)
	win.Present()
}

// ShowLinkGroupDialog allows selecting an existing group to link to a community.
func ShowLinkGroupDialog(parent *gtk.Window, groups []ContactItem, onLink func(targetJID string)) {
	win := adw.NewWindow()
	win.SetTitle("Vincular grupo à comunidade")
	if parent != nil {
		win.SetTransientFor(parent)
	}
	win.SetModal(true)
	win.SetDefaultSize(420, 500)

	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)

	headerBar := adw.NewHeaderBar()
	cancelBtn := gtk.NewButtonWithLabel("Cancelar")
	cancelBtn.ConnectClicked(func() {
		win.Destroy()
	})
	headerBar.PackStart(cancelBtn)

	titleLabel := gtk.NewLabel("Vincular Grupo")
	titleLabel.AddCSSClass("heading")
	headerBar.SetTitleWidget(titleLabel)

	mainBox.Append(headerBar)

	searchEntry := gtk.NewSearchEntry()
	searchEntry.SetMarginTop(8)
	searchEntry.SetMarginBottom(8)
	searchEntry.SetMarginStart(12)
	searchEntry.SetMarginEnd(12)
	searchEntry.SetPlaceholderText("Buscar seus grupos...")
	mainBox.Append(searchEntry)

	listBox := gtk.NewListBox()
	listBox.AddCSSClass("content")

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetChild(listBox)
	mainBox.Append(scrolled)

	populateList := func(filter string) {
		for listBox.FirstChild() != nil {
			listBox.Remove(listBox.FirstChild())
		}

		filter = strings.ToLower(filter)
		for _, g := range groups {
			if filter != "" && !strings.Contains(strings.ToLower(g.Name), filter) {
				continue
			}

			grp := g
			row := adw.NewActionRow()
			row.SetTitle(grp.Name)
			row.SetSubtitle(grp.JID)

			avatar := adw.NewAvatar(36, grp.Name, true)
			if grp.Avatar != nil {
				avatar.SetCustomImage(grp.Avatar)
			}
			row.AddPrefix(avatar)

			linkBtn := gtk.NewButtonWithLabel("Vincular")
			linkBtn.AddCSSClass("suggested-action")
			linkBtn.SetVAlign(gtk.AlignCenter)
			linkBtn.ConnectClicked(func() {
				win.Destroy()
				if onLink != nil {
					onLink(grp.JID)
				}
			})
			row.AddSuffix(linkBtn)

			listBox.Append(row)
		}
	}

	searchEntry.ConnectSearchChanged(func() {
		populateList(searchEntry.Text())
	})

	populateList("")
	win.SetContent(mainBox)
	win.Present()
}

// ShowEditTextDialog opens an entry dialog to edit a single text field (e.g. name).
func ShowEditTextDialog(parent *gtk.Window, title, placeholder, initialValue string, onSave func(value string)) {
	win := adw.NewWindow()
	win.SetTitle(title)
	if parent != nil {
		win.SetTransientFor(parent)
	}
	win.SetModal(true)
	win.SetDefaultSize(380, 180)
	win.SetResizable(false)

	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)

	headerBar := adw.NewHeaderBar()
	cancelBtn := gtk.NewButtonWithLabel("Cancelar")
	cancelBtn.ConnectClicked(func() {
		win.Destroy()
	})
	headerBar.PackStart(cancelBtn)

	titleLabel := gtk.NewLabel(title)
	titleLabel.AddCSSClass("heading")
	headerBar.SetTitleWidget(titleLabel)

	saveBtn := gtk.NewButtonWithLabel("Salvar")
	saveBtn.AddCSSClass("suggested-action")
	headerBar.PackEnd(saveBtn)

	mainBox.Append(headerBar)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 12)
	contentBox.SetMarginStart(16)
	contentBox.SetMarginEnd(16)
	contentBox.SetMarginTop(16)
	contentBox.SetMarginBottom(16)

	entryRow := adw.NewEntryRow()
	entryRow.SetTitle(placeholder)
	entryRow.SetText(initialValue)
	entryRow.SetShowApplyButton(false)

	prefGroup := adw.NewPreferencesGroup()
	prefGroup.Add(entryRow)
	contentBox.Append(prefGroup)

	saveBtn.ConnectClicked(func() {
		val := strings.TrimSpace(entryRow.Text())
		win.Destroy()
		if onSave != nil {
			onSave(val)
		}
	})

	mainBox.Append(contentBox)
	win.SetContent(mainBox)
	win.Present()
}

// ShowEditTextAreaDialog opens a dialog with a multi-line text area (textarea) to edit longer texts, such as group descriptions.
func ShowEditTextAreaDialog(parent *gtk.Window, title, placeholder, initialValue string, onSave func(value string)) {
	win := adw.NewWindow()
	win.SetTitle(title)
	if parent != nil {
		win.SetTransientFor(parent)
	}
	win.SetModal(true)
	win.SetDefaultSize(480, 320)
	win.SetResizable(true)

	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)

	headerBar := adw.NewHeaderBar()
	cancelBtn := gtk.NewButtonWithLabel("Cancelar")
	cancelBtn.ConnectClicked(func() {
		win.Destroy()
	})
	headerBar.PackStart(cancelBtn)

	titleLabel := gtk.NewLabel(title)
	titleLabel.AddCSSClass("heading")
	headerBar.SetTitleWidget(titleLabel)

	saveBtn := gtk.NewButtonWithLabel("Salvar")
	saveBtn.AddCSSClass("suggested-action")
	headerBar.PackEnd(saveBtn)

	mainBox.Append(headerBar)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 8)
	contentBox.SetMarginStart(16)
	contentBox.SetMarginEnd(16)
	contentBox.SetMarginTop(16)
	contentBox.SetMarginBottom(16)
	contentBox.SetVExpand(true)

	if placeholder != "" {
		descLabel := gtk.NewLabel(placeholder)
		descLabel.SetHAlign(gtk.AlignStart)
		descLabel.AddCSSClass("dim-label")
		contentBox.Append(descLabel)
	}

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scrolled.SetVExpand(true)
	scrolled.SetHExpand(true)
	scrolled.SetMinContentHeight(160)
	scrolled.AddCSSClass("card")
	scrolled.AddCSSClass("description-editor-scroll")

	textView := gtk.NewTextView()
	textView.SetWrapMode(gtk.WrapWordChar)
	textView.SetAcceptsTab(false)
	textView.SetLeftMargin(12)
	textView.SetRightMargin(12)
	textView.SetTopMargin(10)
	textView.SetBottomMargin(10)
	textView.AddCSSClass("description-editor-view")

	buffer := textView.Buffer()
	buffer.SetEnableUndo(true)
	if initialValue != "" {
		buffer.SetText(initialValue)
		end := buffer.EndIter()
		buffer.PlaceCursor(end)
	}

	scrolled.SetChild(textView)
	contentBox.Append(scrolled)

	bottomBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	hintLabel := gtk.NewLabel("Pressione Ctrl+Enter para salvar")
	hintLabel.AddCSSClass("dim-label")
	hintLabel.AddCSSClass("caption")
	bottomBox.Append(hintLabel)

	spacer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	spacer.SetHExpand(true)
	bottomBox.Append(spacer)

	charCountLabel := gtk.NewLabel("")
	charCountLabel.AddCSSClass("dim-label")
	charCountLabel.AddCSSClass("caption")
	updateCount := func() {
		cnt := buffer.CharCount()
		if cnt == 1 {
			charCountLabel.SetLabel("1 caractere")
		} else {
			charCountLabel.SetLabel(fmt.Sprintf("%d caracteres", cnt))
		}
	}
	updateCount()
	buffer.ConnectChanged(updateCount)
	bottomBox.Append(charCountLabel)

	contentBox.Append(bottomBox)

	doSave := func() {
		start, end := buffer.Bounds()
		val := strings.TrimSpace(buffer.Text(start, end, false))
		win.Destroy()
		if onSave != nil {
			onSave(val)
		}
	}
	saveBtn.ConnectClicked(doSave)

	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.SetPropagationPhase(gtk.PhaseCapture)
	keyCtrl.ConnectKeyPressed(func(keyval uint, keycode uint, state gdk.ModifierType) bool {
		if keyval == gdk.KEY_Escape {
			win.Destroy()
			return true
		}
		if (keyval == gdk.KEY_Return || keyval == gdk.KEY_KP_Enter) && (state&gdk.ControlMask != 0) {
			doSave()
			return true
		}
		return false
	})
	win.AddController(keyCtrl)

	mainBox.Append(contentBox)
	win.SetContent(mainBox)
	win.Present()

	glib.IdleAdd(func() {
		textView.GrabFocus()
	})
}
