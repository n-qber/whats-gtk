package database

import (
	"database/sql"
	"time"
)

type Reaction struct {
	MessageID string
	SenderJID string
	Reaction  string
	Timestamp time.Time
}

func (a *AppDB) SaveReaction(r Reaction) error {
	return a.saveReaction(a.db, r)
}

func (a *AppDB) SaveReactionTx(tx *sql.Tx, r Reaction) error {
	return a.saveReaction(tx, r)
}

func (a *AppDB) saveReaction(exec DBExecutor, r Reaction) error {
	if r.Reaction == "" {
		// Empty reaction means remove
		query := `DELETE FROM reactions WHERE msg_id = ? AND sender_jid = ?`
		_, err := exec.Exec(query, r.MessageID, r.SenderJID)
		return err
	}
	query := `INSERT OR REPLACE INTO reactions (msg_id, sender_jid, reaction, timestamp) 
	          VALUES (?, ?, ?, ?)`
	_, err := exec.Exec(query, r.MessageID, r.SenderJID, r.Reaction, r.Timestamp)
	return err
}

func (a *AppDB) GetReactions(msgID string) ([]Reaction, error) {
	query := `SELECT msg_id, sender_jid, reaction, timestamp FROM reactions WHERE msg_id = ? ORDER BY timestamp ASC`
	rows, err := a.db.Query(query, msgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reactions []Reaction
	for rows.Next() {
		var r Reaction
		err := rows.Scan(&r.MessageID, &r.SenderJID, &r.Reaction, &r.Timestamp)
		if err != nil {
			return nil, err
		}
		reactions = append(reactions, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return reactions, nil
}
