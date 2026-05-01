package database

import (
	"time"
)

type Message struct {
	ID        string
	ChatJID   string
	SenderJID string
	Content   string
	Type      string
	Timestamp time.Time
	Status    string
	IsFromMe  bool
}

func (a *AppDB) SaveMessage(m Message) error {
	query := `INSERT OR REPLACE INTO messages (msg_id, chat_jid, sender_jid, content, type, timestamp, status, is_from_me) 
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := a.db.Exec(query, m.ID, m.ChatJID, m.SenderJID, m.Content, m.Type, m.Timestamp, m.Status, m.IsFromMe)
	return err
}

func (a *AppDB) GetMessages(chatJID string, limit int) ([]Message, error) {
	query := `SELECT msg_id, chat_jid, sender_jid, content, type, timestamp, status, is_from_me 
	          FROM (SELECT * FROM messages WHERE chat_jid = ? ORDER BY timestamp DESC LIMIT ?)
	          ORDER BY timestamp ASC`
	rows, err := a.db.Query(query, chatJID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		err := rows.Scan(&m.ID, &m.ChatJID, &m.SenderJID, &m.Content, &m.Type, &m.Timestamp, &m.Status, &m.IsFromMe)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}
