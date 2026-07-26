# Contributing to whats-gtk

Thank you for your interest in contributing to **whats-gtk**! We welcome contributions of all kinds — whether you're fixing bugs, adding new features, improving documentation, or polishing the GTK4/Adwaita user interface.

---

## 📋 Table of Contents

- [Code of Conduct & Vision](#-code-of-conduct--vision)
- [How Can I Contribute?](#-how-can-i-contribute)
  - [Reporting Bugs](#reporting-bugs)
  - [Suggesting Features](#suggesting-features)
  - [Submitting Pull Requests](#submitting-pull-requests)
- [Development Setup & Workflow](#-development-setup--workflow)
  - [Quick Start with Nix](#quick-start-with-nix)
  - [Build Commands](#build-commands)
- [Architecture & Coding Guidelines](#-architecture--coding-guidelines)
  - [Project Structure](#project-structure)
  - [GTK4 Threading Safety Rule](#gtk4-threading-safety-rule)
  - [Code Formatting](#code-formatting)
- [Commit Message Convention](#-commit-message-convention)
- [License](#-license)

---

## 🌟 Code of Conduct & Vision

`whats-gtk` aims to be the premier native WhatsApp client for the Linux desktop. We strive to:
- **Respect GNOME Human Interface Guidelines (HIG)**: Keep UI elements modern, clean, and native.
- **Maintain Performance**: Keep memory footprint low and startup instant.
- **Foster a Welcoming Community**: Be respectful, constructive, and helpful to all contributors.

---

## 🙋 How Can I Contribute?

### Reporting Bugs

Before creating a bug report, please check existing issues to see if it has already been reported. When submitting an issue, please include:
1. **Summary & Steps to Reproduce**: Clear, concise steps to reproduce the bug.
2. **Environment Information**:
   - Linux Distribution & Desktop Environment (e.g. NixOS + Sway, Fedora + GNOME).
   - Display Server (Wayland or X11).
   - `whats-gtk` commit hash or version.
3. **Relevant Logs**: Terminal output or log trace snippets.

### Suggesting Features

Feature requests are always welcome! When suggesting a feature:
- Describe the problem or use-case the feature addresses.

### Submitting Pull Requests

1. **Fork the repository** and create your feature branch from `main`:
   ```bash
   git checkout -b feature/my-cool-feature
   ```
2. **Make your changes** following the coding guidelines below.
3. **Verify your build** and format your code:
   ```bash
   make dev
   go fmt ./...
   ```
4. **Commit your changes** with descriptive commit messages.
5. **Push to your fork** and open a Pull Request against `main`.

---

## 🛠️ Development Setup & Workflow

### Quick Start with Nix

The easiest and most reproducible way to set up the development environment is using **Nix**:

```bash
git clone https://github.com/n-qber/whats-gtk.git
cd whats-gtk
nix-shell
```

Inside `nix-shell`, all necessary dependencies (Go, GTK4, Libadwaita, mold linker, SQLite) are automatically available.

### Build Commands

We provide a `Makefile` for fast development cycles:

| Command | Action |
| :--- | :--- |
| `make dev` | Fast development build using `mold` linker and CGO optimizations (`-O1`) |
| `make run` | Builds the dev binary and launches `whats-gtk` immediately |
| `make build` | Optimized release build (stripped symbols with `-ldflags="-s -w"`) |
| `make clean` | Removes compiled binaries |

---

## 🏗️ Architecture & Coding Guidelines

### Project Structure

- **`cmd/whats-gtk/`**: Application entrypoint (`main.go`), system tray initialization (`fyne.io/systray`), and embedded assets.
- **`internal/backend/`**: `whatsmeow` client initialization, database connection, and WhatsApp protocol handlers.
- **`internal/bridge/`**: Event bridge routing async events from `whatsmeow` to the GTK main loop.
- **`internal/database/`**: SQLite local app database logic.
- **`internal/ui/`**: GTK4 / Libadwaita widgets (`chat/`, `sidebar/`, `info/`).

### ⚠️ GTK4 Threading Safety Rule

GTK is single-threaded. **All UI updates triggered by background goroutines or async callbacks MUST be dispatched to the GTK main thread using `glib.IdleAdd`**:

```go
// ❌ INCORRECT: Updating GTK widget directly from a goroutine
go func() {
    label.SetText("New Message") // May crash or cause memory corruption
}()

// ✅ CORRECT: Wrapping UI updates in glib.IdleAdd
go func() {
    glib.IdleAdd(func() {
        label.SetText("New Message")
    })
}()
```

### Code Formatting

Before submitting a PR, make sure your code passes standard Go formatting and lint checks:

```bash
go fmt ./...
go vet ./...
```

---

## 📝 Commit Message Convention

We recommend using **Conventional Commits** for clear git history:

- `feat(ui): add emoji picker to message input`
- `fix(tray): handle activate signal on swaybar`
- `docs: update build instructions in README`
- `refactor(bridge): simplify message event handlers`

---

## 📄 License

By contributing to `whats-gtk`, you agree that your contributions will be licensed under the project's [LICENSE](LICENSE).
