package database

import (
	"database/sql"
	"fmt"
	"time"
)

const (
	ProfileArchivedID int64 = -1
	ProfileAllChatsID int64 = 0
)

type Profile struct {
	ID           int64
	Name         string
	CreatedAt    time.Time
	ContactCount int
}

func (a *AppDB) CreateProfile(name string) (*Profile, error) {
	res, err := a.db.Exec("INSERT INTO profiles (name) VALUES (?)", name)
	if err != nil {
		return nil, fmt.Errorf("failed to create profile: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Profile{
		ID:        id,
		Name:      name,
		CreatedAt: time.Now(),
	}, nil
}

func (a *AppDB) GetProfiles() ([]Profile, error) {
	query := `
		SELECT p.id, p.name, p.created_at, COUNT(pc.jid) as contact_count
		FROM profiles p
		LEFT JOIN profile_contacts pc ON p.id = pc.profile_id
		GROUP BY p.id, p.name, p.created_at
		ORDER BY p.name ASC
	`
	rows, err := a.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get profiles: %w", err)
	}
	defer rows.Close()

	var profiles []Profile
	for rows.Next() {
		var p Profile
		var createdAt sql.NullTime
		if err := rows.Scan(&p.ID, &p.Name, &createdAt, &p.ContactCount); err != nil {
			return nil, err
		}
		if createdAt.Valid {
			p.CreatedAt = createdAt.Time
		}
		profiles = append(profiles, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed during profiles iteration: %w", err)
	}
	return profiles, nil
}

func (a *AppDB) GetProfile(id int64) (*Profile, error) {
	query := `
		SELECT p.id, p.name, p.created_at, COUNT(pc.jid) as contact_count
		FROM profiles p
		LEFT JOIN profile_contacts pc ON p.id = pc.profile_id
		WHERE p.id = ?
		GROUP BY p.id, p.name, p.created_at
	`
	row := a.db.QueryRow(query, id)
	var p Profile
	var createdAt sql.NullTime
	if err := row.Scan(&p.ID, &p.Name, &createdAt, &p.ContactCount); err != nil {
		return nil, err
	}
	if createdAt.Valid {
		p.CreatedAt = createdAt.Time
	}
	return &p, nil
}

func (a *AppDB) UpdateProfileName(id int64, name string) error {
	_, err := a.db.Exec("UPDATE profiles SET name = ? WHERE id = ?", name, id)
	return err
}

func (a *AppDB) DeleteProfile(id int64) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("DELETE FROM profile_contacts WHERE profile_id = ?", id)
	if err != nil {
		return err
	}
	_, err = tx.Exec("DELETE FROM profiles WHERE id = ?", id)
	if err != nil {
		return err
	}

	// If the deleted profile was active, reset active profile setting to 0
	activeID, _ := a.GetActiveProfileID()
	if activeID == id {
		_, _ = tx.Exec("INSERT INTO app_settings (key, value) VALUES ('active_profile_id', '0') ON CONFLICT(key) DO UPDATE SET value = '0'")
	}

	return tx.Commit()
}

func (a *AppDB) AddContactToProfile(profileID int64, jid string) error {
	_, err := a.db.Exec("INSERT OR IGNORE INTO profile_contacts (profile_id, jid) VALUES (?, ?)", profileID, jid)
	return err
}

func (a *AppDB) RemoveContactFromProfile(profileID int64, jid string) error {
	_, err := a.db.Exec("DELETE FROM profile_contacts WHERE profile_id = ? AND jid = ?", profileID, jid)
	return err
}

func (a *AppDB) SetContactInProfile(profileID int64, jid string, inProfile bool) error {
	if inProfile {
		return a.AddContactToProfile(profileID, jid)
	}
	return a.RemoveContactFromProfile(profileID, jid)
}

func (a *AppDB) GetProfileJIDs(profileID int64) ([]string, error) {
	rows, err := a.db.Query("SELECT jid FROM profile_contacts WHERE profile_id = ?", profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jids []string
	for rows.Next() {
		var jid string
		if err := rows.Scan(&jid); err == nil {
			jids = append(jids, jid)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return jids, nil
}

func (a *AppDB) GetProfileJIDMap(profileID int64) (map[string]bool, error) {
	jids, err := a.GetProfileJIDs(profileID)
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool)
	for _, j := range jids {
		m[j] = true
	}
	return m, nil
}

func (a *AppDB) GetActiveProfileID() (int64, error) {
	var val string
	err := a.db.QueryRow("SELECT value FROM app_settings WHERE key = 'active_profile_id'").Scan(&val)
	if err != nil {
		return 0, nil
	}
	var id int64
	fmt.Sscanf(val, "%d", &id)
	return id, nil
}

func (a *AppDB) SetActiveProfileID(profileID int64) error {
	val := fmt.Sprintf("%d", profileID)
	_, err := a.db.Exec("INSERT INTO app_settings (key, value) VALUES ('active_profile_id', ?) ON CONFLICT(key) DO UPDATE SET value = ?", val, val)
	return err
}

func (a *AppDB) GetArchivedContactsCount() (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM contacts 
	          WHERE ((jid NOT LIKE '%@lid') OR (lid IS NULL OR lid = ''))
	            AND is_archived = 1`
	err := a.db.QueryRow(query).Scan(&count)
	return count, err
}
