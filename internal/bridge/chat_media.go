package bridge

import (
	"database/sql"
	"fmt"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"whats-gtk/internal/database"
	"whats-gtk/internal/paths"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// HandlePasteImage sends a pasted image with optimistic UI.
func (cc *ChatController) HandlePasteImage(targetJID types.JID, tex *gdk.Texture) {
	now := time.Now().Format("15:04")
	tempID := fmt.Sprintf("temp_%d", time.Now().UnixNano())

	glib.IdleAdd(func() {
		if cv := cc.App.GetChatViewForJID(targetJID.ToNonAD().String()); cv != nil {
			cv.AddImage(tempID, "", "", "", tex, nil, "", true, false, "pending", now, nil, "", "", "", int(tex.Width()), int(tex.Height()))
			cv.ScrollToBottom()
		}
	})

	go func() {
		pixbuf := gdk.PixbufGetFromTexture(tex)
		if pixbuf == nil {
			return
		}

		tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("paste_%d.png", time.Now().UnixNano()))
		err := pixbuf.Savev(tmpPath, "png", nil, nil)
		if err != nil {
			fmt.Printf("Bridge: Failed to save temp image: %v\n", err)
			return
		}
		defer os.Remove(tmpPath)

		data, err := os.ReadFile(tmpPath)
		if err != nil {
			fmt.Printf("Bridge: Failed to read temp image: %v\n", err)
			return
		}

		resp, err := cc.Backend.SendImage(cc.ctx, targetJID, data, "image/png")
		if err != nil {
			fmt.Printf("Bridge: SendImage failed: %v\n", err)
			return
		}

		cc.promoteTempMessage(targetJID, tempID, resp.ID)

		path := paths.MediaPath(resp.ID + ".jpg")
		os.WriteFile(path, data, 0644)
		cc.DB.SaveMessage(database.Message{
			ID: resp.ID, ChatJID: targetJID.ToNonAD().String(), SenderJID: cc.Backend.Device.ID.ToNonAD().String(),
			Content: path, Type: "image", Timestamp: resp.Timestamp, Status: "sent", IsFromMe: true,
			MediaWidth:  sql.NullInt64{Int64: int64(tex.Width()), Valid: true},
			MediaHeight: sql.NullInt64{Int64: int64(tex.Height()), Valid: true},
		})
		cc.DB.UpdateContactTimestamp(targetJID.ToNonAD().String(), resp.Timestamp)
	}()
}

// HandleSendFile sends a file (image, video, audio, or document) with optimistic UI.
func (cc *ChatController) HandleSendFile(targetJID types.JID, path string) {
	now := time.Now().Format("15:04")
	filename := filepath.Base(path)

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("Bridge: Failed to read file: %v\n", err)
		return
	}

	mimetype := mime.TypeByExtension(filepath.Ext(path))
	if mimetype == "" {
		mimetype = http.DetectContentType(data)
	}

	msgType := "document"
	if strings.HasPrefix(mimetype, "image/") {
		msgType = "image"
	} else if strings.HasPrefix(mimetype, "video/") {
		msgType = "video"
	} else if strings.HasPrefix(mimetype, "audio/") {
		msgType = "audio"
	}

	tempID := "temp_file"
	glib.IdleAdd(func() {
		if cv := cc.App.GetChatViewForJID(targetJID.ToNonAD().String()); cv != nil {
			jidStr := targetJID.ToNonAD().String()
			switch msgType {
			case "image":
				pixbuf, _ := gdkpixbuf.NewPixbufFromFile(path)
				var tex *gdk.Texture
				if pixbuf != nil {
					tex = gdk.NewTextureForPixbuf(pixbuf)
				}
				cv.AddImage(tempID, jidStr, "", "", tex, nil, path, true, false, "pending", now, nil, "", "", "", 0, 0)
			case "document":
				cv.AddDocument(tempID, jidStr, "", filename, nil, true, false, "pending", now, nil, "", "", "")
			case "audio":
				cv.AddAudio(tempID, jidStr, "", true, false, "pending", now, nil, "", "", "")
			case "video":
				cv.AddVideo(tempID, jidStr, "", "", nil, path, true, false, "pending", now, nil, "", "", "", 0, 0)
			}
			cv.ScrollToBottom()
		}
	})

	go func() {
		var resp whatsmeow.SendResponse
		var err error

		switch msgType {
		case "image":
			resp, err = cc.Backend.SendImage(cc.ctx, targetJID, data, mimetype)
		case "video":
			resp, err = cc.Backend.SendVideo(cc.ctx, targetJID, data, mimetype)
		case "audio":
			resp, err = cc.Backend.SendAudio(cc.ctx, targetJID, data, mimetype)
		case "document":
			resp, err = cc.Backend.SendDocument(cc.ctx, targetJID, data, mimetype, filename)
		}

		if err != nil {
			fmt.Printf("Bridge: SendFile failed: %v\n", err)
			return
		}

		cc.promoteTempMessage(targetJID, tempID, resp.ID)

		ext := filepath.Ext(path)
		if ext == "" {
			ext = ".bin"
			switch msgType {
			case "image":
				ext = ".jpg"
			case "video":
				ext = ".mp4"
			case "audio":
				ext = ".ogg"
			}
		}

		dbPath := paths.MediaPath(resp.ID + ext)
		os.WriteFile(dbPath, data, 0644)

		cc.DB.SaveMessage(database.Message{
			ID: resp.ID, ChatJID: targetJID.ToNonAD().String(), SenderJID: cc.Backend.Device.ID.ToNonAD().String(),
			Content: dbPath, Type: msgType, Timestamp: resp.Timestamp, Status: "sent", IsFromMe: true,
		})
		cc.DB.UpdateContactTimestamp(targetJID.ToNonAD().String(), resp.Timestamp)
	}()
}

// HandleDownloadMedia reads a message from DB and dispatches a DownloadTask.
func (cc *ChatController) HandleDownloadMedia(id string) {
	fmt.Printf("Bridge: handleDownloadMedia called for %s\n", id)
	msg, err := cc.DB.GetMessage(id)
	if err != nil {
		fmt.Printf("Bridge: Message %s not found in DB\n", id)
		return
	}

	if msg.MediaURL.String == "" && msg.MediaDirectPath.String == "" {
		fmt.Printf("Bridge: Message %s has no media URLs\n", id)
		return
	}

	metadata := &MediaMetadata{
		URL: msg.MediaURL.String, DirectPath: msg.MediaDirectPath.String,
		MediaKey: msg.MediaKey, Mimetype: msg.MediaMimetype.String,
		FileEncSHA256: msg.MediaEncSHA256, FileSHA256: msg.MediaSHA256,
		FileLength: uint64(msg.MediaLength.Int64),
	}

	fmt.Printf("Bridge: Sending DownloadTask for %s (type %s)\n", id, msg.Type)
	cc.Media.Download(DownloadTask{
		ID: id, ChatJID: msg.ChatJID, SenderJID: msg.SenderJID, MsgType: msg.Type, Metadata: metadata,
	})
}

// HandleOpenImage opens a file with the system's default application.
func (cc *ChatController) HandleOpenImage(path string) {
	fmt.Printf("Bridge: handleOpenImage called for %s\n", path)
	cmd := exec.Command("xdg-open", path)
	err := cmd.Start()
	if err != nil {
		fmt.Printf("Bridge: Failed to open image: %v\n", err)
	}
}

// promoteTempMessage replaces a temporary message ID with the real one in the UI.
func (cc *ChatController) promoteTempMessage(targetJID types.JID, tempID, realID string) {
	glib.IdleAdd(func() {
		if cc.selectedJID != nil && cc.selectedJID.ToNonAD().String() == targetJID.ToNonAD().String() {
			cc.App.ChatView.UpdateMessageStatus(tempID, "sent")
			if b, exists := cc.App.ChatView.MessageList.MessageRows[tempID]; exists {
				cc.App.ChatView.MessageList.MessageRows[realID] = b
				delete(cc.App.ChatView.MessageList.MessageRows, tempID)
			}
			if r, exists := cc.App.ChatView.MessageList.MessageListRows[tempID]; exists {
				cc.App.ChatView.MessageList.MessageListRows[realID] = r
				delete(cc.App.ChatView.MessageList.MessageListRows, tempID)
			}
		}
	})
}

// HandleSendSticker sends a sticker item with optimistic UI.
func (cc *ChatController) HandleSendSticker(targetJID types.JID, item database.StickerItem) {
	now := time.Now().Format("15:04")
	tempID := fmt.Sprintf("temp_stkr_%d", time.Now().UnixNano())
	jidStr := targetJID.ToNonAD().String()

	data, err := os.ReadFile(item.FilePath)
	if err != nil {
		fmt.Printf("Bridge: Failed to read sticker file %s: %v\n", item.FilePath, err)
		return
	}

	anim, _ := gdkpixbuf.NewPixbufAnimationFromFile(item.FilePath)
	isAnimated := item.IsAnimated || (anim != nil && !anim.IsStaticImage())

	var tex *gdk.Texture
	pb, _ := gdkpixbuf.NewPixbufFromFileAtSize(item.FilePath, 160, 160)
	if pb != nil {
		tex = gdk.NewTextureForPixbuf(pb)
	}

	w, h := item.Width, item.Height
	if w <= 0 && pb != nil {
		w, h = pb.Width(), pb.Height()
	}

	glib.IdleAdd(func() {
		if cv := cc.App.GetChatViewForJID(jidStr); cv != nil {
			cv.AddSticker(tempID, jidStr, "", anim, tex, nil, true, false, "pending", now, nil, "", "", "", w, h)
			cv.ScrollToBottom()
		}
	})

	go func() {
		resp, err := cc.Backend.SendSticker(cc.ctx, targetJID, data, isAnimated)
		if err != nil {
			fmt.Printf("Bridge: SendSticker failed: %v\n", err)
			glib.IdleAdd(func() {
				if cv := cc.App.GetChatViewForJID(jidStr); cv != nil {
					cv.UpdateMessageStatus(tempID, "failed")
				}
			})
			return
		}

		cc.promoteTempMessage(targetJID, tempID, resp.ID)

		dbPath := paths.MediaPath(resp.ID + ".webp")
		if item.FilePath != dbPath {
			_ = os.WriteFile(dbPath, data, 0644)
		}

		cc.DB.SaveMessage(database.Message{
			ID: resp.ID, ChatJID: jidStr, SenderJID: cc.Backend.Device.ID.ToNonAD().String(),
			Content: dbPath, Type: "sticker", Timestamp: resp.Timestamp, Status: "sent", IsFromMe: true,
			MediaMimetype: sql.NullString{String: "image/webp", Valid: true},
			MediaWidth:    sql.NullInt64{Int64: int64(w), Valid: w > 0},
			MediaHeight:   sql.NullInt64{Int64: int64(h), Valid: h > 0},
		})
		cc.DB.UpdateContactTimestamp(jidStr, resp.Timestamp)

		item.ID = resp.ID
		item.FilePath = dbPath
		item.LastUsedAt = resp.Timestamp
		item.IsAnimated = isAnimated
		_ = cc.DB.RecordStickerHistory(item)
	}()
}

// HandleSendStickerFile converts (if needed) and sends a local image or webp file as a sticker.
func (cc *ChatController) HandleSendStickerFile(targetJID types.JID, filePath string) {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".webp" {
		item := database.StickerItem{
			ID:       fmt.Sprintf("file_%d", time.Now().UnixNano()),
			FilePath: filePath,
			Mimetype: "image/webp",
		}
		cc.HandleSendSticker(targetJID, item)
		return
	}

	// If not WebP, load and convert to a 512x512 max WebP sticker
	pb, err := gdkpixbuf.NewPixbufFromFile(filePath)
	if err != nil {
		fmt.Printf("Bridge: Failed to load image for sticker: %v\n", err)
		return
	}

	origW, origH := float64(pb.Width()), float64(pb.Height())
	maxDim := 512.0
	targetW, targetH := origW, origH
	if origW > maxDim || origH > maxDim {
		scale := maxDim / origW
		if origH > origW {
			scale = maxDim / origH
		}
		targetW = origW * scale
		targetH = origH * scale
	}

	scaled := pb.ScaleSimple(int(targetW), int(targetH), gdkpixbuf.InterpBilinear)
	if scaled == nil {
		scaled = pb
	}

	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("stkr_conv_%d.webp", time.Now().UnixNano()))
	err = scaled.Savev(tmpPath, "webp", nil, nil)
	if err != nil {
		fmt.Printf("Bridge: Failed to convert image to webp sticker: %v\n", err)
		return
	}

	item := database.StickerItem{
		ID:       fmt.Sprintf("conv_%d", time.Now().UnixNano()),
		FilePath: tmpPath,
		Mimetype: "image/webp",
		Width:    int(targetW),
		Height:   int(targetH),
	}
	cc.HandleSendSticker(targetJID, item)
}

// HandleToggleFavoriteSticker toggles favorite status for a sticker in database and WhatsApp appstate.
func (cc *ChatController) HandleToggleFavoriteSticker(item database.StickerItem, isFav bool) {
	if isFav {
		_ = cc.DB.SaveFavoriteSticker(item)
	} else {
		_ = cc.DB.DeleteFavoriteSticker(item.ID)
		if item.FilePath != "" {
			_ = cc.DB.DeleteFavoriteSticker(item.FilePath)
		}
	}

	go func() {
		if cc.Backend != nil {
			_ = cc.Backend.SetFavoriteStickerAppState(cc.ctx, item, isFav)
		}
	}()
}
