package chat

import (
	"context"
	"os"
	"path/filepath"
	"time"
	"whats-gtk/internal/database"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// StickerPicker represents a popover containing favorited, recent, and custom stickers.
type StickerPicker struct {
	Popover       *gtk.Popover
	Stack         *gtk.Stack
	FavFlowBox    *gtk.FlowBox
	RecFlowBox    *gtk.FlowBox
	FavEmptyBox   *gtk.Box
	RecEmptyBox   *gtk.Box

	OnSelectSticker    func(item database.StickerItem)
	OnSendStickerFile  func(filePath string)
	OnToggleFavorite   func(item database.StickerItem, isFav bool)
	OnDeleteHistory    func(id string)
	OnSyncFavorites    func()
	IsFavorite         func(id, filePath string) bool
	LoadFavorites      func() []database.StickerItem
	LoadHistory        func() []database.StickerItem

	ctx          context.Context
	lastSyncTime time.Time
}

// NewStickerPicker creates a new sticker picker popover attached to the given parent widget.
func NewStickerPicker(parent gtk.Widgetter, ctx context.Context) *StickerPicker {
	sp := &StickerPicker{
		ctx: ctx,
	}

	sp.Popover = gtk.NewPopover()
	sp.Popover.SetParent(parent)
	sp.Popover.AddCSSClass("sticker-picker-popover")

	container := gtk.NewBox(gtk.OrientationVertical, 6)
	container.SetMarginTop(8)
	container.SetMarginBottom(8)
	container.SetMarginStart(8)
	container.SetMarginEnd(8)
	container.SetSizeRequest(320, 360)

	// Top Bar: Switcher + Sync + Custom Sticker Button
	topBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	topBox.SetHAlign(gtk.AlignFill)

	sp.Stack = gtk.NewStack()
	sp.Stack.SetTransitionType(gtk.StackTransitionTypeSlideLeftRight)
	sp.Stack.SetVExpand(true)
	sp.Stack.SetHExpand(true)

	switcher := gtk.NewStackSwitcher()
	switcher.SetStack(sp.Stack)
	switcher.AddCSSClass("sticker-picker-switcher")
	topBox.Append(switcher)

	spacer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	spacer.SetHExpand(true)
	topBox.Append(spacer)

	syncBtn := gtk.NewButtonFromIconName("view-refresh-symbolic")
	syncBtn.SetTooltipText("Sync favorites from WhatsApp")
	syncBtn.AddCSSClass("flat")
	syncBtn.ConnectClicked(func() {
		sp.lastSyncTime = time.Now()
		if sp.OnSyncFavorites != nil {
			sp.OnSyncFavorites()
		}
		glib.TimeoutAdd(1200, func() bool {
			sp.Reload()
			return false
		})
	})
	topBox.Append(syncBtn)

	addCustomBtn := gtk.NewButtonFromIconName("list-add-symbolic")
	addCustomBtn.SetTooltipText("Send image or .webp file as sticker")
	addCustomBtn.AddCSSClass("flat")
	topBox.Append(addCustomBtn)

	container.Append(topBox)

	// Favorites Page
	favScrolled := gtk.NewScrolledWindow()
	favScrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	favScrolled.SetVExpand(true)
	favScrolled.SetHExpand(true)

	favContainer := gtk.NewBox(gtk.OrientationVertical, 0)
	sp.FavFlowBox = gtk.NewFlowBox()
	sp.FavFlowBox.SetMaxChildrenPerLine(4)
	sp.FavFlowBox.SetMinChildrenPerLine(4)
	sp.FavFlowBox.SetSelectionMode(gtk.SelectionNone)
	sp.FavFlowBox.SetHomogeneous(true)
	sp.FavFlowBox.AddCSSClass("sticker-grid")
	favContainer.Append(sp.FavFlowBox)

	sp.FavEmptyBox = gtk.NewBox(gtk.OrientationVertical, 8)
	sp.FavEmptyBox.SetVAlign(gtk.AlignCenter)
	sp.FavEmptyBox.SetHAlign(gtk.AlignCenter)
	sp.FavEmptyBox.SetVExpand(true)
	sp.FavEmptyBox.SetMarginTop(40)
	favEmptyIcon := gtk.NewImageFromIconName("starred-symbolic")
	favEmptyIcon.SetPixelSize(36)
	favEmptyLabel := gtk.NewLabel("No favorite stickers yet.\nRight-click any sticker in chat to add!")
	favEmptyLabel.SetJustify(gtk.JustifyCenter)
	favEmptyLabel.AddCSSClass("dim-label")
	sp.FavEmptyBox.Append(favEmptyIcon)
	sp.FavEmptyBox.Append(favEmptyLabel)
	favContainer.Append(sp.FavEmptyBox)

	favScrolled.SetChild(favContainer)
	sp.Stack.AddTitled(favScrolled, "favorites", "⭐ Favorites")

	// Recent Page
	recScrolled := gtk.NewScrolledWindow()
	recScrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	recScrolled.SetVExpand(true)
	recScrolled.SetHExpand(true)

	recContainer := gtk.NewBox(gtk.OrientationVertical, 0)
	sp.RecFlowBox = gtk.NewFlowBox()
	sp.RecFlowBox.SetMaxChildrenPerLine(4)
	sp.RecFlowBox.SetMinChildrenPerLine(4)
	sp.RecFlowBox.SetSelectionMode(gtk.SelectionNone)
	sp.RecFlowBox.SetHomogeneous(true)
	sp.RecFlowBox.AddCSSClass("sticker-grid")
	recContainer.Append(sp.RecFlowBox)

	sp.RecEmptyBox = gtk.NewBox(gtk.OrientationVertical, 8)
	sp.RecEmptyBox.SetVAlign(gtk.AlignCenter)
	sp.RecEmptyBox.SetHAlign(gtk.AlignCenter)
	sp.RecEmptyBox.SetVExpand(true)
	sp.RecEmptyBox.SetMarginTop(40)
	recEmptyIcon := gtk.NewImageFromIconName("document-open-recent-symbolic")
	recEmptyIcon.SetPixelSize(36)
	recEmptyLabel := gtk.NewLabel("No recent stickers yet.")
	recEmptyLabel.SetJustify(gtk.JustifyCenter)
	recEmptyLabel.AddCSSClass("dim-label")
	sp.RecEmptyBox.Append(recEmptyIcon)
	sp.RecEmptyBox.Append(recEmptyLabel)
	recContainer.Append(sp.RecEmptyBox)

	recScrolled.SetChild(recContainer)
	sp.Stack.AddTitled(recScrolled, "recents", "🕒 Recent")

	container.Append(sp.Stack)
	sp.Popover.SetChild(container)

	// File chooser for custom stickers
	addCustomBtn.ConnectClicked(func() {
		dialog := gtk.NewFileDialog()
		dialog.SetTitle("Select Sticker Image")
		
		filter := gtk.NewFileFilter()
		filter.SetName("Sticker Images (*.webp, *.png, *.jpg, *.gif)")
		filter.AddPixbufFormats()
		filter.AddMIMEType("image/webp")
		
		filters := gio.NewListStore(filter.Type())
		filters.Append(filter.Object)
		dialog.SetFilters(filters)

		var window *gtk.Window
		if root := gtk.BaseWidget(parent).Root(); root != nil {
			if win, ok := root.Cast().(*gtk.Window); ok {
				window = win
			}
		}

		dialog.Open(sp.ctx, window, func(res gio.AsyncResulter) {
			file, err := dialog.OpenFinish(res)
			if err == nil && file != nil && sp.OnSendStickerFile != nil {
				sp.OnSendStickerFile(file.Path())
				sp.Popover.Popdown()
			}
		})
	})

	return sp
}

// Reload re-queries favorites and history and populates the flow boxes.
func (sp *StickerPicker) Reload() {
	// 1. Load Favorites
	clearFlowBox(sp.FavFlowBox)
	var favs []database.StickerItem
	if sp.LoadFavorites != nil {
		favs = sp.LoadFavorites()
	}

	validFavs := 0
	for _, item := range favs {
		if _, err := os.Stat(item.FilePath); err == nil {
			tile := sp.createTile(item, true)
			sp.FavFlowBox.Append(tile)
			validFavs++
		}
	}
	if validFavs > 0 {
		sp.FavEmptyBox.Hide()
		sp.FavFlowBox.Show()
	} else {
		sp.FavEmptyBox.Show()
		sp.FavFlowBox.Hide()
	}

	// 2. Load History
	clearFlowBox(sp.RecFlowBox)
	var history []database.StickerItem
	if sp.LoadHistory != nil {
		history = sp.LoadHistory()
	}

	validRecents := 0
	for _, item := range history {
		if _, err := os.Stat(item.FilePath); err == nil {
			tile := sp.createTile(item, false)
			sp.RecFlowBox.Append(tile)
			validRecents++
		}
	}
	if validRecents > 0 {
		sp.RecEmptyBox.Hide()
		sp.RecFlowBox.Show()
	} else {
		sp.RecEmptyBox.Show()
		sp.RecFlowBox.Hide()
	}
}

func (sp *StickerPicker) createTile(item database.StickerItem, fromFavoritesTab bool) *gtk.Button {
	btn := gtk.NewButton()
	btn.SetHasFrame(false)
	btn.AddCSSClass("sticker-picker-tile")
	btn.SetTooltipText(filepath.Base(item.FilePath))

	pic := gtk.NewPicture()
	pic.SetCanShrink(true)
	pic.SetContentFit(gtk.ContentFitContain)
	pic.SetSizeRequest(64, 64)

	pixbuf, _ := gdkpixbuf.NewPixbufFromFileAtSize(item.FilePath, 64, 64)
	if pixbuf != nil {
		pic.SetPaintable(gdk.NewTextureForPixbuf(pixbuf))
	} else {
		icon := gtk.NewImageFromIconName("image-missing-symbolic")
		icon.SetPixelSize(32)
		btn.SetChild(icon)
		return btn
	}

	btn.SetChild(pic)

	// Left click sends sticker
	btn.ConnectClicked(func() {
		if sp.OnSelectSticker != nil {
			sp.OnSelectSticker(item)
		}
		sp.Popover.Popdown()
	})

	// Right click shows context menu (Favorite / Unfavorite / Remove from History)
	rightClick := gtk.NewGestureClick()
	rightClick.SetButton(3)
	rightClick.ConnectPressed(func(n int, x, y float64) {
		tileMenu := gtk.NewPopover()
		menuBox := gtk.NewBox(gtk.OrientationVertical, 2)

		isFav := fromFavoritesTab
		if !isFav && sp.IsFavorite != nil {
			isFav = sp.IsFavorite(item.ID, item.FilePath)
		}

		favLabel := "⭐ Add to Favorites"
		if isFav {
			favLabel = "Remove from Favorites"
		}
		favBtn := gtk.NewButtonWithLabel(favLabel)
		favBtn.SetHasFrame(false)
		favBtn.ConnectClicked(func() {
			tileMenu.Popdown()
			if sp.OnToggleFavorite != nil {
				sp.OnToggleFavorite(item, !isFav)
			}
			glib.IdleAdd(func() {
				sp.Reload()
			})
		})
		menuBox.Append(favBtn)

		if !fromFavoritesTab {
			delBtn := gtk.NewButtonWithLabel("Remove from History")
			delBtn.SetHasFrame(false)
			delBtn.ConnectClicked(func() {
				tileMenu.Popdown()
				if sp.OnDeleteHistory != nil {
					sp.OnDeleteHistory(item.ID)
				}
				glib.IdleAdd(func() {
					sp.Reload()
				})
			})
			menuBox.Append(delBtn)
		}

		tileMenu.SetChild(menuBox)
		tileMenu.SetParent(btn)
		tileMenu.Popup()
	})
	btn.AddController(rightClick)

	return btn
}

func (sp *StickerPicker) Popup() {
	sp.Reload()
	if sp.OnSyncFavorites != nil && sp.FavFlowBox != nil && sp.FavFlowBox.FirstChild() == nil {
		if time.Since(sp.lastSyncTime) > 5*time.Minute {
			sp.lastSyncTime = time.Now()
			sp.OnSyncFavorites()
		}
	}
	sp.Popover.Popup()
}

func (sp *StickerPicker) Popdown() {
	sp.Popover.Popdown()
}

func clearFlowBox(fb *gtk.FlowBox) {
	if fb == nil {
		return
	}
	for {
		child := fb.FirstChild()
		if child == nil {
			break
		}
		fb.Remove(child)
	}
}
