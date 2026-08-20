package paths

import (
	"os"
	"path/filepath"
)

// DataDir returns the directory where application databases and persistent state are stored.
// Conforms to the XDG Base Directory specification ($XDG_DATA_HOME/whats-gtk or ~/.local/share/whats-gtk).
func DataDir() string {
	if custom := os.Getenv("WHATS_GTK_DATA_DIR"); custom != "" {
		_ = os.MkdirAll(custom, 0755)
		return custom
	}

	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		dataHome = filepath.Join(home, ".local", "share")
	}

	dir := filepath.Join(dataHome, "whats-gtk")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// CacheDir returns the directory where cached media and temporary assets are stored.
// Conforms to the XDG Base Directory specification ($XDG_CACHE_HOME/whats-gtk or ~/.cache/whats-gtk).
func CacheDir() string {
	if custom := os.Getenv("WHATS_GTK_CACHE_DIR"); custom != "" {
		_ = os.MkdirAll(custom, 0755)
		return custom
	}

	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		cacheHome = filepath.Join(home, ".cache")
	}

	dir := filepath.Join(cacheHome, "whats-gtk")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// MediaDir returns the directory where downloaded media and avatars are stored.
// Backward compatibility: If a "./media" directory exists in the current working directory,
// it is preferred for development environments. Otherwise, $XDG_CACHE_HOME/whats-gtk/media is used.
func MediaDir() string {
	if stat, err := os.Stat("media"); err == nil && stat.IsDir() {
		return "media"
	}
	dir := filepath.Join(CacheDir(), "media")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// MediaPath returns the full path for a media file inside MediaDir().
func MediaPath(filename string) string {
	return filepath.Join(MediaDir(), filename)
}

// AppDBPath returns the path to the application SQLite database.
// Backward compatibility: If "./app.db" exists in the current working directory,
// it is used directly; otherwise DataDir()/app.db is returned.
func AppDBPath() string {
	if _, err := os.Stat("app.db"); err == nil {
		return "app.db"
	}
	return filepath.Join(DataDir(), "app.db")
}

// StoreDBPath returns the path to the whatsmeow session container SQLite database.
// Backward compatibility: If "./store.db" exists in the current working directory,
// it is used directly; otherwise DataDir()/store.db is returned.
func StoreDBPath() string {
	if _, err := os.Stat("store.db"); err == nil {
		return "store.db"
	}
	return filepath.Join(DataDir(), "store.db")
}
