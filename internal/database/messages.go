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

	MediaURL        sql.NullString
	MediaDirectPath sql.NullString
	MediaKey        []byte
	MediaMimetype   sql.NullString
	MediaEncSHA256  []byte
	MediaSHA256     []byte
	MediaLength     sql.NullInt64
	MediaWidth      sql.NullInt64
	MediaHeight     sql.NullInt64

	QuotedMsgID      sql.NullString
	QuotedMsgContent sql.NullString
	QuotedMsgSender  sql.NullString

	IsPinned   bool
	IsEdited   bool
	IsViewOnce bool
	IsForwarded bool
}

func scanMessage(s RowScanner) (Message, error) {
	var m Message
	err := s.Scan(
		&m.ID, &m.ChatJID, &m.SenderJID, &m.Content, &m.Caption, &m.Type, &m.Timestamp, &m.Status, &m.IsFromMe, &m.Thumbnail,
		&m.MediaURL, &m.MediaDirectPath, &m.MediaKey, &m.MediaMimetype, &m.MediaEncSHA256, &m.MediaSHA256, &m.MediaLength,
		&m.MediaWidth, &m.MediaHeight,
		&m.QuotedMsgID, &m.QuotedMsgContent, &m.QuotedMsgSender, &m.IsPinned, &m.IsEdited, &m.IsViewOnce, &m.IsForwarded,
	)
	return m, err
}

func (a *AppDB) SaveMessage(m Message) error {
	return a.saveMessage(a.db, m)
}

func (a *AppDB) SaveMessageTx(tx *sql.Tx, m Message) error {
	if tx == nil {
		return a.saveMessage(a.db, m)
	}
	return a.saveMessage(tx, m)
}

func (a *AppDB) saveMessage(exec DBExecutor, m Message) error {
	query := `INSERT INTO messages (
		msg_id, chat_jid, sender_jid, content, caption, type, timestamp, status, is_from_me, thumbnail,
		media_url, media_direct_path, media_key, media_mimetype, media_enc_sha256, media_sha256, media_length,
		media_width, media_height,
		quoted_msg_id, quoted_msg_content, quoted_msg_sender, is_pinned, is_edited, is_view_once, is_forwarded
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(msg_id) DO UPDATE SET
		chat_jid = CASE WHEN excluded.chat_jid != '' THEN excluded.chat_jid ELSE messages.chat_jid END,
		sender_jid = CASE WHEN excluded.sender_jid != '' THEN excluded.sender_jid ELSE messages.sender_jid END,
		type = CASE WHEN excluded.type != '' AND excluded.type IS NOT NULL THEN excluded.type ELSE messages.type END,
		timestamp = CASE WHEN excluded.timestamp IS NOT NULL AND excluded.timestamp != '' THEN excluded.timestamp ELSE messages.timestamp END,
		status = CASE WHEN excluded.status != '' THEN excluded.status ELSE messages.status END,
		is_from_me = excluded.is_from_me,
		content = CASE WHEN excluded.content != '' THEN excluded.content ELSE messages.content END,
		caption = CASE WHEN excluded.caption IS NOT NULL AND excluded.caption != '' THEN excluded.caption ELSE messages.caption END,
		thumbnail = CASE WHEN excluded.thumbnail IS NOT NULL THEN excluded.thumbnail ELSE messages.thumbnail END,
		media_url = CASE WHEN excluded.media_url IS NOT NULL AND excluded.media_url != '' THEN excluded.media_url ELSE messages.media_url END,
		media_direct_path = CASE WHEN excluded.media_direct_path IS NOT NULL AND excluded.media_direct_path != '' THEN excluded.media_direct_path ELSE messages.media_direct_path END,
		media_key = CASE WHEN excluded.media_key IS NOT NULL THEN excluded.media_key ELSE messages.media_key END,
		media_mimetype = CASE WHEN excluded.media_mimetype IS NOT NULL AND excluded.media_mimetype != '' THEN excluded.media_mimetype ELSE messages.media_mimetype END,
		media_enc_sha256 = CASE WHEN excluded.media_enc_sha256 IS NOT NULL THEN excluded.media_enc_sha256 ELSE messages.media_enc_sha256 END,
		media_sha256 = CASE WHEN excluded.media_sha256 IS NOT NULL THEN excluded.media_sha256 ELSE messages.media_sha256 END,
		media_length = CASE WHEN excluded.media_length IS NOT NULL AND excluded.media_length > 0 THEN excluded.media_length ELSE messages.media_length END,
		media_width = CASE WHEN excluded.media_width IS NOT NULL AND excluded.media_width > 0 THEN excluded.media_width ELSE messages.media_width END,
		media_height = CASE WHEN excluded.media_height IS NOT NULL AND excluded.media_height > 0 THEN excluded.media_height ELSE messages.media_height END,
		quoted_msg_id = CASE WHEN excluded.quoted_msg_id IS NOT NULL AND excluded.quoted_msg_id != '' THEN excluded.quoted_msg_id ELSE messages.quoted_msg_id END,
		quoted_msg_content = CASE WHEN excluded.quoted_msg_content IS NOT NULL AND excluded.quoted_msg_content != '' THEN excluded.quoted_msg_content ELSE messages.quoted_msg_content END,
		quoted_msg_sender = CASE WHEN excluded.quoted_msg_sender IS NOT NULL AND excluded.quoted_msg_sender != '' THEN excluded.quoted_msg_sender ELSE messages.quoted_msg_sender END,
		is_pinned = excluded.is_pinned,
		is_edited = excluded.is_edited,
		is_view_once = excluded.is_view_once,
		is_forwarded = excluded.is_forwarded`

	_, err := exec.Exec(query,
		m.ID, m.ChatJID, m.SenderJID, m.Content, m.Caption, m.Type, m.Timestamp, m.Status, m.IsFromMe, m.Thumbnail,
		m.MediaURL, m.MediaDirectPath, m.MediaKey, m.MediaMimetype, m.MediaEncSHA256, m.MediaSHA256, m.MediaLength,
		m.MediaWidth, m.MediaHeight,
		m.QuotedMsgID, m.QuotedMsgContent, m.QuotedMsgSender, m.IsPinned, m.IsEdited, m.IsViewOnce, m.IsForwarded,
	)
	return err
}

func (a *AppDB) UpdateMessageStatus(msgID string, chatJID string, status string) error {
	var query string
	var args []interface{}
	if chatJID != "" {
		query = `UPDATE messages SET status = ? WHERE msg_id = ? AND (chat_jid = ? OR chat_jid = '')`
		args = []interface{}{status, msgID, chatJID}
	} else {
		query = `UPDATE messages SET status = ? WHERE msg_id = ?`
		args = []interface{}{status, msgID}
	}
	_, err := a.db.Exec(query, args...)
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
	query := `SELECT receipt_type, COUNT(DISTINCT user_jid) 
	          FROM message_receipts 
	          WHERE msg_id = ? AND user_jid != ? 
	          GROUP BY receipt_type`
	rows, err := a.db.Query(query, msgID, msgSenderJID)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var rType string
		var count int
		if err := rows.Scan(&rType, &count); err == nil {
			if rType == "read" || rType == "read-self" {
				readCount += count
			} else if rType == "delivered" {
				deliveredCount += count
			}
		}
	}
	return readCount, deliveredCount, nil
}

func (a *AppDB) UpdateMessageContent(msgID, chatJID, content string, isEdited bool) error {
	query := `UPDATE messages SET content = ?, is_edited = ? WHERE msg_id = ? AND (chat_jid = ? OR chat_jid = '')`
	_, err := a.db.Exec(query, content, isEdited, msgID, chatJID)
	return err
}

func (a *AppDB) UpdateMessagePinned(msgID, chatJID string, pinned bool) error {
	query := `UPDATE messages SET is_pinned = ? WHERE msg_id = ? AND (chat_jid = ? OR chat_jid = '')`
	_, err := a.db.Exec(query, pinned, msgID, chatJID)
	return err
}

func (a *AppDB) GetMessage(msgID string) (*Message, error) {
	query := `SELECT msg_id, chat_jid, sender_jid, content, caption, type, timestamp, status, is_from_me, thumbnail,
				media_url, media_direct_path, media_key, media_mimetype, media_enc_sha256, media_sha256, media_length,
				media_width, media_height,
				quoted_msg_id, quoted_msg_content, quoted_msg_sender, is_pinned, is_edited, is_view_once, is_forwarded
	          FROM messages WHERE msg_id = ?`
	m, err := scanMessage(a.db.QueryRow(query, msgID))
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (a *AppDB) GetMessages(jids []string, limit int) ([]Message, error) {
	if len(jids) == 0 {
		return nil, nil
	}
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
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (a *AppDB) GetMessagesBetween(jids []string, start, end time.Time, limit int) ([]Message, error) {
	if len(jids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(jids))
	args := make([]interface{}, len(jids))
	for i, j := range jids {
		placeholders[i] = "?"
		args[i] = j
	}
	args = append(args, start, end, limit)

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
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return msgs, nil
}

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
			if m, err := scanMessage(rowsBefore); err == nil {
				msgs = append(msgs, m)
			}
		}
		rowsBefore.Close()
	}

	rowsAfter, err := a.db.Query(queryAfter, argsAfter...)
	if err == nil {
		for rowsAfter.Next() {
			if m, err := scanMessage(rowsAfter); err == nil {
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
	if len(jids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(jids))
	args := make([]interface{}, len(jids))
	for i, j := range jids {
		placeholders[i] = "?"
		args[i] = j
	}
	args = append(args, before, limit)

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
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return msgs, nil
}

func sanitizeFTS5Query(term string) string {
	term = strings.TrimSpace(term)
	if term == "" {
		return ""
	}
	term = strings.ReplaceAll(term, `"`, `""`)
	words := strings.Fields(term)
	if len(words) == 0 {
		return ""
	}
	var ftsWords []string
	for _, w := range words {
		w = strings.Map(func(r rune) rune {
			if strings.ContainsRune(`*^:()[]{}-+`, r) {
				return -1
			}
			return r
		}, w)
		if w != "" {
			ftsWords = append(ftsWords, `"`+w+`"*`)
		}
	}
	return strings.Join(ftsWords, " ")
}

func (a *AppDB) SearchMessagesInChat(jids []string, queryStr string, limit int) ([]Message, error) {
	if len(jids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(jids))
	args := make([]interface{}, len(jids))
	for i, j := range jids {
		placeholders[i] = "?"
		args[i] = j
	}

	if a.hasFTS5 {
		ftsQ := sanitizeFTS5Query(queryStr)
		if ftsQ != "" {
			ftsQuery := fmt.Sprintf(`SELECT m.msg_id, m.chat_jid, m.sender_jid, m.content, m.caption, m.type, m.timestamp, m.status, m.is_from_me, m.thumbnail,
						m.media_url, m.media_direct_path, m.media_key, m.media_mimetype, m.media_enc_sha256, m.media_sha256, m.media_length,
						m.media_width, m.media_height,
						m.quoted_msg_id, m.quoted_msg_content, m.quoted_msg_sender, m.is_pinned, m.is_edited, m.is_view_once, m.is_forwarded
					FROM messages m
					JOIN messages_fts f ON m.msg_id = f.msg_id
					WHERE m.chat_jid IN (%s) AND messages_fts MATCH ?
					ORDER BY m.timestamp ASC LIMIT ?`, strings.Join(placeholders, ","))

			ftsArgs := make([]interface{}, len(jids))
			copy(ftsArgs, args)
			ftsArgs = append(ftsArgs, ftsQ, limit)

			if rows, err := a.db.Query(ftsQuery, ftsArgs...); err == nil {
				defer rows.Close()
				var msgs []Message
				for rows.Next() {
					if m, err := scanMessage(rows); err == nil {
						msgs = append(msgs, m)
					}
				}
				if err := rows.Err(); err == nil && len(msgs) > 0 {
					return msgs, nil
				}
			}
		}
	}

	searchQuery := "%" + queryStr + "%"
	args = append(args, searchQuery, limit)

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
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (a *AppDB) SearchMessages(profileID int64, query string, limit int) ([]Message, error) {
	if a.hasFTS5 {
		ftsQ := sanitizeFTS5Query(query)
		if ftsQ != "" {
			var ftsSql string
			var ftsRows *sql.Rows
			var ftsErr error
			if profileID == ProfileArchivedID {
				ftsSql = `SELECT m.msg_id, m.chat_jid, m.sender_jid, m.content, m.caption, m.type, m.timestamp, m.status, m.is_from_me, m.thumbnail,
						m.media_url, m.media_direct_path, m.media_key, m.media_mimetype, m.media_enc_sha256, m.media_sha256, m.media_length,
						m.media_width, m.media_height,
						m.quoted_msg_id, m.quoted_msg_content, m.quoted_msg_sender, m.is_pinned, m.is_edited, m.is_view_once, m.is_forwarded
					FROM messages m
					JOIN messages_fts f ON m.msg_id = f.msg_id
					WHERE messages_fts MATCH ?
					  AND (
					      m.chat_jid IN (SELECT jid FROM contacts WHERE is_archived = 1)
					      OR m.chat_jid IN (SELECT lid FROM contacts WHERE is_archived = 1)
					  )
					ORDER BY m.timestamp DESC LIMIT ?`
				ftsRows, ftsErr = a.db.Query(ftsSql, ftsQ, limit)
			} else if profileID > 0 {
				ftsSql = `SELECT m.msg_id, m.chat_jid, m.sender_jid, m.content, m.caption, m.type, m.timestamp, m.status, m.is_from_me, m.thumbnail,
						m.media_url, m.media_direct_path, m.media_key, m.media_mimetype, m.media_enc_sha256, m.media_sha256, m.media_length,
						m.media_width, m.media_height,
						m.quoted_msg_id, m.quoted_msg_content, m.quoted_msg_sender, m.is_pinned, m.is_edited, m.is_view_once, m.is_forwarded
					FROM messages m
					JOIN messages_fts f ON m.msg_id = f.msg_id
					WHERE messages_fts MATCH ?
					  AND (
					      m.chat_jid IN (SELECT jid FROM profile_contacts WHERE profile_id = ?)
					      OR m.chat_jid IN (SELECT lid FROM contacts WHERE jid IN (SELECT jid FROM profile_contacts WHERE profile_id = ?))
					  )
					  AND (
					      m.chat_jid NOT IN (SELECT jid FROM contacts WHERE is_archived = 1)
					      AND m.chat_jid NOT IN (SELECT lid FROM contacts WHERE is_archived = 1 AND lid IS NOT NULL AND lid != '')
					  )
					ORDER BY m.timestamp DESC LIMIT ?`
				ftsRows, ftsErr = a.db.Query(ftsSql, ftsQ, profileID, profileID, limit)
			} else {
				ftsSql = `SELECT m.msg_id, m.chat_jid, m.sender_jid, m.content, m.caption, m.type, m.timestamp, m.status, m.is_from_me, m.thumbnail,
						m.media_url, m.media_direct_path, m.media_key, m.media_mimetype, m.media_enc_sha256, m.media_sha256, m.media_length,
						m.media_width, m.media_height,
						m.quoted_msg_id, m.quoted_msg_content, m.quoted_msg_sender, m.is_pinned, m.is_edited, m.is_view_once, m.is_forwarded
					FROM messages m
					JOIN messages_fts f ON m.msg_id = f.msg_id
					WHERE messages_fts MATCH ?
					  AND (
					      m.chat_jid NOT IN (SELECT jid FROM contacts WHERE is_archived = 1)
					      AND m.chat_jid NOT IN (SELECT lid FROM contacts WHERE is_archived = 1 AND lid IS NOT NULL AND lid != '')
					  )
					ORDER BY m.timestamp DESC LIMIT ?`
				ftsRows, ftsErr = a.db.Query(ftsSql, ftsQ, limit)
			}
			if ftsErr == nil && ftsRows != nil {
				defer ftsRows.Close()
				var msgs []Message
				for ftsRows.Next() {
					if m, err := scanMessage(ftsRows); err == nil {
						msgs = append(msgs, m)
					}
				}
				if err := ftsRows.Err(); err == nil && len(msgs) > 0 {
					return msgs, nil
				}
			}
		}
	}

	var q string
	var rows *sql.Rows
	var err error

	searchStr := "%" + query + "%"

	if profileID == ProfileArchivedID {
		q = `SELECT msg_id, chat_jid, sender_jid, content, caption, type, timestamp, status, is_from_me, thumbnail,
				media_url, media_direct_path, media_key, media_mimetype, media_enc_sha256, media_sha256, media_length,
				media_width, media_height,
				quoted_msg_id, quoted_msg_content, quoted_msg_sender, is_pinned, is_edited, is_view_once, is_forwarded
			  FROM messages 
			  WHERE (content LIKE ? OR caption LIKE ?)
			    AND (
			        chat_jid IN (SELECT jid FROM contacts WHERE is_archived = 1)
			        OR chat_jid IN (SELECT lid FROM contacts WHERE is_archived = 1)
			    )
			  ORDER BY timestamp DESC LIMIT ?`
		rows, err = a.db.Query(q, searchStr, searchStr, limit)
	} else if profileID > 0 {
		q = `SELECT msg_id, chat_jid, sender_jid, content, caption, type, timestamp, status, is_from_me, thumbnail,
				media_url, media_direct_path, media_key, media_mimetype, media_enc_sha256, media_sha256, media_length,
				media_width, media_height,
				quoted_msg_id, quoted_msg_content, quoted_msg_sender, is_pinned, is_edited, is_view_once, is_forwarded
			  FROM messages 
			  WHERE (content LIKE ? OR caption LIKE ?)
			    AND (
			        chat_jid IN (SELECT jid FROM profile_contacts WHERE profile_id = ?)
			        OR chat_jid IN (SELECT lid FROM contacts WHERE jid IN (SELECT jid FROM profile_contacts WHERE profile_id = ?))
			    )
			    AND (
			        chat_jid NOT IN (SELECT jid FROM contacts WHERE is_archived = 1)
			        AND chat_jid NOT IN (SELECT lid FROM contacts WHERE is_archived = 1 AND lid IS NOT NULL AND lid != '')
			    )
			  ORDER BY timestamp DESC LIMIT ?`
		rows, err = a.db.Query(q, searchStr, searchStr, profileID, profileID, limit)
	} else {
		q = `SELECT msg_id, chat_jid, sender_jid, content, caption, type, timestamp, status, is_from_me, thumbnail,
				media_url, media_direct_path, media_key, media_mimetype, media_enc_sha256, media_sha256, media_length,
				media_width, media_height,
				quoted_msg_id, quoted_msg_content, quoted_msg_sender, is_pinned, is_edited, is_view_once, is_forwarded
			  FROM messages 
			  WHERE (content LIKE ? OR caption LIKE ?)
			    AND (
			        chat_jid NOT IN (SELECT jid FROM contacts WHERE is_archived = 1)
			        AND chat_jid NOT IN (SELECT lid FROM contacts WHERE is_archived = 1 AND lid IS NOT NULL AND lid != '')
			    )
			  ORDER BY timestamp DESC LIMIT ?`
		rows, err = a.db.Query(q, searchStr, searchStr, limit)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return msgs, nil
}
