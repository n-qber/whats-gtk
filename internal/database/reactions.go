package database

import (
	"time"
)

type Reaction struct {
	MessageID string
	SenderJID string
	Reaction  string
	Timestamp time.Time
}

func (a *AppDB) SaveReaction(r Reaction) error {
	if r.Reaction == "" {
		// Empty reaction means remove
		query := `DELETE FROM reactions WHERE msg_id = ? AND sender_jid = ?`
		_, err := a.db.Exec(query, r.MessageID, r.SenderJID)
		return err
	}
	query := `INSERT OR REPLACE INTO reactions (msg_id, sender_jid, reaction, timestamp) 
	          VALUES (?, ?, ?, ?)`
	_, err := a.db.Exec(query, r.MessageID, r.SenderJID, r.Reaction, r.Timestamp)
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
	return reactions, nil
}
