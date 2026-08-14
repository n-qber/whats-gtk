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
