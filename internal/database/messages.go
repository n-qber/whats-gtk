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
	Caption   sql.NullString
	Type      string
	Timestamp time.Time
	Status    string
	IsFromMe  bool
	Thumbnail []byte
	IsPinned   bool
	IsEdited   bool
	IsViewOnce bool
	IsForwarded bool

	// Media Metadata
	MediaURL           sql.NullString
	MediaDirectPath    sql.NullString
	MediaKey           []byte
	MediaMimetype      sql.NullString
	MediaEncSHA256     []byte
	MediaSHA256        []byte
	MediaLength        sql.NullInt64
	MediaWidth         sql.NullInt64
	MediaHeight        sql.NullInt64

	// Quoted Message
	QuotedMsgID      sql.NullString
	QuotedMsgContent sql.NullString
	QuotedMsgSender  sql.NullString
}

func (a *AppDB) SaveMessage(m Message) error {
	query := `INSERT INTO messages (
				msg_id, chat_jid, sender_jid, content, caption, type, timestamp, status, is_from_me, thumbnail,
				media_url, media_direct_path, media_key, media_mimetype, media_enc_sha256, media_sha256, media_length,
				media_width, media_height,
				quoted_msg_id, quoted_msg_content, quoted_msg_sender, is_pinned, is_edited, is_view_once, is_forwarded
			  ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			  ON CONFLICT(msg_id) DO UPDATE SET
				chat_jid=excluded.chat_jid,
				sender_jid=excluded.sender_jid,
				content=CASE WHEN excluded.content != '' THEN excluded.content ELSE messages.content END,
				caption=COALESCE(excluded.caption, messages.caption),
				type=CASE WHEN excluded.type != '' THEN excluded.type ELSE messages.type END,
				timestamp=excluded.timestamp,
				status=CASE 
					WHEN excluded.status != '' AND excluded.status IS NOT NULL THEN 
						CASE WHEN (
							CASE excluded.status WHEN 'read' THEN 4 WHEN 'delivered' THEN 3 WHEN 'sent' THEN 2 WHEN 'pending' THEN 1 ELSE 0 END >=
							CASE messages.status WHEN 'read' THEN 4 WHEN 'delivered' THEN 3 WHEN 'sent' THEN 2 WHEN 'pending' THEN 1 ELSE 0 END
						) THEN excluded.status ELSE messages.status END
					ELSE messages.status 
				END,
				is_from_me=excluded.is_from_me,
				thumbnail=COALESCE(excluded.thumbnail, messages.thumbnail),
				media_url=COALESCE(excluded.media_url, messages.media_url),
				media_direct_path=COALESCE(excluded.media_direct_path, messages.media_direct_path),
				media_key=COALESCE(excluded.media_key, messages.media_key),
				media_mimetype=COALESCE(excluded.media_mimetype, messages.media_mimetype),
				media_enc_sha256=COALESCE(excluded.media_enc_sha256, messages.media_enc_sha256),
				media_sha256=COALESCE(excluded.media_sha256, messages.media_sha256),
				media_length=COALESCE(excluded.media_length, messages.media_length),
				media_width=COALESCE(excluded.media_width, messages.media_width),
				media_height=COALESCE(excluded.media_height, messages.media_height),
				quoted_msg_id=COALESCE(excluded.quoted_msg_id, messages.quoted_msg_id),
				quoted_msg_content=COALESCE(excluded.quoted_msg_content, messages.quoted_msg_content),
				quoted_msg_sender=COALESCE(excluded.quoted_msg_sender, messages.quoted_msg_sender),
				is_pinned=COALESCE(excluded.is_pinned, messages.is_pinned),
				is_edited=COALESCE(excluded.is_edited, messages.is_edited),
				is_view_once=COALESCE(excluded.is_view_once, messages.is_view_once),
				is_forwarded=COALESCE(excluded.is_forwarded, messages.is_forwarded)`
	_, err := a.db.Exec(query, 
		m.ID, m.ChatJID, m.SenderJID, m.Content, m.Caption, m.Type, m.Timestamp, m.Status, m.IsFromMe, m.Thumbnail,
		m.MediaURL, m.MediaDirectPath, m.MediaKey, m.MediaMimetype, m.MediaEncSHA256, m.MediaSHA256, m.MediaLength,
		m.MediaWidth, m.MediaHeight,
		m.QuotedMsgID, m.QuotedMsgContent, m.QuotedMsgSender, m.IsPinned, m.IsEdited, m.IsViewOnce, m.IsForwarded,
	)
	return err
}

func (a *AppDB) UpdateMessageStatus(msgID string, chatJID string, status string) error {
	query := `UPDATE messages SET status = ? 
	          WHERE msg_id = ? AND (chat_jid = ? OR chat_jid IS NULL OR chat_jid = '')
	          AND (
	              status IS NULL OR status = '' OR
	              CASE status WHEN 'read' THEN 4 WHEN 'delivered' THEN 3 WHEN 'sent' THEN 2 WHEN 'pending' THEN 1 ELSE 0 END <=
	              CASE ? WHEN 'read' THEN 4 WHEN 'delivered' THEN 3 WHEN 'sent' THEN 2 WHEN 'pending' THEN 1 ELSE 0 END
	          )`
	_, err := a.db.Exec(query, status, msgID, chatJID, status)
	return err
}

func (a *AppDB) SaveReceipt(msgID, chatJID, userJID, receiptType string, timestamp time.Time) error {
	query := `INSERT INTO message_receipts (msg_id, chat_jid, user_jid, receipt_type, timestamp)
	          VALUES (?, ?, ?, ?, ?)
	          ON CONFLICT(msg_id, user_jid, receipt_type) DO UPDATE SET timestamp = excluded.timestamp`
	_, err := a.db.Exec(query, msgID, chatJID, userJID, receiptType, timestamp)
	return err
}

func (a *AppDB) GetGroupReceiptCounts(msgID string, msgSenderJID string) (readCount int, deliveredCount int, err error) {
	readQuery := `SELECT COUNT(DISTINCT user_jid) FROM message_receipts 
	              WHERE msg_id = ? AND receipt_type = 'read' AND user_jid != ?`
	err = a.db.QueryRow(readQuery, msgID, msgSenderJID).Scan(&readCount)
	if err != nil {
		return 0, 0, err
	}

	delivQuery := `SELECT COUNT(DISTINCT user_jid) FROM message_receipts 
	               WHERE msg_id = ? AND receipt_type IN ('delivered', 'read') AND user_jid != ?`
	err = a.db.QueryRow(delivQuery, msgID, msgSenderJID).Scan(&deliveredCount)
	if err != nil {
		return readCount, 0, err
	}

	return readCount, deliveredCount, nil
}

func (a *AppDB) UpdateMessageContent(msgID, chatJID, content string, isEdited bool) error {
	query := `UPDATE messages SET content = ?, is_edited = ? WHERE msg_id = ? AND chat_jid = ?`
	_, err := a.db.Exec(query, content, isEdited, msgID, chatJID)
	return err
}

func (a *AppDB) UpdateMessagePinned(msgID, chatJID string, pinned bool) error {
	query := `UPDATE messages SET is_pinned = ? WHERE msg_id = ? AND chat_jid = ?`
	_, err := a.db.Exec(query, pinned, msgID, chatJID)
	return err
}

func (a *AppDB) GetMessage(msgID string) (*Message, error) {
	query := `SELECT msg_id, chat_jid, sender_jid, content, caption, type, timestamp, status, is_from_me, thumbnail,
				media_url, media_direct_path, media_key, media_mimetype, media_enc_sha256, media_sha256, media_length,
				media_width, media_height,
				quoted_msg_id, quoted_msg_content, quoted_msg_sender, is_pinned, is_edited, is_view_once, is_forwarded
	          FROM messages WHERE msg_id = ?`
	row := a.db.QueryRow(query, msgID)
	var m Message
	err := row.Scan(
		&m.ID, &m.ChatJID, &m.SenderJID, &m.Content, &m.Caption, &m.Type, &m.Timestamp, &m.Status, &m.IsFromMe, &m.Thumbnail,
		&m.MediaURL, &m.MediaDirectPath, &m.MediaKey, &m.MediaMimetype, &m.MediaEncSHA256, &m.MediaSHA256, &m.MediaLength,
		&m.MediaWidth, &m.MediaHeight,
		&m.QuotedMsgID, &m.QuotedMsgContent, &m.QuotedMsgSender, &m.IsPinned, &m.IsEdited, &m.IsViewOnce, &m.IsForwarded,
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

	query := fmt.Sprintf(`SELECT msg_id, chat_jid, sender_jid, content, caption, type, timestamp, status, is_from_me, thumbnail,
				media_url, media_direct_path, media_key, media_mimetype, media_enc_sha256, media_sha256, media_length,
				media_width, media_height,
				quoted_msg_id, quoted_msg_content, quoted_msg_sender, is_pinned, is_edited, is_view_once, is_forwarded
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
			&m.ID, &m.ChatJID, &m.SenderJID, &m.Content, &m.Caption, &m.Type, &m.Timestamp, &m.Status, &m.IsFromMe, &m.Thumbnail,
			&m.MediaURL, &m.MediaDirectPath, &m.MediaKey, &m.MediaMimetype, &m.MediaEncSHA256, &m.MediaSHA256, &m.MediaLength,
			&m.MediaWidth, &m.MediaHeight,
			&m.QuotedMsgID, &m.QuotedMsgContent, &m.QuotedMsgSender, &m.IsPinned, &m.IsEdited, &m.IsViewOnce, &m.IsForwarded,
		)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

func (a *AppDB) GetMessagesBetween(jids []string, start, end time.Time, limit int) ([]Message, error) {
	if len(jids) == 0 { return nil, nil }
	placeholders := make([]string, len(jids))
	args := make([]interface{}, len(jids))
	for i, j := range jids {
		placeholders[i] = "?"
		args[i] = j
	}
	args = append(args, start)
	args = append(args, end)
	args = append(args, limit)

	query := fmt.Sprintf(`SELECT msg_id, chat_jid, sender_jid, content, caption, type, timestamp, status, is_from_me, thumbnail,
				media_url, media_direct_path, media_key, media_mimetype, media_enc_sha256, media_sha256, media_length,
				media_width, media_height,
				quoted_msg_id, quoted_msg_content, quoted_msg_sender, is_pinned, is_edited, is_view_once, is_forwarded
	          FROM (SELECT * FROM messages WHERE chat_jid IN (%s) AND timestamp >= ? AND timestamp < ? ORDER BY timestamp DESC LIMIT ?)
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
			&m.ID, &m.ChatJID, &m.SenderJID, &m.Content, &m.Caption, &m.Type, &m.Timestamp, &m.Status, &m.IsFromMe, &m.Thumbnail,
			&m.MediaURL, &m.MediaDirectPath, &m.MediaKey, &m.MediaMimetype, &m.MediaEncSHA256, &m.MediaSHA256, &m.MediaLength,
			&m.MediaWidth, &m.MediaHeight,
			&m.QuotedMsgID, &m.QuotedMsgContent, &m.QuotedMsgSender, &m.IsPinned, &m.IsEdited, &m.IsViewOnce, &m.IsForwarded,
		)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// GetMessagesAround returns messages centered around a specific target message ID.
func (a *AppDB) GetMessagesAround(jids []string, targetMsgID string, limit int) ([]Message, error) {
	targetMsg, err := a.GetMessage(targetMsgID)
	if err != nil || targetMsg == nil {
		return a.GetMessages(jids, limit)
	}

	half := limit / 2
	if half < 10 {
		half = 10
	}

	if len(jids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(jids))
	argsBefore := make([]interface{}, len(jids))
	argsAfter := make([]interface{}, len(jids))
	for i, j := range jids {
		placeholders[i] = "?"
		argsBefore[i] = j
		argsAfter[i] = j
	}
	argsBefore = append(argsBefore, targetMsg.Timestamp, half)
	argsAfter = append(argsAfter, targetMsg.Timestamp, half)

	queryBefore := fmt.Sprintf(`SELECT msg_id, chat_jid, sender_jid, content, caption, type, timestamp, status, is_from_me, thumbnail,
				media_url, media_direct_path, media_key, media_mimetype, media_enc_sha256, media_sha256, media_length,
				media_width, media_height,
				quoted_msg_id, quoted_msg_content, quoted_msg_sender, is_pinned, is_edited, is_view_once, is_forwarded
	          FROM (SELECT * FROM messages WHERE chat_jid IN (%s) AND timestamp <= ? ORDER BY timestamp DESC LIMIT ?)
	          ORDER BY timestamp ASC`, strings.Join(placeholders, ","))

	queryAfter := fmt.Sprintf(`SELECT msg_id, chat_jid, sender_jid, content, caption, type, timestamp, status, is_from_me, thumbnail,
				media_url, media_direct_path, media_key, media_mimetype, media_enc_sha256, media_sha256, media_length,
				media_width, media_height,
				quoted_msg_id, quoted_msg_content, quoted_msg_sender, is_pinned, is_edited, is_view_once, is_forwarded
	          FROM (SELECT * FROM messages WHERE chat_jid IN (%s) AND timestamp > ? ORDER BY timestamp ASC LIMIT ?)
	          ORDER BY timestamp ASC`, strings.Join(placeholders, ","))

	var msgs []Message
	rowsBefore, err := a.db.Query(queryBefore, argsBefore...)
	if err == nil {
		for rowsBefore.Next() {
			var m Message
			if err := rowsBefore.Scan(
				&m.ID, &m.ChatJID, &m.SenderJID, &m.Content, &m.Caption, &m.Type, &m.Timestamp, &m.Status, &m.IsFromMe, &m.Thumbnail,
				&m.MediaURL, &m.MediaDirectPath, &m.MediaKey, &m.MediaMimetype, &m.MediaEncSHA256, &m.MediaSHA256, &m.MediaLength,
				&m.MediaWidth, &m.MediaHeight,
				&m.QuotedMsgID, &m.QuotedMsgContent, &m.QuotedMsgSender, &m.IsPinned, &m.IsEdited, &m.IsViewOnce, &m.IsForwarded,
			); err == nil {
				msgs = append(msgs, m)
			}
		}
		rowsBefore.Close()
	}

	rowsAfter, err := a.db.Query(queryAfter, argsAfter...)
	if err == nil {
		for rowsAfter.Next() {
			var m Message
			if err := rowsAfter.Scan(
				&m.ID, &m.ChatJID, &m.SenderJID, &m.Content, &m.Caption, &m.Type, &m.Timestamp, &m.Status, &m.IsFromMe, &m.Thumbnail,
				&m.MediaURL, &m.MediaDirectPath, &m.MediaKey, &m.MediaMimetype, &m.MediaEncSHA256, &m.MediaSHA256, &m.MediaLength,
				&m.MediaWidth, &m.MediaHeight,
				&m.QuotedMsgID, &m.QuotedMsgContent, &m.QuotedMsgSender, &m.IsPinned, &m.IsEdited, &m.IsViewOnce, &m.IsForwarded,
			); err == nil {
				msgs = append(msgs, m)
			}
		}
		rowsAfter.Close()
	}

	if len(msgs) == 0 {
		return a.GetMessages(jids, limit)
	}

	return msgs, nil
}

func (a *AppDB) GetOlderMessages(jids []string, before time.Time, limit int) ([]Message, error) {
	if len(jids) == 0 { return nil, nil }
	placeholders := make([]string, len(jids))
	args := make([]interface{}, len(jids))
	for i, j := range jids {
		placeholders[i] = "?"
		args[i] = j
	}
	args = append(args, before)
	args = append(args, limit)

	query := fmt.Sprintf(`SELECT msg_id, chat_jid, sender_jid, content, caption, type, timestamp, status, is_from_me, thumbnail,
				media_url, media_direct_path, media_key, media_mimetype, media_enc_sha256, media_sha256, media_length,
				media_width, media_height,
				quoted_msg_id, quoted_msg_content, quoted_msg_sender, is_pinned, is_edited, is_view_once, is_forwarded
	          FROM (SELECT * FROM messages WHERE chat_jid IN (%s) AND timestamp < ? ORDER BY timestamp DESC LIMIT ?)
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
			&m.ID, &m.ChatJID, &m.SenderJID, &m.Content, &m.Caption, &m.Type, &m.Timestamp, &m.Status, &m.IsFromMe, &m.Thumbnail,
			&m.MediaURL, &m.MediaDirectPath, &m.MediaKey, &m.MediaMimetype, &m.MediaEncSHA256, &m.MediaSHA256, &m.MediaLength,
			&m.MediaWidth, &m.MediaHeight,
			&m.QuotedMsgID, &m.QuotedMsgContent, &m.QuotedMsgSender, &m.IsPinned, &m.IsEdited, &m.IsViewOnce, &m.IsForwarded,
		)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

func (a *AppDB) SearchMessagesInChat(jids []string, queryStr string, limit int) ([]Message, error) {
	if len(jids) == 0 { return nil, nil }
	placeholders := make([]string, len(jids))
	args := make([]interface{}, len(jids))
	for i, j := range jids {
		placeholders[i] = "?"
		args[i] = j
	}
	
	// Add wildcard to query
	searchQuery := "%" + queryStr + "%"
	args = append(args, searchQuery)
	args = append(args, limit)

	query := fmt.Sprintf(`SELECT msg_id, chat_jid, sender_jid, content, caption, type, timestamp, status, is_from_me, thumbnail,
				media_url, media_direct_path, media_key, media_mimetype, media_enc_sha256, media_sha256, media_length,
				media_width, media_height,
				quoted_msg_id, quoted_msg_content, quoted_msg_sender, is_pinned, is_edited, is_view_once, is_forwarded
	          FROM (SELECT * FROM messages WHERE chat_jid IN (%s) AND content LIKE ? COLLATE NOCASE ORDER BY timestamp DESC LIMIT ?)
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
			&m.ID, &m.ChatJID, &m.SenderJID, &m.Content, &m.Caption, &m.Type, &m.Timestamp, &m.Status, &m.IsFromMe, &m.Thumbnail,
			&m.MediaURL, &m.MediaDirectPath, &m.MediaKey, &m.MediaMimetype, &m.MediaEncSHA256, &m.MediaSHA256, &m.MediaLength,
			&m.MediaWidth, &m.MediaHeight,
			&m.QuotedMsgID, &m.QuotedMsgContent, &m.QuotedMsgSender, &m.IsPinned, &m.IsEdited, &m.IsViewOnce, &m.IsForwarded,
		)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

func (a *AppDB) SearchMessages(query string, limit int) ([]Message, error) {
	q := `SELECT msg_id, chat_jid, sender_jid, content, caption, type, timestamp, status, is_from_me, thumbnail,
			media_url, media_direct_path, media_key, media_mimetype, media_enc_sha256, media_sha256, media_length,
			media_width, media_height,
			quoted_msg_id, quoted_msg_content, quoted_msg_sender, is_pinned, is_edited, is_view_once, is_forwarded
		  FROM messages 
		  WHERE content LIKE ? OR caption LIKE ? 
		  ORDER BY timestamp DESC LIMIT ?`
	
	searchStr := "%" + query + "%"
	rows, err := a.db.Query(q, searchStr, searchStr, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(
			&m.ID, &m.ChatJID, &m.SenderJID, &m.Content, &m.Caption, &m.Type, &m.Timestamp, &m.Status, &m.IsFromMe, &m.Thumbnail,
			&m.MediaURL, &m.MediaDirectPath, &m.MediaKey, &m.MediaMimetype, &m.MediaEncSHA256, &m.MediaSHA256, &m.MediaLength,
			&m.MediaWidth, &m.MediaHeight,
			&m.QuotedMsgID, &m.QuotedMsgContent, &m.QuotedMsgSender, &m.IsPinned, &m.IsEdited, &m.IsViewOnce, &m.IsForwarded,
		); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}
