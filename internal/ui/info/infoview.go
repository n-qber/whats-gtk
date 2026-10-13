package info

import (
	"fmt"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"whats-gtk/internal/ui/chat"
)

type ParticipantModel struct {
	Name    string
	JID     string
	IsAdmin bool
	IsOwner bool
	Avatar  *gdk.Texture
}

type CommunitySubGroupModel struct {
	JID          string
	Name         string
	IsDefaultSub bool
}

type ExtendedGroupDetails struct {
	JID             string
	Description     string
	IsCommunity     bool
	IsAdmin         bool
	IsOwner         bool
	IsAnnounce      bool
	IsLocked        bool
	ParentCommunity string
	ParentName      string
	MyJID           string
	SubGroups       []CommunitySubGroupModel
	Participants    []ParticipantModel
	InviteLink      string
}

type InfoView struct {
	Box           *gtk.Box
	HeaderBar     *adw.HeaderBar
	TitleWidget   *adw.WindowTitle
	BackBtn       *gtk.Button
	Scrolled      *gtk.ScrolledWindow
	Avatar        *adw.Avatar
	NameLabel     *gtk.Label
	EditNameBtn   *gtk.Button
	JIDLabel      *gtk.Label

	// Linked Community
	ParentCommunityGroup *adw.PreferencesGroup
	ParentCommunityRow   *adw.ActionRow

	// Description
	GroupDescGroup *adw.PreferencesGroup
	GroupDescRow   *adw.ActionRow
	EditDescBtn    *gtk.Button

	// Community Sub-groups
	CommunityGroupsGroup *adw.PreferencesGroup
	CommunityGroupsList  *gtk.ListBox
	AddExistingGroupBtn  *gtk.Button
	CreateSubGroupBtn    *gtk.Button

	// Group Permissions & Settings
	GroupSettingsGroup *adw.PreferencesGroup
	AnnounceRow        *adw.ActionRow
	AnnounceSwitch     *gtk.Switch
	LockedRow          *adw.ActionRow
	LockedSwitch       *gtk.Switch
	InviteLinkRow      *adw.ActionRow
	CopyInviteLinkBtn  *gtk.Button

	// Participants
	ParticipantsGroup *adw.PreferencesGroup
	ParticipantsList  *gtk.ListBox
	AddParticipantBtn *gtk.Button

	// Options
	OptionsGroup  *adw.PreferencesGroup
	MuteRow       *adw.ActionRow
	MuteSwitch    *gtk.Switch
	ArchiveRow    *adw.ActionRow
	ArchiveSwitch *gtk.Switch
	ExitRow       *adw.ActionRow

	// Callbacks
	OnClose                func()
	OnMuteToggled          func(muted bool)
	OnArchiveToggled       func(archived bool)
	OnExitGroup            func()
	OnAddParticipants      func()
	OnRemoveParticipant    func(participantJID string)
	OnPromoteParticipant   func(participantJID string)
	OnDemoteParticipant    func(participantJID string)
	OnMessageParticipant   func(participantJID string)
	OnEditGroupName        func(newName string)
	OnEditGroupDescription func(newDesc string)
	OnToggleAnnounce       func(announce bool)
	OnToggleLocked         func(locked bool)
	OnGetInviteLink        func()
	OnSelectSubGroup       func(groupJID string)
	OnUnlinkSubGroup       func(groupJID string)
	OnLinkExistingGroup    func()
	OnCreateSubGroup       func()
	OnOpenParentCommunity  func(parentJID string)

	currentJID         string
	currentParentJID   string
	currentDescription string
	isMuting           bool
	isArchiving        bool
	isUpdatingSettings bool
}

func NewInfoView() *InfoView {
	iv := &InfoView{}

	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(true)
	headerBar.SetShowStartTitleButtons(false)
	headerBar.SetDecorationLayout(chat.WindowDecorationLayout())

	backBtn := gtk.NewButtonFromIconName("go-previous-symbolic")
	backBtn.SetTooltipText("Voltar (Esc)")
	headerBar.PackStart(backBtn)

	titleWidget := adw.NewWindowTitle("Dados do contato", "")
	headerBar.SetTitleWidget(titleWidget)

	iv.HeaderBar = headerBar
	iv.TitleWidget = titleWidget
	iv.BackBtn = backBtn

	backBtn.ConnectClicked(func() {
		if iv.OnClose != nil {
			iv.OnClose()
		}
	})

	mainBox := gtk.NewBox(gtk.OrientationVertical, 16)
	mainBox.SetMarginTop(18)
	mainBox.SetMarginBottom(24)
	mainBox.SetMarginStart(18)
	mainBox.SetMarginEnd(18)

	// Top Avatar & Name
	iv.Avatar = adw.NewAvatar(120, "", true)
	iv.Avatar.SetHAlign(gtk.AlignCenter)
	mainBox.Append(iv.Avatar)

	nameBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	nameBox.SetHAlign(gtk.AlignCenter)

	iv.NameLabel = gtk.NewLabel("")
	iv.NameLabel.SetWrap(true)
	iv.NameLabel.SetJustify(gtk.JustifyCenter)
	iv.NameLabel.AddCSSClass("title-2")
	nameBox.Append(iv.NameLabel)

	iv.EditNameBtn = gtk.NewButtonFromIconName("document-edit-symbolic")
	iv.EditNameBtn.AddCSSClass("flat")
	iv.EditNameBtn.SetTooltipText("Editar nome")
	iv.EditNameBtn.SetVisible(false)
	nameBox.Append(iv.EditNameBtn)
	mainBox.Append(nameBox)

	iv.EditNameBtn.ConnectClicked(func() {
		win := iv.getWindow()
		ShowEditTextDialog(win, "Editar nome", "Nome", iv.NameLabel.Text(), func(val string) {
			if iv.OnEditGroupName != nil && val != "" {
				iv.OnEditGroupName(val)
			}
		})
	})

	iv.JIDLabel = gtk.NewLabel("")
	iv.JIDLabel.SetWrap(true)
	iv.JIDLabel.SetJustify(gtk.JustifyCenter)
	iv.JIDLabel.AddCSSClass("dim-label")
	mainBox.Append(iv.JIDLabel)

	// 1. Linked Community (when viewing a sub-group)
	iv.ParentCommunityGroup = adw.NewPreferencesGroup()
	iv.ParentCommunityGroup.SetTitle("Comunidade")
	iv.ParentCommunityRow = adw.NewActionRow()
	iv.ParentCommunityRow.SetTitle("Ver comunidade vinculada")
	iv.ParentCommunityRow.AddPrefix(gtk.NewImageFromIconName("system-users-symbolic"))
	openCommBtn := gtk.NewButtonFromIconName("go-next-symbolic")
	openCommBtn.AddCSSClass("flat")
	openCommBtn.SetVAlign(gtk.AlignCenter)
	iv.ParentCommunityRow.AddSuffix(openCommBtn)
	iv.ParentCommunityRow.SetActivatable(true)
	iv.ParentCommunityRow.ConnectActivated(func() {
		if iv.OnOpenParentCommunity != nil && iv.currentParentJID != "" {
			iv.OnOpenParentCommunity(iv.currentParentJID)
		}
	})
	iv.ParentCommunityGroup.Add(iv.ParentCommunityRow)
	iv.ParentCommunityGroup.Hide()
	mainBox.Append(iv.ParentCommunityGroup)

	// 2. Description
	iv.GroupDescGroup = adw.NewPreferencesGroup()
	iv.GroupDescGroup.SetTitle("Descrição")
	iv.GroupDescRow = adw.NewActionRow()
	iv.GroupDescRow.SetSubtitleLines(0)

	iv.EditDescBtn = gtk.NewButtonFromIconName("document-edit-symbolic")
	iv.EditDescBtn.AddCSSClass("flat")
	iv.EditDescBtn.SetVAlign(gtk.AlignCenter)
	iv.EditDescBtn.SetTooltipText("Editar descrição")
	iv.EditDescBtn.SetVisible(false)
	iv.GroupDescRow.AddSuffix(iv.EditDescBtn)

	openDescEditor := func() {
		win := iv.getWindow()
		initialDesc := iv.currentDescription
		if initialDesc == "" && iv.GroupDescRow.Subtitle() != "Adicionar uma descrição..." {
			initialDesc = iv.GroupDescRow.Subtitle()
		}
		ShowEditTextAreaDialog(win, "Editar descrição", "Descrição do grupo", initialDesc, func(val string) {
			iv.currentDescription = val
			if val != "" {
				iv.GroupDescRow.SetSubtitle(val)
			} else {
				iv.GroupDescRow.SetSubtitle("Adicionar uma descrição...")
			}
			if iv.OnEditGroupDescription != nil {
				iv.OnEditGroupDescription(val)
			}
		})
	}

	iv.EditDescBtn.ConnectClicked(openDescEditor)
	iv.GroupDescRow.ConnectActivated(func() {
		if iv.EditDescBtn.Visible() {
			openDescEditor()
		}
	})

	iv.GroupDescGroup.Add(iv.GroupDescRow)
	mainBox.Append(iv.GroupDescGroup)

	// 3. Community Sub-groups (when viewing a Community)
	iv.CommunityGroupsGroup = adw.NewPreferencesGroup()
	iv.CommunityGroupsGroup.SetTitle("Grupos desta comunidade")

	communityActionsBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	communityActionsBox.SetMarginBottom(8)

	iv.AddExistingGroupBtn = gtk.NewButtonWithLabel("Vincular grupo existente")
	iv.AddExistingGroupBtn.AddCSSClass("flat")
	iv.AddExistingGroupBtn.ConnectClicked(func() {
		if iv.OnLinkExistingGroup != nil {
			iv.OnLinkExistingGroup()
		}
	})
	communityActionsBox.Append(iv.AddExistingGroupBtn)

	iv.CreateSubGroupBtn = gtk.NewButtonWithLabel("+ Criar novo grupo")
	iv.CreateSubGroupBtn.AddCSSClass("suggested-action")
	iv.CreateSubGroupBtn.ConnectClicked(func() {
		if iv.OnCreateSubGroup != nil {
			iv.OnCreateSubGroup()
		}
	})
	communityActionsBox.Append(iv.CreateSubGroupBtn)

	iv.CommunityGroupsGroup.SetHeaderSuffix(communityActionsBox)

	iv.CommunityGroupsList = gtk.NewListBox()
	iv.CommunityGroupsList.AddCSSClass("content")
	iv.CommunityGroupsList.SetSelectionMode(gtk.SelectionNone)
	iv.CommunityGroupsGroup.Add(iv.CommunityGroupsList)
	iv.CommunityGroupsGroup.Hide()
	mainBox.Append(iv.CommunityGroupsGroup)

	// 4. Group Settings / Permissions
	iv.GroupSettingsGroup = adw.NewPreferencesGroup()
	iv.GroupSettingsGroup.SetTitle("Configurações do grupo")

	iv.AnnounceRow = adw.NewActionRow()
	iv.AnnounceRow.SetTitle("Apenas admins enviam mensagens")
	iv.AnnounceRow.SetSubtitle("Restringe mensagens no grupo aos administradores")
	iv.AnnounceSwitch = gtk.NewSwitch()
	iv.AnnounceSwitch.SetVAlign(gtk.AlignCenter)
	iv.AnnounceRow.AddSuffix(iv.AnnounceSwitch)
	iv.GroupSettingsGroup.Add(iv.AnnounceRow)

	iv.AnnounceSwitch.ConnectStateSet(func(state bool) bool {
		if iv.isUpdatingSettings {
			return false
		}
		if iv.OnToggleAnnounce != nil {
			iv.OnToggleAnnounce(state)
		}
		return false
	})

	iv.LockedRow = adw.NewActionRow()
	iv.LockedRow.SetTitle("Apenas admins editam dados")
	iv.LockedRow.SetSubtitle("Restringe alteração de nome e descrição aos administradores")
	iv.LockedSwitch = gtk.NewSwitch()
	iv.LockedSwitch.SetVAlign(gtk.AlignCenter)
	iv.LockedRow.AddSuffix(iv.LockedSwitch)
	iv.GroupSettingsGroup.Add(iv.LockedRow)

	iv.LockedSwitch.ConnectStateSet(func(state bool) bool {
		if iv.isUpdatingSettings {
			return false
		}
		if iv.OnToggleLocked != nil {
			iv.OnToggleLocked(state)
		}
		return false
	})

	iv.InviteLinkRow = adw.NewActionRow()
	iv.InviteLinkRow.SetTitle("Link de convite do grupo")
	iv.InviteLinkRow.SetSubtitle("Convidar participantes através de um link")
	iv.CopyInviteLinkBtn = gtk.NewButtonFromIconName("edit-copy-symbolic")
	iv.CopyInviteLinkBtn.SetTooltipText("Copiar link de convite")
	iv.CopyInviteLinkBtn.SetVAlign(gtk.AlignCenter)
	iv.CopyInviteLinkBtn.ConnectClicked(func() {
		if iv.OnGetInviteLink != nil {
			iv.OnGetInviteLink()
		}
	})
	iv.InviteLinkRow.AddSuffix(iv.CopyInviteLinkBtn)
	iv.GroupSettingsGroup.Add(iv.InviteLinkRow)

	iv.GroupSettingsGroup.Hide()
	mainBox.Append(iv.GroupSettingsGroup)

	// 5. Participants
	iv.ParticipantsGroup = adw.NewPreferencesGroup()
	iv.ParticipantsGroup.SetTitle("Participantes")

	iv.AddParticipantBtn = gtk.NewButtonWithLabel("+ Adicionar")
	iv.AddParticipantBtn.AddCSSClass("flat")
	iv.AddParticipantBtn.ConnectClicked(func() {
		if iv.OnAddParticipants != nil {
			iv.OnAddParticipants()
		}
	})
	iv.ParticipantsGroup.SetHeaderSuffix(iv.AddParticipantBtn)

	iv.ParticipantsList = gtk.NewListBox()
	iv.ParticipantsList.AddCSSClass("content")
	iv.ParticipantsList.SetSelectionMode(gtk.SelectionNone)
	iv.ParticipantsGroup.Add(iv.ParticipantsList)
	mainBox.Append(iv.ParticipantsGroup)

	// 6. Options
	iv.OptionsGroup = adw.NewPreferencesGroup()
	iv.OptionsGroup.SetTitle("Opções")

	iv.MuteRow = adw.NewActionRow()
	iv.MuteRow.SetTitle("Silenciar notificações")
	iv.MuteSwitch = gtk.NewSwitch()
	iv.MuteSwitch.SetVAlign(gtk.AlignCenter)
	iv.MuteRow.AddSuffix(iv.MuteSwitch)
	iv.OptionsGroup.Add(iv.MuteRow)

	iv.MuteSwitch.ConnectStateSet(func(state bool) bool {
		if iv.isMuting {
			return false
		}
		if iv.OnMuteToggled != nil {
			iv.OnMuteToggled(state)
		}
		return false
	})

	iv.ArchiveRow = adw.NewActionRow()
	iv.ArchiveRow.SetTitle("Arquivar conversa")
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
	iv.ExitRow.SetTitle("Sair do grupo")
	iv.ExitRow.SetTitleLines(1)
	iv.ExitRow.SetActivatable(true)
	iv.ExitRow.ConnectActivated(func() {
		iv.promptExitGroup()
	})
	iv.OptionsGroup.Add(iv.ExitRow)

	mainBox.Append(iv.OptionsGroup)

	iv.Scrolled = gtk.NewScrolledWindow()
	iv.Scrolled.SetChild(mainBox)
	iv.Scrolled.SetPropagateNaturalWidth(true)
	iv.Scrolled.SetPropagateNaturalHeight(true)
	iv.Scrolled.SetVExpand(true)

	iv.Box = gtk.NewBox(gtk.OrientationVertical, 0)
	iv.Box.SetVExpand(true)
	iv.Box.SetSizeRequest(360, -1)
	iv.Box.Append(iv.HeaderBar)
	iv.Box.Append(iv.Scrolled)
	iv.Box.AddCSSClass("background")
	iv.Box.AddCSSClass("info-view-box")

	return iv
}

func (iv *InfoView) getWindow() *gtk.Window {
	if iv.Box != nil && iv.Box.Root() != nil {
		if win, ok := iv.Box.Root().Cast().(*gtk.Window); ok {
			return win
		}
	}
	return nil
}

func (iv *InfoView) promptExitGroup() {
	win := iv.getWindow()
	title := "Sair do grupo?"
	body := "Você não poderá mais enviar nem receber mensagens neste grupo."
	if iv.TitleWidget != nil && iv.TitleWidget.Title() == "Comunidade" {
		title = "Sair da comunidade?"
		body = "Você deixará de participar e não receberá mais os avisos desta comunidade."
	}

	dialog := adw.NewMessageDialog(win, title, body)
	dialog.AddResponse("cancel", "Cancelar")
	dialog.AddResponse("leave", "Sair")
	dialog.SetResponseAppearance("leave", adw.ResponseDestructive)
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")

	dialog.ConnectResponse(func(resp string) {
		if resp == "leave" && iv.OnExitGroup != nil {
			iv.OnExitGroup()
		}
	})
	dialog.Present()
}

func (iv *InfoView) SetInfo(name string, jid string, tex *gdk.Texture) {
	iv.currentJID = jid
	iv.currentParentJID = ""

	if iv.TitleWidget != nil {
		if strings.HasSuffix(jid, "@g.us") {
			iv.TitleWidget.SetTitle("Dados do grupo")
		} else {
			iv.TitleWidget.SetTitle("Dados do contato")
		}
	}

	iv.NameLabel.SetText(name)
	iv.JIDLabel.SetText(jid)

	if tex != nil {
		iv.Avatar.SetCustomImage(tex)
	} else {
		iv.Avatar.SetCustomImage(nil)
		iv.Avatar.SetText(name)
	}

	iv.ParentCommunityGroup.Hide()
	iv.CommunityGroupsGroup.Hide()
	iv.GroupSettingsGroup.Hide()
	iv.GroupDescGroup.Hide()
	iv.ParticipantsGroup.Hide()
	iv.EditNameBtn.Hide()
	iv.EditDescBtn.Hide()
	iv.ExitRow.Hide()
	iv.OptionsGroup.Show()
}

func (iv *InfoView) SetArchived(archived bool) {
	iv.isArchiving = true
	iv.ArchiveSwitch.SetActive(archived)
	if archived {
		iv.ArchiveRow.SetTitle("Conversa arquivada")
	} else {
		iv.ArchiveRow.SetTitle("Arquivar conversa")
	}
	iv.isArchiving = false
}

func (iv *InfoView) SetMuted(muted bool) {
	iv.isMuting = true
	iv.MuteSwitch.SetActive(muted)
	if muted {
		iv.MuteRow.SetTitle("Notificações silenciadas")
	} else {
		iv.MuteRow.SetTitle("Silenciar notificações")
	}
	iv.isMuting = false
}

func (iv *InfoView) SetGroupDetails(desc string, participants []ParticipantModel) {
	iv.SetExtendedGroupDetails(ExtendedGroupDetails{
		JID:          iv.currentJID,
		Description:  desc,
		Participants: participants,
	})
}

// SetExtendedGroupDetails populates the complete group/community information view.
func (iv *InfoView) SetExtendedGroupDetails(details ExtendedGroupDetails) {
	iv.currentJID = details.JID
	iv.currentParentJID = details.ParentCommunity

	// 1. Title
	if iv.TitleWidget != nil {
		if details.IsCommunity {
			iv.TitleWidget.SetTitle("Comunidade")
			iv.ExitRow.SetTitle("Sair da comunidade")
		} else {
			iv.TitleWidget.SetTitle("Dados do grupo")
			iv.ExitRow.SetTitle("Sair do grupo")
		}
	}

	// 2. Name editing
	canEdit := details.IsAdmin || !details.IsLocked
	iv.EditNameBtn.SetVisible(canEdit)

	// 3. Description
	iv.currentDescription = details.Description
	if details.Description != "" {
		iv.GroupDescRow.SetSubtitle(details.Description)
		iv.GroupDescGroup.Show()
	} else if canEdit {
		iv.GroupDescRow.SetSubtitle("Adicionar uma descrição...")
		iv.GroupDescGroup.Show()
	} else {
		iv.GroupDescGroup.Hide()
	}
	iv.EditDescBtn.SetVisible(canEdit)
	iv.GroupDescRow.SetActivatable(canEdit)

	// 4. Linked Community
	if details.ParentCommunity != "" {
		label := details.ParentName
		if label == "" {
			label = details.ParentCommunity
		}
		iv.ParentCommunityRow.SetSubtitle(label)
		iv.ParentCommunityGroup.Show()
	} else {
		iv.ParentCommunityGroup.Hide()
	}

	// 5. Community Sub-groups
	if details.IsCommunity {
		for iv.CommunityGroupsList.FirstChild() != nil {
			iv.CommunityGroupsList.Remove(iv.CommunityGroupsList.FirstChild())
		}

		for _, sg := range details.SubGroups {
			subGroup := sg
			row := adw.NewActionRow()
			row.SetTitle(subGroup.Name)
			row.SetActivatable(true)

			icon := gtk.NewImageFromIconName("system-users-symbolic")
			row.AddPrefix(icon)

			if subGroup.IsDefaultSub {
				annLabel := gtk.NewLabel("Avisos")
				annLabel.AddCSSClass("dim-label")
				annLabel.SetVAlign(gtk.AlignCenter)
				row.AddSuffix(annLabel)
				row.SetSubtitle("Grupo de avisos da comunidade")
			}

			// Open button
			openBtn := gtk.NewButtonFromIconName("go-next-symbolic")
			openBtn.AddCSSClass("flat")
			openBtn.SetVAlign(gtk.AlignCenter)
			openBtn.SetTooltipText("Abrir grupo")
			openBtn.ConnectClicked(func() {
				if iv.OnSelectSubGroup != nil {
					iv.OnSelectSubGroup(subGroup.JID)
				}
			})
			row.AddSuffix(openBtn)

			// Unlink button for admins
			if details.IsAdmin && !subGroup.IsDefaultSub {
				unlinkBtn := gtk.NewButtonFromIconName("edit-delete-symbolic")
				unlinkBtn.AddCSSClass("flat")
				unlinkBtn.SetVAlign(gtk.AlignCenter)
				unlinkBtn.SetTooltipText("Desvincular da comunidade")
				unlinkBtn.ConnectClicked(func() {
					if iv.OnUnlinkSubGroup != nil {
						iv.OnUnlinkSubGroup(subGroup.JID)
					}
				})
				row.AddSuffix(unlinkBtn)
			}

			row.ConnectActivated(func() {
				if iv.OnSelectSubGroup != nil {
					iv.OnSelectSubGroup(subGroup.JID)
				}
			})

			iv.CommunityGroupsList.Append(row)
		}

		iv.AddExistingGroupBtn.SetVisible(details.IsAdmin)
		iv.CreateSubGroupBtn.SetVisible(details.IsAdmin)
		iv.CommunityGroupsGroup.SetTitle(fmt.Sprintf("Grupos desta comunidade (%d)", len(details.SubGroups)))
		iv.CommunityGroupsGroup.Show()
	} else {
		iv.CommunityGroupsGroup.Hide()
	}

	// 6. Permissions / Settings
	if details.IsAdmin {
		iv.isUpdatingSettings = true
		iv.AnnounceSwitch.SetActive(details.IsAnnounce)
		iv.LockedSwitch.SetActive(details.IsLocked)
		iv.isUpdatingSettings = false

		if details.InviteLink != "" {
			iv.InviteLinkRow.SetSubtitle(details.InviteLink)
		} else {
			iv.InviteLinkRow.SetSubtitle("Clique no ícone para copiar o link de convite")
		}

		iv.GroupSettingsGroup.Show()
		iv.AddParticipantBtn.Show()
	} else {
		iv.GroupSettingsGroup.Hide()
		iv.AddParticipantBtn.Hide()
	}

	// 7. Participants
	for iv.ParticipantsList.FirstChild() != nil {
		iv.ParticipantsList.Remove(iv.ParticipantsList.FirstChild())
	}

	for _, p := range details.Participants {
		part := p
		isMe := details.MyJID != "" && (part.JID == details.MyJID || strings.HasPrefix(details.MyJID, part.JID) || strings.HasPrefix(part.JID, details.MyJID))

		row := adw.NewActionRow()
		nameText := part.Name
		if isMe {
			nameText += " (Você)"
		}
		row.SetTitle(nameText)
		row.SetSubtitle(part.JID)

		avatar := adw.NewAvatar(40, part.Name, true)
		if part.Avatar != nil {
			avatar.SetCustomImage(part.Avatar)
		}
		row.AddPrefix(avatar)

		badgeBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
		badgeBox.SetVAlign(gtk.AlignCenter)

		if part.IsOwner {
			ownerLabel := gtk.NewLabel("Criador")
			ownerLabel.AddCSSClass("dim-label")
			badgeBox.Append(ownerLabel)
		} else if part.IsAdmin {
			adminLabel := gtk.NewLabel("Admin")
			adminLabel.AddCSSClass("dim-label")
			badgeBox.Append(adminLabel)
		}

		if !isMe {
			msgBtn := gtk.NewButtonFromIconName("chat-symbolic")
			msgBtn.SetTooltipText("Conversar")
			msgBtn.AddCSSClass("flat")
			msgBtn.ConnectClicked(func() {
				if iv.OnMessageParticipant != nil {
					iv.OnMessageParticipant(part.JID)
				}
			})
			badgeBox.Append(msgBtn)

			if details.IsAdmin {
				if !part.IsAdmin {
					promoteBtn := gtk.NewButtonFromIconName("security-high-symbolic")
					promoteBtn.SetTooltipText("Promover a admin")
					promoteBtn.AddCSSClass("flat")
					promoteBtn.ConnectClicked(func() {
						if iv.OnPromoteParticipant != nil {
							iv.OnPromoteParticipant(part.JID)
						}
					})
					badgeBox.Append(promoteBtn)
				} else if !part.IsOwner {
					demoteBtn := gtk.NewButtonFromIconName("security-low-symbolic")
					demoteBtn.SetTooltipText("Remover como admin")
					demoteBtn.AddCSSClass("flat")
					demoteBtn.ConnectClicked(func() {
						if iv.OnDemoteParticipant != nil {
							iv.OnDemoteParticipant(part.JID)
						}
					})
					badgeBox.Append(demoteBtn)
				}

				if !part.IsOwner {
					removeBtn := gtk.NewButtonFromIconName("user-trash-symbolic")
					removeBtn.SetTooltipText("Remover do grupo")
					removeBtn.AddCSSClass("flat")
					removeBtn.ConnectClicked(func() {
						if iv.OnRemoveParticipant != nil {
							iv.OnRemoveParticipant(part.JID)
						}
					})
					badgeBox.Append(removeBtn)
				}
			}
		}

		row.AddSuffix(badgeBox)
		iv.ParticipantsList.Append(row)
	}

	iv.ParticipantsGroup.SetTitle(fmt.Sprintf("Participantes (%d)", len(details.Participants)))
	iv.ParticipantsGroup.Show()
	iv.ExitRow.Show()
	iv.OptionsGroup.Show()
}

// SetGroupDescription updates the stored description and UI presentation.
func (iv *InfoView) SetGroupDescription(desc string) {
	iv.currentDescription = desc
	if desc != "" {
		iv.GroupDescRow.SetSubtitle(desc)
		iv.GroupDescGroup.Show()
	} else {
		iv.GroupDescRow.SetSubtitle("Adicionar uma descrição...")
	}
}
