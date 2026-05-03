package bubbles

import (
	"math"

	"github.com/gotk3/gotk3/cairo"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

func ApplyCircularAvatar(img *gtk.Image, pixbuf *gdk.Pixbuf, size int) {
	if pixbuf == nil {
		img.Clear()
		return
	}

	scaled, _ := pixbuf.ScaleSimple(size, size, gdk.INTERP_BILINEAR)

	surface := cairo.CreateImageSurface(cairo.FORMAT_ARGB32, size, size)
	cr := cairo.Create(surface)

	cr.Arc(float64(size)/2, float64(size)/2, float64(size)/2, 0, 2*math.Pi)
	cr.Clip()

	pixSurface, _ := gdk.CairoSurfaceCreateFromPixbuf(scaled, 1, nil)
	cr.SetSourceSurface(pixSurface, 0, 0)
	cr.Paint()

	img.SetFromSurface(surface)
}
