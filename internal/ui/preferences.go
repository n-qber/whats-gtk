package ui

import (
	"os/exec"
	"whats-gtk/internal/notifications"
	"whats-gtk/internal/paths"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// ShowPreferencesDialog presents the native GNOME Adwaita preferences window.
func ShowPreferencesDialog(parent *gtk.Window, notifier *notifications.Notifier, onLogout func()) *adw.PreferencesWindow {
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

	soundRow := adw.NewSwitchRow()
	soundRow.SetTitle("Sons de notificação")
	soundRow.SetSubtitle("Tocar um som ao receber novas mensagens")
	if notifier != nil {
		soundRow.SetActive(notifier.IsSoundEnabled())
	}
	soundRow.Connect("notify::active", func() {
		if notifier != nil {
			notifier.SetSoundEnabled(soundRow.Active())
		}
	})
	notifGroup.Add(soundRow)
	page.Add(notifGroup)

	// 2. Storage & Media Group
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

	// 3. Account Group
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
