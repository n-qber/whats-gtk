<div align="center">
  <img src="cmd/whats-gtk/assets/icon.png" alt="whats-gtk logo" width="128" />
  <h1>whats-gtk</h1>
  <p><strong>A fast, native GTK4 & Libadwaita WhatsApp desktop client written in Go.</strong></p>

  [![Go Version](https://img.shields.io/badge/Go-1.21%2B-00ADD8?style=flat&logo=go)](https://golang.org)
  [![GTK4](https://img.shields.io/badge/GTK-4.0-3584E4?style=flat&logo=gnome)](https://gtk.org)
  [![Libadwaita](https://img.shields.io/badge/Libadwaita-1.0-3584E4?style=flat&logo=gnome)](https://gnome.pages.gitlab.gnome.org/libadwaita/)
  [![Linux](https://img.shields.io/badge/Platform-Linux-FCC624?style=flat&logo=linux&logoColor=black)](https://kernel.org)
  [![NixOS Ready](https://img.shields.io/badge/NixOS-Supported-5277C3?style=flat&logo=nixos)](https://nixos.org)
</div>

---

`whats-gtk` is a modern, lightweight Linux desktop client for WhatsApp. It blends seamlessly into GNOME and Wayland compositors using native GTK4 and Libadwaita widgets, backed by the high-performance Go [whatsmeow](https://github.com/tulir/whatsmeow) engine.

---

## ✨ Features

- 🎨 **Native GNOME & Libadwaita Design**: Follows modern Linux HIG design guidelines, including adaptive split-views, native dark mode, and Adwaita styling.
- 💬 **Rich Messaging Support**: Full support for text messages, images, stickers, voice notes, audio playback sliders, documents, and media progress indicators.
- 🔔 **System Tray Integration**: Runs quietly in the background when closed (`SetHideOnClose`). Features a custom status tray icon with single-click toggle (show/hide).
- 🔐 **Instant QR Login**: Embedded, seamless QR code dialog for fast device linking.
- 🔍 **Interactive Search & Shortcuts**: Instant contact/chat search and quick keyboard navigation.
- 🔍 **Dynamic UI Zooming**: Zoom in or out easily to match your HiDPI screen preferences.
- 📌 **Pinned Messages & Quoted Replies**: View pinned chat bars, quoted references, and reaction badges.
- ⚡ **Blazing Fast**: Low memory usage and instant startup powered by Go and `mold`.

---

## 🎹 Keyboard Shortcuts

| Shortcut | Action |
| :--- | :--- |
| <kbd>Ctrl</kbd> + <kbd>+</kbd> or <kbd>=</kbd> | **Zoom In** UI |
| <kbd>Ctrl</kbd> + <kbd>-</kbd> | **Zoom Out** UI |
| <kbd>Ctrl</kbd> + <kbd>Space</kbd> | Focus **Search Entry** |
| <kbd>Close (X)</kbd> | Hide window to **System Tray** |

---

## 🛠️ Prerequisites & Building

`whats-gtk` can be built reproducibly using **Nix / NixOS** or standard Linux package managers.

### Option A: Using Nix / NixOS (Recommended — Fully Reproducible)

If you have Nix installed (or are running NixOS), you can enter an isolated build shell with all dependencies pre-configured:

```bash
# 1. Clone the repository
git clone https://github.com/n-qber/whats-gtk.git
cd whats-gtk

# 2. Enter the reproducible environment & build
nix-shell --run "make build"

# 3. Launch the application
./whats-gtk
```

---

### Option B: Building on Standard Linux Distros

Ensure you have **Go 1.21+**, **GTK4**, **Libadwaita**, and development headers installed.

#### 📦 Install System Dependencies

**Ubuntu 22.04 / 24.04 & Debian 12+**:
```bash
sudo apt update
sudo apt install -y golang pkg-config libgtk-4-dev libadwaita-1-dev libasound2-dev libsqlite3-dev mold
```

**Fedora 38+**:
```bash
sudo dnf install -y golang pkgconfig gtk4-devel libadwaita-devel alsa-lib-devel sqlite-devel mold
```

**Arch Linux**:
```bash
sudo pacman -S --needed go pkgconf gtk4 libadwaita alsa-lib sqlite mold
```

#### 🏗️ Build & Run

```bash
# Fast development build
make dev

# Full optimized release build
make build

# Run directly
make run
# or
./whats-gtk
```

---

## 🖥️ System Tray Configuration Notes

`whats-gtk` uses the **StatusNotifierItem (SNI)** protocol for system tray support.

### 🔷 GNOME Desktop
GNOME requires the **AppIndicator** extension to show tray icons:
- Install and enable **[AppIndicator and KStatusNotifierItem Support](https://extensions.gnome.org/extension/615/appindicator-support/)**.
- On NixOS:
  ```nix
  services.xserver.desktopManager.gnome.extraGnomeExtensions = [
    pkgs.gnomeExtensions.appindicator
  ];
  ```

### 🔷 Sway & Wayland Compositors (`swaybar` / `waybar`)
In `swaybar`, tray icons must be explicitly enabled for display outputs in `~/.config/sway/config`:
```swayconfig
bar {
    position top
    tray_output *   # Enables system tray icons
}
```
Reload Sway with `Mod4 + Shift + C` (`swaymsg reload`).

---

## 📁 Project Architecture

```
whats-gtk/
├── cmd/
│   └── whats-gtk/         # Main entry point & embedded tray assets
│       ├── assets/        # Native application & tray icons
│       └── main.go
├── internal/
│   ├── backend/           # WhatsApp client integration (whatsmeow)
│   ├── bridge/            # Event bridge between backend & UI
│   ├── database/          # SQLite local app storage
│   └── ui/                # GTK4 & Libadwaita UI components (Chat, Sidebar, Info)
├── shell.nix              # Reproducible Nix environment specification
├── Makefile               # Build & development scripts
└── README.md
```

---

## 🤝 Contributing

Contributions, bug reports, and feature requests are welcome!
Check out [CONTRIBUTING.md](CONTRIBUTING.md) to get started.

---

## 📄 License

This project is licensed under the terms found in the [LICENSE](LICENSE) file.

---

<div align="center">
  <sub>Built with ❤️ using <a href="https://github.com/tulir/whatsmeow">whatsmeow</a> and <a href="https://github.com/diamondburned/gotk4">gotk4</a>.</sub>
</div>
