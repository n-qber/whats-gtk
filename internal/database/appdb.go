package database

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type DBExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

type AppDB struct {
	db      *sql.DB
	hasFTS5 bool
}

func InitDB(path string) (*AppDB, error) {
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL&_sync=NORMAL&_parse_time=true", path))
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
			lid TEXT,
			saved_name TEXT,
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
			caption TEXT,
			type TEXT,
			timestamp DATETIME,
			status TEXT,
			is_from_me BOOLEAN
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_chat ON messages(chat_jid, timestamp)`,
		`CREATE TABLE IF NOT EXISTS reactions (
			msg_id TEXT,
			sender_jid TEXT,
			reaction TEXT,
			timestamp DATETIME,
			PRIMARY KEY (msg_id, sender_jid)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_reactions_msg ON reactions(msg_id)`,
		`CREATE TABLE IF NOT EXISTS poll_options (
			msg_id TEXT,
			option_hash TEXT,
			option_name TEXT,
			PRIMARY KEY (msg_id, option_hash)
		)`,
		`CREATE TABLE IF NOT EXISTS poll_votes (
			msg_id TEXT,
			option_hash TEXT,
			sender_jid TEXT,
			PRIMARY KEY (msg_id, option_hash, sender_jid)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_poll_options_msg ON poll_options(msg_id)`,
		`CREATE INDEX IF NOT EXISTS idx_poll_votes_msg ON poll_votes(msg_id)`,
		`CREATE TABLE IF NOT EXISTS message_receipts (
			msg_id TEXT,
			chat_jid TEXT,
			user_jid TEXT,
			receipt_type TEXT,
			timestamp DATETIME,
			PRIMARY KEY (msg_id, user_jid, receipt_type)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_message_receipts_msg ON message_receipts(msg_id)`,
		`CREATE TABLE IF NOT EXISTS profiles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS profile_contacts (
			profile_id INTEGER NOT NULL,
			jid TEXT NOT NULL,
			PRIMARY KEY (profile_id, jid),
			FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value TEXT
		)`,
	}

	for _, q := range queries {
		if _, err := a.db.Exec(q); err != nil {
			return fmt.Errorf("failed to create table/index: %w", err)
		}
	}
	
	// Migration logic: Add columns if missing
	a.ensureColumn("contacts", "lid", "TEXT")
	a.ensureColumn("contacts", "saved_name", "TEXT")
	a.ensureColumn("contacts", "push_name", "TEXT")
	a.ensureColumn("contacts", "last_message_at", "DATETIME")
	a.ensureColumn("contacts", "unread_count", "INTEGER DEFAULT 0")
	a.ensureColumn("contacts", "is_pinned", "BOOLEAN DEFAULT 0")
	a.ensureColumn("contacts", "is_archived", "BOOLEAN DEFAULT 0")
	a.ensureColumn("messages", "thumbnail", "BLOB")
	a.ensureColumn("messages", "media_url", "TEXT")
	a.ensureColumn("messages", "media_direct_path", "TEXT")
	a.ensureColumn("messages", "media_key", "BLOB")
	a.ensureColumn("messages", "media_mimetype", "TEXT")
	a.ensureColumn("messages", "media_enc_sha256", "BLOB")
	a.ensureColumn("messages", "media_sha256", "BLOB")
	a.ensureColumn("messages", "media_length", "INTEGER")
	a.ensureColumn("messages", "media_width", "INTEGER")
	a.ensureColumn("messages", "media_height", "INTEGER")
	a.ensureColumn("messages", "caption", "TEXT")
	a.ensureColumn("messages", "quoted_msg_id", "TEXT")
	a.ensureColumn("messages", "quoted_msg_content", "TEXT")
	a.ensureColumn("messages", "quoted_msg_sender", "TEXT")
	a.ensureColumn("messages", "is_pinned", "BOOLEAN DEFAULT 0")
	a.ensureColumn("messages", "is_edited", "BOOLEAN DEFAULT 0")
	a.ensureColumn("messages", "is_view_once", "BOOLEAN DEFAULT 0")
	a.ensureColumn("messages", "is_forwarded", "BOOLEAN DEFAULT 0")

	// Create unique index for lid to handle mapping and prevent duplicates
	_, _ = a.db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_contacts_lid ON contacts(lid) WHERE lid IS NOT NULL")

	// Initialize FTS5 virtual table for full-text message search
	_, err := a.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
			msg_id UNINDEXED,
			chat_jid UNINDEXED,
			content,
			caption,
			tokenize='unicode61'
		);
	`)
	if err == nil {
		a.hasFTS5 = true
		triggers := []string{
			`CREATE TRIGGER IF NOT EXISTS trg_messages_ai AFTER INSERT ON messages BEGIN
				INSERT INTO messages_fts(msg_id, chat_jid, content, caption)
				VALUES (new.msg_id, new.chat_jid, coalesce(new.content, ''), coalesce(new.caption, ''));
			END;`,
			`CREATE TRIGGER IF NOT EXISTS trg_messages_ad AFTER DELETE ON messages BEGIN
				DELETE FROM messages_fts WHERE msg_id = old.msg_id;
			END;`,
			`CREATE TRIGGER IF NOT EXISTS trg_messages_au AFTER UPDATE ON messages BEGIN
				DELETE FROM messages_fts WHERE msg_id = old.msg_id;
				INSERT INTO messages_fts(msg_id, chat_jid, content, caption)
				VALUES (new.msg_id, new.chat_jid, coalesce(new.content, ''), coalesce(new.caption, ''));
			END;`,
		}
		for _, trg := range triggers {
			_, _ = a.db.Exec(trg)
		}
		// Populate existing messages into messages_fts if missing
		_, _ = a.db.Exec(`
			INSERT INTO messages_fts(msg_id, chat_jid, content, caption)
			SELECT msg_id, chat_jid, coalesce(content, ''), coalesce(caption, '')
			FROM messages
			WHERE msg_id NOT IN (SELECT msg_id FROM messages_fts);
		`)
	}

	return nil
}

func (a *AppDB) ensureColumn(table, column, colType string) {
	query := fmt.Sprintf("SELECT %s FROM %s LIMIT 1", column, table)
	_, err := a.db.Exec(query)
	if err != nil {
		fmt.Printf("Database: Adding missing column %s to table %s\n", column, table)
		alter := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, colType)
		if _, err := a.db.Exec(alter); err != nil {
			fmt.Printf("Database: Failed to add column %s: %v\n", column, err)
		}
	}
}

// Begin starts a new database transaction.
func (a *AppDB) Begin() (*sql.Tx, error) {
	return a.db.Begin()
}

func (a *AppDB) Close() error {
	return a.db.Close()
}
