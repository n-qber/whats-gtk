package chat

import (
	"strings"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
)

// FormatDecorationLayout ensures minimize and maximize are included in the decoration layout
// while preserving the user's start/end button orientation.
func FormatDecorationLayout(val string) string {
	val = strings.TrimSpace(val)
	if val == "" {
		return ":minimize,maximize,close"
	}

	parts := strings.Split(val, ":")
	if len(parts) != 2 {
		return ":minimize,maximize,close"
	}

	left := parts[0]
	right := parts[1]

	// Check if close is on the left side (start)
	if strings.Contains(left, "close") {
		btns := strings.Split(left, ",")
		var newBtns []string
		hasMin := false
		hasMax := false
		for _, b := range btns {
			b = strings.TrimSpace(b)
			if b == "minimize" {
				hasMin = true
			}
			if b == "maximize" {
				hasMax = true
			}
			if b != "" {
				newBtns = append(newBtns, b)
			}
		}
		if !hasMin {
			newBtns = append(newBtns, "minimize")
		}
		if !hasMax {
			newBtns = append(newBtns, "maximize")
		}
		return strings.Join(newBtns, ",") + ":" + right
	}

	// Buttons on right (standard GNOME/Linux/Windows)
	return left + ":minimize,maximize,close"
}

// WindowDecorationLayout returns a window decoration layout string that guarantees
// the presence of minimize, maximize, and close buttons.
func WindowDecorationLayout() string {
	defaultLayout := ":minimize,maximize,close"

	src := gio.SettingsSchemaSourceGetDefault()
	if src == nil {
		return defaultLayout
	}
	schema := src.Lookup("org.gnome.desktop.wm.preferences", true)
	if schema == nil {
		return defaultLayout
	}

	settings := gio.NewSettings("org.gnome.desktop.wm.preferences")
	val := settings.String("button-layout")
	return FormatDecorationLayout(val)
}
