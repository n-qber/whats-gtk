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
	PushName      string
	AvatarPath    string
	IsGroup       bool
	LastMessageAt time.Time
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
	          push_name=CASE WHEN excluded.push_name != '' THEN excluded.push_name ELSE contacts.push_name END,
	          avatar_path=CASE WHEN excluded.avatar_path != '' THEN excluded.avatar_path ELSE contacts.avatar_path END,
	          is_group=excluded.is_group,
	          last_message_at=MAX(COALESCE(last_message_at, '0001-01-01'), excluded.last_message_at)`
	_, err := a.db.Exec(query, c.JID, c.LID, c.SavedName, c.PushName, c.AvatarPath, c.IsGroup, c.LastMessageAt)
	return err
}

// MergeLID links a phone number JID with an LID and removes the duplicate LID entry if it exists
func (a *AppDB) MergeLID(pnJID, lidJID string) error {
	if pnJID == lidJID || pnJID == "" || lidJID == "" {
		return nil
	}

	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Update the PN record with the LID
	_, err = tx.Exec("UPDATE contacts SET lid = ? WHERE jid = ?", lidJID, pnJID)
	if err != nil {
		return err
	}

	// 2. Transfer names, messages and timestamps from LID record to PN record
	_, err = tx.Exec(`
		UPDATE contacts 
		SET saved_name = COALESCE(NULLIF(saved_name, ''), (SELECT saved_name FROM contacts WHERE jid = ?)),
		    push_name = COALESCE(NULLIF(push_name, ''), (SELECT push_name FROM contacts WHERE jid = ?)),
		    last_message_at = MAX(COALESCE(last_message_at, '0001-01-01'), 
		                          (SELECT COALESCE(last_message_at, '0001-01-01') FROM contacts WHERE jid = ?))
		WHERE jid = ?`, lidJID, lidJID, lidJID, pnJID)
	
	_, err = tx.Exec("UPDATE messages SET chat_jid = ? WHERE chat_jid = ?", pnJID, lidJID)
	_, err = tx.Exec("UPDATE messages SET sender_jid = ? WHERE sender_jid = ?", pnJID, lidJID)
	
	// 3. Delete the duplicate LID-only record
	_, err = tx.Exec("DELETE FROM contacts WHERE jid = ? AND jid != ?", lidJID, pnJID)
	
	return tx.Commit()
}

func (a *AppDB) UpdateContactTimestamp(jid string, timestamp time.Time) error {
	// Update by JID or LID
	query := `UPDATE contacts SET last_message_at = MAX(COALESCE(last_message_at, '0001-01-01'), ?) WHERE jid = ? OR lid = ?`
	_, err := a.db.Exec(query, timestamp, jid, jid)
	return err
}

func (a *AppDB) GetContact(jid string) (*Contact, error) {
	// Prioritize rows that have a name
	query := `SELECT jid, lid, saved_name, push_name, avatar_path, is_group, last_message_at 
	          FROM contacts 
	          WHERE jid = ? OR lid = ? 
	          ORDER BY (saved_name IS NOT NULL AND saved_name != '') DESC, (push_name != '') DESC 
	          LIMIT 1`
	row := a.db.QueryRow(query, jid, jid)
	
	var c Contact
	err := row.Scan(&c.JID, &c.LID, &c.SavedName, &c.PushName, &c.AvatarPath, &c.IsGroup, &c.LastMessageAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (a *AppDB) GetAllContacts(limit int) ([]Contact, error) {
	// Hide raw LIDs that are already mapped to a PN
	query := `SELECT jid, lid, saved_name, push_name, avatar_path, is_group, last_message_at 
	          FROM contacts 
	          WHERE jid NOT LIKE '%@lid' OR lid IS NULL
	          ORDER BY last_message_at DESC, saved_name ASC, jid ASC 
	          LIMIT ?`
	rows, err := a.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var c Contact
		err := rows.Scan(&c.JID, &c.LID, &c.SavedName, &c.PushName, &c.AvatarPath, &c.IsGroup, &c.LastMessageAt)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, nil
}

func (a *AppDB) SearchContacts(term string, limit int) ([]Contact, error) {
	query := `SELECT jid, lid, saved_name, push_name, avatar_path, is_group, last_message_at 
	          FROM contacts 
	          WHERE (saved_name LIKE ? OR push_name LIKE ? OR jid LIKE ?)
	          AND (jid NOT LIKE '%@lid' OR lid IS NULL)
	          ORDER BY last_message_at DESC, saved_name ASC, jid ASC 
	          LIMIT ?`
	pattern := "%" + term + "%"
	rows, err := a.db.Query(query, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var c Contact
		err := rows.Scan(&c.JID, &c.LID, &c.SavedName, &c.PushName, &c.AvatarPath, &c.IsGroup, &c.LastMessageAt)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, nil
}

func (a *AppDB) GetUnresolvedPNs(limit int) ([]Contact, error) {
	query := `SELECT jid, lid, saved_name, push_name, avatar_path, is_group, last_message_at 
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
		err := rows.Scan(&c.JID, &c.LID, &c.SavedName, &c.PushName, &c.AvatarPath, &c.IsGroup, &c.LastMessageAt)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, nil
}

func (c *Contact) DisplayName() string {
	if c.SavedName.Valid && c.SavedName.String != "" {
		return c.SavedName.String
	}
	if c.PushName != "" {
		return c.PushName
	}
	return c.JID
}
