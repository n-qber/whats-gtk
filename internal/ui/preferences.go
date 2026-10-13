package ui

import (
	"os/exec"
	"whats-gtk/internal/notifications"
	"whats-gtk/internal/paths"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// PreferencesSettings contains configurable preferences values and callbacks.
type PreferencesSettings struct {
	GroupNotificationsEnabled bool
	GroupUnreadEnabled        bool
	ShowLastMessagePreview    bool
	ShowGroupTag              bool
	ShowCommunityTag          bool

	OnSettingChanged func(key string, val bool)
}

// ShowPreferencesDialog presents the native GNOME Adwaita preferences window.
func ShowPreferencesDialog(parent *gtk.Window, notifier *notifications.Notifier, settings PreferencesSettings, onLogout func()) *adw.PreferencesWindow {
	win := adw.NewPreferencesWindow()
	win.SetTitle("Preferências")
	if parent != nil {
		win.SetTransientFor(parent)
	}
	win.SetModal(true)
	win.SetDefaultSize(560, 520)

	// General Page
	page := adw.NewPreferencesPage()
	page.SetTitle("Geral")
	page.SetIconName("preferences-other-symbolic")

	// 1. Notifications Group
	notifGroup := adw.NewPreferencesGroup()
	notifGroup.SetTitle("Notificações")
	notifGroup.SetDescription("Configurações de alertas na área de trabalho")

	dndRow := adw.NewSwitchRow()
	dndRow.SetTitle("Modo Não Perturbe")
	dndRow.SetSubtitle("Pausar todas as notificações e sons")
	if notifier != nil {
		dndRow.SetActive(notifier.IsDNDEnabled())
	}
	dndRow.Connect("notify::active", func() {
		if notifier != nil && notifier.IsDNDEnabled() != dndRow.Active() {
			notifier.SetDNDEnabled(dndRow.Active())
		}
	})
	notifGroup.Add(dndRow)

	soundRow := adw.NewSwitchRow()
	soundRow.SetTitle("Sons de notificação")
	soundRow.SetSubtitle("Tocar um som ao receber novas mensagens")
	if notifier != nil {
		soundRow.SetActive(notifier.IsSoundEnabled())
	}
	soundRow.Connect("notify::active", func() {
		if notifier != nil && notifier.IsSoundEnabled() != soundRow.Active() {
			notifier.SetSoundEnabled(soundRow.Active())
		}
	})
	notifGroup.Add(soundRow)

	grpNotifRow := adw.NewSwitchRow()
	grpNotifRow.SetTitle("Notificações de grupos")
	grpNotifRow.SetSubtitle("Exibir alertas na área de trabalho para mensagens recebidas em grupos")
	grpNotifRow.SetActive(settings.GroupNotificationsEnabled)
	grpNotifRow.Connect("notify::active", func() {
		active := grpNotifRow.Active()
		if notifier != nil && notifier.IsGroupNotificationsEnabled() != active {
			notifier.SetGroupNotificationsEnabled(active)
		}
		if settings.OnSettingChanged != nil {
			settings.OnSettingChanged("group_notifications_enabled", active)
		}
	})
	notifGroup.Add(grpNotifRow)
	page.Add(notifGroup)

	if notifier != nil {
		unsubDND := notifier.OnDNDChange(func(enabled bool) {
			glib.IdleAdd(func() {
				if dndRow.Active() != enabled {
					dndRow.SetActive(enabled)
				}
			})
		})
		unsubSound := notifier.OnSoundChange(func(enabled bool) {
			glib.IdleAdd(func() {
				if soundRow.Active() != enabled {
					soundRow.SetActive(enabled)
				}
			})
		})
		unsubGrp := notifier.OnGroupNotificationsChange(func(enabled bool) {
			glib.IdleAdd(func() {
				if grpNotifRow.Active() != enabled {
					grpNotifRow.SetActive(enabled)
				}
			})
		})
		win.Connect("destroy", func() {
			unsubDND()
			unsubSound()
			unsubGrp()
		})
	}

	// 2. Chat & Sidebar Group
	chatGroup := adw.NewPreferencesGroup()
	chatGroup.SetTitle("Conversas e Barra Lateral")
	chatGroup.SetDescription("Configurações de exibição de mensagens e identificadores na lista")

	previewRow := adw.NewSwitchRow()
	previewRow.SetTitle("Prévia da última mensagem")
	previewRow.SetSubtitle("Exibir o texto ou mídia da última mensagem na lista de conversas")
	previewRow.SetActive(settings.ShowLastMessagePreview)
	previewRow.Connect("notify::active", func() {
		if settings.OnSettingChanged != nil {
			settings.OnSettingChanged("show_last_message_preview", previewRow.Active())
		}
	})
	chatGroup.Add(previewRow)

	unreadRow := adw.NewSwitchRow()
	unreadRow.SetTitle("Contador de não lidas em grupos")
	unreadRow.SetSubtitle("Exibir contador de mensagens não lidas nos títulos dos grupos")
	unreadRow.SetActive(settings.GroupUnreadEnabled)
	unreadRow.Connect("notify::active", func() {
		if settings.OnSettingChanged != nil {
			settings.OnSettingChanged("group_unread_enabled", unreadRow.Active())
		}
	})
	chatGroup.Add(unreadRow)

	groupTagRow := adw.NewSwitchRow()
	groupTagRow.SetTitle("Identificador [G] em grupos")
	groupTagRow.SetSubtitle("Exibir o prefixo [G] nos nomes de grupos")
	groupTagRow.SetActive(settings.ShowGroupTag)
	groupTagRow.Connect("notify::active", func() {
		if settings.OnSettingChanged != nil {
			settings.OnSettingChanged("show_group_tag", groupTagRow.Active())
		}
	})
	chatGroup.Add(groupTagRow)

	commTagRow := adw.NewSwitchRow()
	commTagRow.SetTitle("Identificador [C] em comunidades")
	commTagRow.SetSubtitle("Exibir o prefixo [C] nos nomes de comunidades")
	commTagRow.SetActive(settings.ShowCommunityTag)
	commTagRow.Connect("notify::active", func() {
		if settings.OnSettingChanged != nil {
			settings.OnSettingChanged("show_community_tag", commTagRow.Active())
		}
	})
	chatGroup.Add(commTagRow)
	page.Add(chatGroup)

	// 3. Storage & Media Group
	storageGroup := adw.NewPreferencesGroup()
	storageGroup.SetTitle("Armazenamento e Mídia")
	storageGroup.SetDescription("Localização de mídias e arquivos baixados")

	mediaRow := adw.NewActionRow()
	mediaRow.SetTitle("Pasta de mídias")
	mediaRow.SetSubtitle(paths.MediaDir())

	openFolderBtn := gtk.NewButtonFromIconName("folder-open-symbolic")
	openFolderBtn.SetTooltipText("Abrir pasta no gerenciador de arquivos")
	openFolderBtn.SetVAlign(gtk.AlignCenter)
	openFolderBtn.ConnectClicked(func() {
		_ = exec.Command("xdg-open", paths.MediaDir()).Start()
	})
	mediaRow.AddSuffix(openFolderBtn)
	storageGroup.Add(mediaRow)
	page.Add(storageGroup)

	// 4. Account Group
	accountGroup := adw.NewPreferencesGroup()
	accountGroup.SetTitle("Conta")
	accountGroup.SetDescription("Gerenciamento da sessão ativa do WhatsApp")

	logoutRow := adw.NewActionRow()
	logoutRow.SetTitle("Desconectar da conta")
	logoutRow.SetSubtitle("Encerra a sessão deste computador e retorna à tela de pareamento QR code")

	logoutBtn := gtk.NewButtonWithLabel("Desconectar")
	logoutBtn.AddCSSClass("destructive-action")
	logoutBtn.SetVAlign(gtk.AlignCenter)
	logoutBtn.ConnectClicked(func() {
		win.Destroy()
		if onLogout != nil {
			onLogout()
		}
	})
	logoutRow.AddSuffix(logoutBtn)
	accountGroup.Add(logoutRow)
	page.Add(accountGroup)

	win.Add(page)
	win.Present()
	return win
}
