package database

import (
	"time"
)

type Contact struct {
	JID           string
	Name          string
	PushName      string
	AvatarPath    string
	IsGroup       bool
	LastMessageAt time.Time
}

func (a *AppDB) SaveContact(c Contact) error {
	query := `INSERT INTO contacts (jid, name, push_name, avatar_path, is_group, last_message_at) 
	          VALUES (?, ?, ?, ?, ?, ?)
	          ON CONFLICT(jid) DO UPDATE SET
	          name=COALESCE(excluded.name, name),
	          push_name=COALESCE(excluded.push_name, push_name),
	          is_group=excluded.is_group,
	          last_message_at=MAX(COALESCE(last_message_at, '0001-01-01'), excluded.last_message_at)`
	_, err := a.db.Exec(query, c.JID, c.Name, c.PushName, c.AvatarPath, c.IsGroup, c.LastMessageAt)
	return err
}

func (a *AppDB) UpdateContactTimestamp(jid string, timestamp time.Time) error {
	query := `UPDATE contacts SET last_message_at = MAX(COALESCE(last_message_at, '0001-01-01'), ?) WHERE jid = ?`
	_, err := a.db.Exec(query, timestamp, jid)
	return err
}

func (a *AppDB) GetAllContacts() ([]Contact, error) {
	query := `SELECT jid, name, push_name, avatar_path, is_group, last_message_at FROM contacts ORDER BY last_message_at DESC`
	rows, err := a.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var c Contact
		err := rows.Scan(&c.JID, &c.Name, &c.PushName, &c.AvatarPath, &c.IsGroup, &c.LastMessageAt)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, nil
}
