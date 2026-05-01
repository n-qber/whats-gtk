package database

type Contact struct {
	JID        string
	Name       string
	PushName   string
	AvatarPath string
	IsGroup    bool
}

func (a *AppDB) SaveContact(c Contact) error {
	query := `INSERT OR REPLACE INTO contacts (jid, name, push_name, avatar_path, is_group) VALUES (?, ?, ?, ?, ?)`
	_, err := a.db.Exec(query, c.JID, c.Name, c.PushName, c.AvatarPath, c.IsGroup)
	return err
}

func (a *AppDB) GetAllContacts() ([]Contact, error) {
	query := `SELECT jid, name, push_name, avatar_path, is_group FROM contacts`
	rows, err := a.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var c Contact
		err := rows.Scan(&c.JID, &c.Name, &c.PushName, &c.AvatarPath, &c.IsGroup)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, nil
}
