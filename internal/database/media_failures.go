package database

import (
	"time"
)

// MarkMediaFailed records a media ID that failed permanently or has expired.
func (a *AppDB) MarkMediaFailed(id, reason string) error {
	if id == "" {
		return nil
	}
	_, err := a.db.Exec(`
		INSERT INTO media_failures (id, failed_at, reason)
		VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			failed_at = excluded.failed_at,
			reason = excluded.reason
	`, id, time.Now(), reason)
	return err
}

// IsMediaFailed checks whether a media ID is marked as failed.
func (a *AppDB) IsMediaFailed(id string) bool {
	if id == "" {
		return false
	}
	var exists bool
	err := a.db.QueryRow("SELECT EXISTS(SELECT 1 FROM media_failures WHERE id = ?)", id).Scan(&exists)
	return err == nil && exists
}

// ClearMediaFailed removes a media ID from failed_media (e.g. upon successful download).
func (a *AppDB) ClearMediaFailed(id string) error {
	if id == "" {
		return nil
	}
	_, err := a.db.Exec("DELETE FROM media_failures WHERE id = ?", id)
	return err
}

// GetFailedMediaIDs returns all recorded failed media IDs.
func (a *AppDB) GetFailedMediaIDs() ([]string, error) {
	rows, err := a.db.Query("SELECT id FROM media_failures")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}
