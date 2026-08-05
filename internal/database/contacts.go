package database

import (
	"database/sql"
	"strings"
	"time"
)

type Contact struct {
	JID           string
	LID           sql.NullString
	SavedName     sql.NullString
	PushName      sql.NullString
	AvatarPath    sql.NullString
	IsGroup       sql.NullBool
	LastMessageAt sql.NullTime
	UnreadCount   int
	IsPinned      bool
	IsArchived    bool
}

func (a *AppDB) SaveSyncData(jid string, unreadCount int, isPinned bool, isArchived bool, name string, pushName string, timestamp uint64) error {
	var ts interface{}
	if timestamp > 0 {
		ts = time.Unix(int64(timestamp), 0)
	}
	
	query := `INSERT INTO contacts (jid, unread_count, is_pinned, is_archived, saved_name, push_name, last_message_at) 
	          VALUES (?, ?, ?, ?, ?, ?, ?)
	          ON CONFLICT(jid) DO UPDATE SET
	          unread_count = excluded.unread_count,
	          is_pinned = excluded.is_pinned,
	          is_archived = excluded.is_archived,
	          saved_name = CASE WHEN excluded.saved_name != '' THEN excluded.saved_name ELSE contacts.saved_name END,
	          push_name = CASE WHEN excluded.push_name != '' THEN excluded.push_name ELSE contacts.push_name END,
	          last_message_at = CASE 
	              WHEN contacts.last_message_at IS NULL OR excluded.last_message_at > contacts.last_message_at 
	              THEN excluded.last_message_at 
	              ELSE contacts.last_message_at 
	          END`
	_, err := a.db.Exec(query, jid, unreadCount, isPinned, isArchived, name, pushName, ts)
	return err
}

func (a *AppDB) SaveContact(c Contact) error {
	// 1. If we are saving an LID, check if it's already mapped to a PN
	if strings.HasSuffix(c.JID, "@lid") {
		var pn string
		err := a.db.QueryRow("SELECT jid FROM contacts WHERE lid = ?", c.JID).Scan(&pn)
		if err == nil && pn != "" {
			c.LID = sql.NullString{String: c.JID, Valid: true}
			c.JID = pn
		}
	}

	query := `INSERT INTO contacts (jid, lid, saved_name, push_name, avatar_path, is_group, last_message_at) 
	          VALUES (?, ?, ?, ?, ?, ?, ?)
	          ON CONFLICT(jid) DO UPDATE SET
	          lid=COALESCE(excluded.lid, contacts.lid),
	          saved_name=CASE WHEN excluded.saved_name IS NOT NULL AND excluded.saved_name != '' THEN excluded.saved_name ELSE contacts.saved_name END,
	          push_name=CASE WHEN excluded.push_name IS NOT NULL AND excluded.push_name != '' THEN excluded.push_name ELSE contacts.push_name END,
	          avatar_path=CASE WHEN excluded.avatar_path IS NOT NULL AND excluded.avatar_path != '' THEN excluded.avatar_path ELSE contacts.avatar_path END,
	          is_group=COALESCE(excluded.is_group, contacts.is_group),
	          last_message_at=CASE 
	              WHEN contacts.last_message_at IS NULL OR excluded.last_message_at > contacts.last_message_at 
	              THEN excluded.last_message_at 
	              ELSE contacts.last_message_at 
	          END`
	_, err := a.db.Exec(query, c.JID, c.LID, c.SavedName, c.PushName, c.AvatarPath, c.IsGroup, c.LastMessageAt)
	return err
}

func (a *AppDB) MergeLID(pnJID, lidJID string) error {
	if pnJID == lidJID || pnJID == "" || lidJID == "" {
		return nil
	}

	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Ensure the PN record exists and has the LID set
	_, err = tx.Exec("INSERT INTO contacts (jid, lid) VALUES (?, ?) ON CONFLICT(jid) DO UPDATE SET lid = excluded.lid", pnJID, lidJID)
	if err != nil {
		return err
	}

	// 2. Transfer data from LID record to PN record only if LID record exists
	var dummy int
	err = tx.QueryRow("SELECT 1 FROM contacts WHERE jid = ?", lidJID).Scan(&dummy)
	if err == nil {
		_, err = tx.Exec(`
			UPDATE contacts 
			SET saved_name = COALESCE(NULLIF(saved_name, ''), (SELECT saved_name FROM contacts WHERE jid = ?)),
			    push_name = COALESCE(NULLIF(push_name, ''), (SELECT push_name FROM contacts WHERE jid = ?)),
			    last_message_at = (
			        SELECT CASE 
			            WHEN c1.last_message_at IS NULL THEN c2.last_message_at
			            WHEN c2.last_message_at IS NULL THEN c1.last_message_at
			            WHEN c1.last_message_at > c2.last_message_at THEN c1.last_message_at
			            ELSE c2.last_message_at
			        END
			        FROM contacts c1 JOIN contacts c2 ON c2.jid = ?
			        WHERE c1.jid = contacts.jid
			    )
			WHERE jid = ?`, lidJID, lidJID, lidJID, pnJID)
		if err != nil {
			return err
		}
		
		_, err = tx.Exec("UPDATE messages SET chat_jid = ? WHERE chat_jid = ?", pnJID, lidJID)
		if err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE messages SET sender_jid = ? WHERE sender_jid = ?", pnJID, lidJID)
		if err != nil {
			return err
		}
		
		// 3. Delete the duplicate LID-only record
		_, err = tx.Exec("DELETE FROM contacts WHERE jid = ?", lidJID)
		if err != nil {
			return err
		}
	}
	
	return tx.Commit()
}

func (a *AppDB) UpdateContactTimestamp(jid string, timestamp time.Time) error {
	// Update by JID or LID
	query := `UPDATE contacts SET last_message_at = ? 
	          WHERE (jid = ? OR lid = ?) 
	          AND (last_message_at IS NULL OR ? > last_message_at)`
	_, err := a.db.Exec(query, timestamp, jid, jid, timestamp)
	return err
}

func (a *AppDB) GetContact(jid string) (*Contact, error) {
	// Prioritize rows that have a name
	query := `SELECT jid, lid, saved_name, push_name, avatar_path, is_group, last_message_at, unread_count, is_pinned, is_archived 
	          FROM contacts 
	          WHERE jid = ? OR lid = ? 
	          ORDER BY (saved_name IS NOT NULL AND saved_name != '') DESC, (push_name IS NOT NULL AND push_name != '') DESC 
	          LIMIT 1`
	row := a.db.QueryRow(query, jid, jid)
	
	var c Contact
	err := row.Scan(&c.JID, &c.LID, &c.SavedName, &c.PushName, &c.AvatarPath, &c.IsGroup, &c.LastMessageAt, &c.UnreadCount, &c.IsPinned, &c.IsArchived)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (a *AppDB) GetAllContacts(profileID int64, limit int) ([]Contact, error) {
	var query string
	var rows *sql.Rows
	var err error

	if profileID > 0 {
		query = `SELECT jid, lid, saved_name, push_name, avatar_path, is_group, last_message_at, unread_count, is_pinned, is_archived 
		          FROM contacts 
		          WHERE ((jid NOT LIKE '%@lid') OR (lid IS NULL OR lid = ''))
		            AND (
		                jid IN (SELECT jid FROM profile_contacts WHERE profile_id = ?)
		                OR lid IN (SELECT jid FROM profile_contacts WHERE profile_id = ?)
		            )
		          ORDER BY is_pinned DESC, last_message_at DESC, saved_name ASC, jid ASC 
		          LIMIT ?`
		rows, err = a.db.Query(query, profileID, profileID, limit)
	} else {
		query = `SELECT jid, lid, saved_name, push_name, avatar_path, is_group, last_message_at, unread_count, is_pinned, is_archived 
		          FROM contacts 
		          WHERE (jid NOT LIKE '%@lid') OR (lid IS NULL OR lid = '')
		          ORDER BY is_pinned DESC, last_message_at DESC, saved_name ASC, jid ASC 
		          LIMIT ?`
		rows, err = a.db.Query(query, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var c Contact
		err := rows.Scan(&c.JID, &c.LID, &c.SavedName, &c.PushName, &c.AvatarPath, &c.IsGroup, &c.LastMessageAt, &c.UnreadCount, &c.IsPinned, &c.IsArchived)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, nil
}

func (a *AppDB) SearchContacts(profileID int64, term string, limit int) ([]Contact, error) {
	cleanTerm := strings.TrimSpace(term)
	if cleanTerm == "" {
		return a.GetAllContacts(profileID, limit)
	}

	pattern := "%" + cleanTerm + "%"
	prefixPattern := cleanTerm + "%"

	var query string
	var rows *sql.Rows
	var err error

	if profileID > 0 {
		query = `SELECT jid, lid, saved_name, push_name, avatar_path, is_group, last_message_at, unread_count, is_pinned, is_archived 
		          FROM contacts 
		          WHERE (IFNULL(saved_name, '') LIKE ? OR IFNULL(push_name, '') LIKE ? OR jid LIKE ?)
		          AND (jid NOT LIKE '%@lid' OR lid IS NULL OR lid = '')
		          AND (
		              jid IN (SELECT jid FROM profile_contacts WHERE profile_id = ?)
		              OR lid IN (SELECT jid FROM profile_contacts WHERE profile_id = ?)
		          )
		          ORDER BY 
		              is_pinned DESC,
		              CASE 
		                  WHEN IFNULL(saved_name, '') = ? COLLATE NOCASE THEN 1
		                  WHEN IFNULL(saved_name, '') LIKE ? THEN 2
		                  WHEN IFNULL(push_name, '') = ? COLLATE NOCASE THEN 3
		                  WHEN IFNULL(push_name, '') LIKE ? THEN 4
		                  ELSE 5 
		              END,
		              last_message_at DESC 
		          LIMIT ?`
		rows, err = a.db.Query(query, pattern, pattern, pattern, profileID, profileID, cleanTerm, prefixPattern, cleanTerm, prefixPattern, limit)
	} else {
		query = `SELECT jid, lid, saved_name, push_name, avatar_path, is_group, last_message_at, unread_count, is_pinned, is_archived 
		          FROM contacts 
		          WHERE (IFNULL(saved_name, '') LIKE ? OR IFNULL(push_name, '') LIKE ? OR jid LIKE ?)
		          AND (jid NOT LIKE '%@lid' OR lid IS NULL OR lid = '')
		          ORDER BY 
		              is_pinned DESC,
		              CASE 
		                  WHEN IFNULL(saved_name, '') = ? COLLATE NOCASE THEN 1
		                  WHEN IFNULL(saved_name, '') LIKE ? THEN 2
		                  WHEN IFNULL(push_name, '') = ? COLLATE NOCASE THEN 3
		                  WHEN IFNULL(push_name, '') LIKE ? THEN 4
		                  ELSE 5 
		              END,
		              last_message_at DESC 
		          LIMIT ?`
		rows, err = a.db.Query(query, pattern, pattern, pattern, cleanTerm, prefixPattern, cleanTerm, prefixPattern, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var c Contact
		err := rows.Scan(&c.JID, &c.LID, &c.SavedName, &c.PushName, &c.AvatarPath, &c.IsGroup, &c.LastMessageAt, &c.UnreadCount, &c.IsPinned, &c.IsArchived)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, nil
}

func (a *AppDB) GetUnresolvedPNs(limit int) ([]Contact, error) {
	query := `SELECT jid, lid, saved_name, push_name, avatar_path, is_group, last_message_at, unread_count, is_pinned, is_archived 
	          FROM contacts 
	          WHERE jid NOT LIKE '%@lid' AND lid IS NULL AND is_group = 0
	          ORDER BY last_message_at DESC 
	          LIMIT ?`
	rows, err := a.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var c Contact
		err := rows.Scan(&c.JID, &c.LID, &c.SavedName, &c.PushName, &c.AvatarPath, &c.IsGroup, &c.LastMessageAt, &c.UnreadCount, &c.IsPinned, &c.IsArchived)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, nil
}

func (a *AppDB) GetContactByLID(lid string) (*Contact, error) {
	query := `SELECT jid, lid, saved_name, push_name, avatar_path, is_group, last_message_at, unread_count, is_pinned, is_archived 
	          FROM contacts 
	          WHERE lid = ? 
	          LIMIT 1`
	row := a.db.QueryRow(query, lid)
	
	var c Contact
	err := row.Scan(&c.JID, &c.LID, &c.SavedName, &c.PushName, &c.AvatarPath, &c.IsGroup, &c.LastMessageAt, &c.UnreadCount, &c.IsPinned, &c.IsArchived)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (a *AppDB) ClearUnreadCount(jid string) error {
	_, err := a.db.Exec(`UPDATE contacts SET unread_count = 0 WHERE jid = ? OR lid = ?`, jid, jid)
	return err
}

func (c *Contact) DisplayName() string {
	if c.SavedName.Valid && c.SavedName.String != "" {
		return c.SavedName.String
	}
	if c.PushName.Valid && c.PushName.String != "" {
		return c.PushName.String
	}
	return c.JID
}
