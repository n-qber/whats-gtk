package database

import (
	"database/sql"
	"os"
	"testing"
	"time"
)

func TestMergeLID(t *testing.T) {
	dbPath := "test_merge_lid.db"
	defer os.Remove(dbPath)

	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}
	defer db.Close()

	pnJID := "5519999999999@s.whatsapp.net"
	lidJID := "1029384756102@lid"

	// 1. Create a contact under LID JID
	c1 := Contact{
		JID:       lidJID,
		SavedName: sql.NullString{String: "Alice LID", Valid: true},
		PushName:  sql.NullString{String: "Alice", Valid: true},
	}
	if err := db.SaveContact(c1); err != nil {
		t.Fatalf("Failed to save LID contact: %v", err)
	}

	// 2. Save a message under LID JID
	m1 := Message{
		ID:        "msg_123",
		ChatJID:   lidJID,
		SenderJID: lidJID,
		Content:   "Hello from LID",
		Type:      "text",
		Timestamp: time.Now(),
		Status:    "sent",
	}
	if err := db.SaveMessage(m1); err != nil {
		t.Fatalf("Failed to save LID message: %v", err)
	}

	// 3. Merge LID into PN JID
	if err := db.MergeLID(pnJID, lidJID); err != nil {
		t.Fatalf("MergeLID failed: %v", err)
	}

	// 4. Verify original LID contact is gone and merged into PN JID contact
	contact, err := db.GetContact(pnJID)
	if err != nil {
		t.Fatalf("Failed to get merged contact: %v", err)
	}
	if contact.JID != pnJID {
		t.Errorf("Expected contact JID %s, got %s", pnJID, contact.JID)
	}
	if !contact.LID.Valid || contact.LID.String != lidJID {
		t.Errorf("Expected LID %s, got %v", lidJID, contact.LID)
	}

	// 5. Verify message chat_jid and sender_jid were updated to PN JID
	msg, err := db.GetMessage("msg_123")
	if err != nil {
		t.Fatalf("Failed to get message after MergeLID: %v", err)
	}
	if msg.ChatJID != pnJID {
		t.Errorf("Expected message ChatJID %s, got %s", pnJID, msg.ChatJID)
	}
	if msg.SenderJID != pnJID {
		t.Errorf("Expected message SenderJID %s, got %s", pnJID, msg.SenderJID)
	}

	// 6. Test duplicate message merge safety
	m2 := Message{
		ID:        "msg_123",
		ChatJID:   lidJID,
		SenderJID: lidJID,
		Content:   "Duplicate message",
		Type:      "text",
		Timestamp: time.Now(),
	}
	_ = db.SaveMessage(m2)
	if err := db.MergeLID(pnJID, lidJID); err != nil {
		t.Errorf("MergeLID with duplicate message failed: %v", err)
	}
}

func TestContactMute(t *testing.T) {
	dbPath := "test_contact_mute.db"
	defer os.Remove(dbPath)

	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}
	defer db.Close()

	jid := "5519888888888@s.whatsapp.net"
	c := Contact{
		JID:       jid,
		SavedName: sql.NullString{String: "Bob", Valid: true},
	}
	if err := db.SaveContact(c); err != nil {
		t.Fatalf("Failed to save contact: %v", err)
	}

	contact, err := db.GetContact(jid)
	if err != nil {
		t.Fatalf("Failed to get contact: %v", err)
	}
	if contact.IsMuted() {
		t.Errorf("Expected contact not to be muted initially")
	}

	// 1. Mute 8 hours in future
	future := time.Now().Add(8 * time.Hour)
	if err := db.SetContactMuted(jid, future); err != nil {
		t.Fatalf("Failed to set contact muted: %v", err)
	}
	contact, err = db.GetContact(jid)
	if err != nil {
		t.Fatalf("Failed to get contact: %v", err)
	}
	if !contact.IsMuted() {
		t.Errorf("Expected contact to be muted")
	}

	// 2. Mute in past (expired)
	past := time.Now().Add(-1 * time.Hour)
	if err := db.SetContactMuted(jid, past); err != nil {
		t.Fatalf("Failed to set contact muted: %v", err)
	}
	contact, err = db.GetContact(jid)
	if err != nil {
		t.Fatalf("Failed to get contact: %v", err)
	}
	if contact.IsMuted() {
		t.Errorf("Expected expired muted contact to not be muted")
	}

	// 3. Unmute (zero time)
	if err := db.SetContactMuted(jid, time.Time{}); err != nil {
		t.Fatalf("Failed to unmute contact: %v", err)
	}
	contact, err = db.GetContact(jid)
	if err != nil {
		t.Fatalf("Failed to get contact: %v", err)
	}
	if contact.IsMuted() {
		t.Errorf("Expected unmuted contact to not be muted")
	}

	// 4. SaveSyncData with mute
	if err := db.SaveSyncData(jid, 0, false, false, future, "Bob", "", uint64(time.Now().Unix())); err != nil {
		t.Fatalf("Failed to SaveSyncData: %v", err)
	}
	contact, err = db.GetContact(jid)
	if err != nil {
		t.Fatalf("Failed to get contact: %v", err)
	}
	if !contact.IsMuted() {
		t.Errorf("Expected contact to be muted after SaveSyncData")
	}
}

func TestAppSettings(t *testing.T) {
	dbPath := "test_app_settings.db"
	defer os.Remove(dbPath)

	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}
	defer db.Close()

	// Setting doesn't exist yet
	_, err = db.GetSetting("dnd_enabled")
	if err == nil {
		t.Errorf("Expected error for non-existent setting")
	}

	// Set setting
	if err := db.SetSetting("dnd_enabled", "true"); err != nil {
		t.Fatalf("Failed to set setting: %v", err)
	}

	val, err := db.GetSetting("dnd_enabled")
	if err != nil {
		t.Fatalf("Failed to get setting: %v", err)
	}
	if val != "true" {
		t.Errorf("Expected 'true', got %s", val)
	}

	// Update setting
	if err := db.SetSetting("dnd_enabled", "false"); err != nil {
		t.Fatalf("Failed to update setting: %v", err)
	}
	val, err = db.GetSetting("dnd_enabled")
	if err != nil {
		t.Fatalf("Failed to get updated setting: %v", err)
	}
	if val != "false" {
		t.Errorf("Expected 'false', got %s", val)
	}
}

