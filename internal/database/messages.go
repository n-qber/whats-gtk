package database

import (
	"database/sql"
	"fmt"
	"strings"
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
	Thumbnail []byte

	// Media Metadata
	MediaURL           sql.NullString
	MediaDirectPath    sql.NullString
	MediaKey           []byte
	MediaMimetype      sql.NullString
	MediaEncSHA256     []byte
	MediaSHA256        []byte
	MediaLength        sql.NullInt64

	// Quoted Message
	QuotedMsgID      sql.NullString
	QuotedMsgContent sql.NullString
	QuotedMsgSender  sql.NullString
}

func (a *AppDB) SaveMessage(m Message) error {
	query := `INSERT OR REPLACE INTO messages (
				msg_id, chat_jid, sender_jid, content, type, timestamp, status, is_from_me, thumbnail,
				media_url, media_direct_path, media_key, media_mimetype, media_enc_sha256, media_sha256, media_length,
				quoted_msg_id, quoted_msg_content, quoted_msg_sender
			  ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := a.db.Exec(query, 
		m.ID, m.ChatJID, m.SenderJID, m.Content, m.Type, m.Timestamp, m.Status, m.IsFromMe, m.Thumbnail,
		m.MediaURL, m.MediaDirectPath, m.MediaKey, m.MediaMimetype, m.MediaEncSHA256, m.MediaSHA256, m.MediaLength,
		m.QuotedMsgID, m.QuotedMsgContent, m.QuotedMsgSender,
	)
	return err
}

func (a *AppDB) UpdateMessageStatus(msgID string, chatJID string, status string) error {
	query := `UPDATE messages SET status = ? WHERE msg_id = ? AND chat_jid = ?`
	_, err := a.db.Exec(query, status, msgID, chatJID)
	return err
}

func (a *AppDB) UpdateMessageContent(msgID, chatJID, content string) error {
	query := `UPDATE messages SET content = ? WHERE msg_id = ? AND chat_jid = ?`
	_, err := a.db.Exec(query, content, msgID, chatJID)
	return err
}

func (a *AppDB) GetMessage(msgID string) (*Message, error) {
	query := `SELECT msg_id, chat_jid, sender_jid, content, type, timestamp, status, is_from_me, thumbnail,
				media_url, media_direct_path, media_key, media_mimetype, media_enc_sha256, media_sha256, media_length,
				quoted_msg_id, quoted_msg_content, quoted_msg_sender
	          FROM messages WHERE msg_id = ?`
	row := a.db.QueryRow(query, msgID)
	var m Message
	err := row.Scan(
		&m.ID, &m.ChatJID, &m.SenderJID, &m.Content, &m.Type, &m.Timestamp, &m.Status, &m.IsFromMe, &m.Thumbnail,
		&m.MediaURL, &m.MediaDirectPath, &m.MediaKey, &m.MediaMimetype, &m.MediaEncSHA256, &m.MediaSHA256, &m.MediaLength,
		&m.QuotedMsgID, &m.QuotedMsgContent, &m.QuotedMsgSender,
	)
	if err != nil { return nil, err }
	return &m, nil
}

func (a *AppDB) GetMessages(jids []string, limit int) ([]Message, error) {
	if len(jids) == 0 { return nil, nil }
	placeholders := make([]string, len(jids))
	args := make([]interface{}, len(jids))
	for i, j := range jids {
		placeholders[i] = "?"
		args[i] = j
	}
	args = append(args, limit)

	query := fmt.Sprintf(`SELECT msg_id, chat_jid, sender_jid, content, type, timestamp, status, is_from_me, thumbnail,
				media_url, media_direct_path, media_key, media_mimetype, media_enc_sha256, media_sha256, media_length,
				quoted_msg_id, quoted_msg_content, quoted_msg_sender
	          FROM (SELECT * FROM messages WHERE chat_jid IN (%s) ORDER BY timestamp DESC LIMIT ?)
	          ORDER BY timestamp ASC`, strings.Join(placeholders, ","))
	
	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		err := rows.Scan(
			&m.ID, &m.ChatJID, &m.SenderJID, &m.Content, &m.Type, &m.Timestamp, &m.Status, &m.IsFromMe, &m.Thumbnail,
			&m.MediaURL, &m.MediaDirectPath, &m.MediaKey, &m.MediaMimetype, &m.MediaEncSHA256, &m.MediaSHA256, &m.MediaLength,
			&m.QuotedMsgID, &m.QuotedMsgContent, &m.QuotedMsgSender,
		)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}
