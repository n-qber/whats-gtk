package database

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type AppDB struct {
	db *sql.DB
}

func InitDB(path string) (*AppDB, error) {
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL&_sync=NORMAL", path))
	if err != nil {
		return nil, fmt.Errorf("failed to open app db: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping app db: %w", err)
	}

	appDB := &AppDB{db: db}
	if err := appDB.createTables(); err != nil {
		return nil, err
	}

	return appDB, nil
}

func (a *AppDB) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS contacts (
			jid TEXT PRIMARY KEY,
			name TEXT,
			push_name TEXT,
			avatar_path TEXT,
			is_group BOOLEAN,
			last_message_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			msg_id TEXT PRIMARY KEY,
			chat_jid TEXT,
			sender_jid TEXT,
			content TEXT,
			type TEXT,
			timestamp DATETIME,
			status TEXT,
			is_from_me BOOLEAN
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_chat ON messages(chat_jid, timestamp)`,
	}

	for _, q := range queries {
		if _, err := a.db.Exec(q); err != nil {
			// If it fails because the column is missing, try adding it
			if q == queries[0] {
				_, _ = a.db.Exec("ALTER TABLE contacts ADD COLUMN last_message_at DATETIME")
			} else {
				return fmt.Errorf("failed to create table/index: %w", err)
			}
		}
	}
	
	// Double check contacts table has last_message_at
	_, err := a.db.Exec("SELECT last_message_at FROM contacts LIMIT 1")
	if err != nil {
		_, _ = a.db.Exec("ALTER TABLE contacts ADD COLUMN last_message_at DATETIME")
	}

	return nil
}

func (a *AppDB) Close() error {
	return a.db.Close()
}
