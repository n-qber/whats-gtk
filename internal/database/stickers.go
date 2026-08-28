package database

import (
	"time"
)

// StickerItem represents a WhatsApp sticker stored in favorites, history, or chat messages.
type StickerItem struct {
	ID              string
	FilePath        string
	MediaURL        string
	MediaDirectPath string
	MediaKey        []byte
	MediaSHA256     []byte
	MediaEncSHA256  []byte
	Mimetype        string
	Width           int
	Height          int
	IsAnimated      bool
	CreatedAt       time.Time
	LastUsedAt      time.Time
	UseCount        int
}

// SaveFavoriteSticker saves or updates a favorite sticker.
func (a *AppDB) SaveFavoriteSticker(item StickerItem) error {
	query := `INSERT INTO favorite_stickers (
		id, file_path, media_url, media_direct_path, media_key, media_sha256, media_enc_sha256,
		media_mimetype, width, height, is_animated, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		file_path = excluded.file_path,
		media_url = CASE WHEN excluded.media_url != '' THEN excluded.media_url ELSE favorite_stickers.media_url END,
		media_direct_path = CASE WHEN excluded.media_direct_path != '' THEN excluded.media_direct_path ELSE favorite_stickers.media_direct_path END,
		media_key = CASE WHEN length(excluded.media_key) > 0 THEN excluded.media_key ELSE favorite_stickers.media_key END,
		media_sha256 = CASE WHEN length(excluded.media_sha256) > 0 THEN excluded.media_sha256 ELSE favorite_stickers.media_sha256 END,
		media_enc_sha256 = CASE WHEN length(excluded.media_enc_sha256) > 0 THEN excluded.media_enc_sha256 ELSE favorite_stickers.media_enc_sha256 END,
		media_mimetype = excluded.media_mimetype,
		width = excluded.width,
		height = excluded.height,
		is_animated = excluded.is_animated`

	createdAt := item.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	_, err := a.db.Exec(query,
		item.ID, item.FilePath, item.MediaURL, item.MediaDirectPath, item.MediaKey, item.MediaSHA256, item.MediaEncSHA256,
		item.Mimetype, item.Width, item.Height, item.IsAnimated, createdAt,
	)
	return err
}

// DeleteFavoriteSticker removes a sticker from favorites.
func (a *AppDB) DeleteFavoriteSticker(id string) error {
	_, err := a.db.Exec("DELETE FROM favorite_stickers WHERE id = ? OR file_path = ?", id, id)
	return err
}

// UpdateFavoriteStickerFilePath updates the file path of a favorite sticker after download.
func (a *AppDB) UpdateFavoriteStickerFilePath(id string, filePath string) error {
	_, err := a.db.Exec("UPDATE favorite_stickers SET file_path = ? WHERE id = ?", filePath, id)
	return err
}

// FindExistingStickerPath searches for an existing downloaded sticker file matching the given encryption hash.
func (a *AppDB) FindExistingStickerPath(encSHA []byte) (string, error) {
	var content string
	err := a.db.QueryRow("SELECT content FROM messages WHERE media_enc_sha256 = ? AND content LIKE '%.webp' LIMIT 1", encSHA).Scan(&content)
	return content, err
}

// IsStickerFavorite checks if a sticker is in the favorites table.
func (a *AppDB) IsStickerFavorite(id string) (bool, error) {
	var exists bool
	err := a.db.QueryRow("SELECT EXISTS(SELECT 1 FROM favorite_stickers WHERE id = ? OR file_path = ?)", id, id).Scan(&exists)
	return exists, err
}

// GetFavoriteStickers returns a list of favorite stickers ordered by newest first.
func (a *AppDB) GetFavoriteStickers(limit int) ([]StickerItem, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `SELECT id, file_path, coalesce(media_url, ''), coalesce(media_direct_path, ''),
		media_key, media_sha256, media_enc_sha256, coalesce(media_mimetype, 'image/webp'),
		coalesce(width, 0), coalesce(height, 0), coalesce(is_animated, 0), created_at
	FROM favorite_stickers
	ORDER BY created_at DESC
	LIMIT ?`

	rows, err := a.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []StickerItem
	for rows.Next() {
		var item StickerItem
		var key, sha, encSha []byte
		err := rows.Scan(
			&item.ID, &item.FilePath, &item.MediaURL, &item.MediaDirectPath,
			&key, &sha, &encSha, &item.Mimetype,
			&item.Width, &item.Height, &item.IsAnimated, &item.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		item.MediaKey = key
		item.MediaSHA256 = sha
		item.MediaEncSHA256 = encSha
		items = append(items, item)
	}
	return items, rows.Err()
}

// RecordStickerHistory records or updates a sticker in recent history.
func (a *AppDB) RecordStickerHistory(item StickerItem) error {
	query := `INSERT INTO sticker_history (
		id, file_path, media_url, media_direct_path, media_key, media_sha256, media_enc_sha256,
		media_mimetype, width, height, is_animated, last_used_at, use_count
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
	ON CONFLICT(id) DO UPDATE SET
		file_path = excluded.file_path,
		media_url = CASE WHEN excluded.media_url != '' THEN excluded.media_url ELSE sticker_history.media_url END,
		media_direct_path = CASE WHEN excluded.media_direct_path != '' THEN excluded.media_direct_path ELSE sticker_history.media_direct_path END,
		media_key = CASE WHEN length(excluded.media_key) > 0 THEN excluded.media_key ELSE sticker_history.media_key END,
		media_sha256 = CASE WHEN length(excluded.media_sha256) > 0 THEN excluded.media_sha256 ELSE sticker_history.media_sha256 END,
		media_enc_sha256 = CASE WHEN length(excluded.media_enc_sha256) > 0 THEN excluded.media_enc_sha256 ELSE sticker_history.media_enc_sha256 END,
		media_mimetype = excluded.media_mimetype,
		width = excluded.width,
		height = excluded.height,
		is_animated = excluded.is_animated,
		last_used_at = excluded.last_used_at,
		use_count = sticker_history.use_count + 1`

	lastUsedAt := item.LastUsedAt
	if lastUsedAt.IsZero() {
		lastUsedAt = time.Now()
	}

	_, err := a.db.Exec(query,
		item.ID, item.FilePath, item.MediaURL, item.MediaDirectPath, item.MediaKey, item.MediaSHA256, item.MediaEncSHA256,
		item.Mimetype, item.Width, item.Height, item.IsAnimated, lastUsedAt,
	)
	return err
}

// DeleteStickerHistory removes an entry from sticker history.
func (a *AppDB) DeleteStickerHistory(id string) error {
	_, err := a.db.Exec("DELETE FROM sticker_history WHERE id = ? OR file_path = ?", id, id)
	return err
}

// GetRecentStickersFromMessages queries distinct sticker files from message history.
// This bootstraps sticker history using already downloaded sticker messages.
func (a *AppDB) GetRecentStickersFromMessages(limit int) ([]StickerItem, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT msg_id, content, coalesce(media_url, ''), coalesce(media_direct_path, ''),
		media_key, media_sha256, media_enc_sha256, coalesce(media_mimetype, 'image/webp'),
		coalesce(media_width, 0), coalesce(media_height, 0), timestamp
	FROM messages
	WHERE type = 'sticker' AND content != '' AND (content LIKE '%.webp' OR content LIKE '%media%')
	GROUP BY coalesce(media_sha256, content)
	ORDER BY timestamp DESC
	LIMIT ?`

	rows, err := a.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []StickerItem
	for rows.Next() {
		var item StickerItem
		var key, sha, encSha []byte
		err := rows.Scan(
			&item.ID, &item.FilePath, &item.MediaURL, &item.MediaDirectPath,
			&key, &sha, &encSha, &item.Mimetype,
			&item.Width, &item.Height, &item.LastUsedAt,
		)
		if err != nil {
			return nil, err
		}
		item.MediaKey = key
		item.MediaSHA256 = sha
		item.MediaEncSHA256 = encSha
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetStickerHistory returns recent stickers combining explicitly recorded history
// and recent stickers from message history.
func (a *AppDB) GetStickerHistory(limit int) ([]StickerItem, error) {
	if limit <= 0 {
		limit = 60
	}

	query := `SELECT id, file_path, coalesce(media_url, ''), coalesce(media_direct_path, ''),
		media_key, media_sha256, media_enc_sha256, coalesce(media_mimetype, 'image/webp'),
		coalesce(width, 0), coalesce(height, 0), coalesce(is_animated, 0), last_used_at, use_count
	FROM sticker_history
	ORDER BY last_used_at DESC
	LIMIT ?`

	rows, err := a.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seenPaths := make(map[string]bool)
	var items []StickerItem
	for rows.Next() {
		var item StickerItem
		var key, sha, encSha []byte
		err := rows.Scan(
			&item.ID, &item.FilePath, &item.MediaURL, &item.MediaDirectPath,
			&key, &sha, &encSha, &item.Mimetype,
			&item.Width, &item.Height, &item.IsAnimated, &item.LastUsedAt, &item.UseCount,
		)
		if err != nil {
			return nil, err
		}
		item.MediaKey = key
		item.MediaSHA256 = sha
		item.MediaEncSHA256 = encSha
		seenPaths[item.FilePath] = true
		items = append(items, item)
	}

	// Supplement with recent stickers from messages if we have room
	remaining := limit - len(items)
	if remaining > 0 {
		fromMsgs, err := a.GetRecentStickersFromMessages(remaining * 2)
		if err == nil {
			for _, m := range fromMsgs {
				if !seenPaths[m.FilePath] {
					seenPaths[m.FilePath] = true
					items = append(items, m)
					if len(items) >= limit {
						break
					}
				}
			}
		}
	}

	return items, nil
}
