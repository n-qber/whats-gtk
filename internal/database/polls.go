package database

import (
	"crypto/sha256"
	"encoding/hex"
)

func (a *AppDB) SavePollOption(msgID, optionName string) error {
	hashBytes := sha256.Sum256([]byte(optionName))
	hash := hex.EncodeToString(hashBytes[:])

	query := `INSERT OR REPLACE INTO poll_options (msg_id, option_hash, option_name) VALUES (?, ?, ?)`
	_, err := a.db.Exec(query, msgID, hash, optionName)
	return err
}

func (a *AppDB) GetPollOptions(msgID string) (map[string]string, error) {
	query := `SELECT option_hash, option_name FROM poll_options WHERE msg_id = ?`
	rows, err := a.db.Query(query, msgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	opts := make(map[string]string)
	for rows.Next() {
		var h, n string
		if err := rows.Scan(&h, &n); err == nil {
			opts[h] = n
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return opts, nil
}

func (a *AppDB) UpdatePollVote(msgID, senderJID string, selectedHashes []string) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}

	// Delete old votes for this sender
	if _, err := tx.Exec(`DELETE FROM poll_votes WHERE msg_id = ? AND sender_jid = ?`, msgID, senderJID); err != nil {
		tx.Rollback()
		return err
	}

	// Insert new votes
	stmt, err := tx.Prepare(`INSERT INTO poll_votes (msg_id, option_hash, sender_jid) VALUES (?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, h := range selectedHashes {
		if _, err := stmt.Exec(msgID, h, senderJID); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func (a *AppDB) GetPollVotes(msgID string) (map[string][]string, error) {
	query := `SELECT option_hash, sender_jid FROM poll_votes WHERE msg_id = ?`
	rows, err := a.db.Query(query, msgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	votes := make(map[string][]string)
	for rows.Next() {
		var h, s string
		if err := rows.Scan(&h, &s); err == nil {
			votes[h] = append(votes[h], s)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return votes, nil
}
