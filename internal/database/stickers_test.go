package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) (*AppDB, func()) {
	tmpDir, err := os.MkdirTemp("", "whats_gtk_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(tmpDir, "test_app.db")
	db, err := InitDB(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to init db: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}
	return db, cleanup
}

func TestFavoriteStickers(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	item := StickerItem{
		ID:              "stkr_1",
		FilePath:        "/tmp/stkr1.webp",
		MediaURL:        "https://cdn.example.com/stkr1.enc",
		MediaDirectPath: "/v/stkr1.enc",
		MediaKey:        []byte("key1"),
		MediaSHA256:     []byte("sha1"),
		Mimetype:        "image/webp",
		Width:           512,
		Height:          512,
		IsAnimated:      false,
		CreatedAt:       time.Now(),
	}

	if err := db.SaveFavoriteSticker(item); err != nil {
		t.Fatalf("SaveFavoriteSticker failed: %v", err)
	}

	favs, err := db.GetFavoriteStickers(10)
	if err != nil {
		t.Fatalf("GetFavoriteStickers failed: %v", err)
	}
	if len(favs) != 1 {
		t.Fatalf("expected 1 favorite sticker, got %d", len(favs))
	}
	if favs[0].ID != "stkr_1" || favs[0].FilePath != "/tmp/stkr1.webp" {
		t.Errorf("unexpected favorite sticker data: %+v", favs[0])
	}

	isFav, err := db.IsStickerFavorite("stkr_1")
	if err != nil || !isFav {
		t.Errorf("expected isFav=true, got %v (err: %v)", isFav, err)
	}

	isFavPath, err := db.IsStickerFavorite("/tmp/stkr1.webp")
	if err != nil || !isFavPath {
		t.Errorf("expected isFav=true by path, got %v (err: %v)", isFavPath, err)
	}

	if err := db.DeleteFavoriteSticker("stkr_1"); err != nil {
		t.Fatalf("DeleteFavoriteSticker failed: %v", err)
	}

	isFavAfter, _ := db.IsStickerFavorite("stkr_1")
	if isFavAfter {
		t.Errorf("expected isFav=false after delete, got true")
	}
}

func TestStickerHistory(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	item := StickerItem{
		ID:         "stkr_hist_1",
		FilePath:   "/tmp/hist1.webp",
		Mimetype:   "image/webp",
		Width:      512,
		Height:     512,
		LastUsedAt: time.Now(),
	}

	if err := db.RecordStickerHistory(item); err != nil {
		t.Fatalf("RecordStickerHistory failed: %v", err)
	}

	hist, err := db.GetStickerHistory(10)
	if err != nil {
		t.Fatalf("GetStickerHistory failed: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(hist))
	}
	if hist[0].ID != "stkr_hist_1" {
		t.Errorf("unexpected history item: %+v", hist[0])
	}

	// Insert a sticker message into messages table to test fallback
	msg := Message{
		ID:        "msg_stkr_2",
		ChatJID:   "123@s.whatsapp.net",
		SenderJID: "123@s.whatsapp.net",
		Content:   "/tmp/msg_stkr2.webp",
		Type:      "sticker",
		Timestamp: time.Now().Add(-1 * time.Hour),
		Status:    "delivered",
		Caption:   sql.NullString{},
	}
	if err := db.SaveMessage(msg); err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}

	combinedHist, err := db.GetStickerHistory(10)
	if err != nil {
		t.Fatalf("GetStickerHistory with fallback failed: %v", err)
	}
	if len(combinedHist) != 2 {
		t.Fatalf("expected 2 history items (1 explicit + 1 from messages), got %d", len(combinedHist))
	}
}
