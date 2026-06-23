package bridge

import (
	"whats-gtk/internal/database"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
)

// bytesToTexture converts raw image bytes to a GDK texture via pixbuf loader.
func bytesToTexture(data []byte) *gdk.Texture {
	if len(data) == 0 { return nil }
	loader := gdkpixbuf.NewPixbufLoader()
	loader.Write(data)
	loader.Close()
	pix := loader.Pixbuf()
	if pix == nil { return nil }
	return gdk.NewTextureForPixbuf(pix)
}

// uniqueReactions deduplicates reaction emojis, preserving insertion order.
func uniqueReactions(reactions []database.Reaction) []string {
	unique := make(map[string]bool)
	var result []string
	for _, r := range reactions {
		if !unique[r.Reaction] {
			unique[r.Reaction] = true
			result = append(result, r.Reaction)
		}
	}
	return result
}
