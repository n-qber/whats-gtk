package database

import (
	"os"
	"testing"
	"time"
)

func TestFTS5Search(t *testing.T) {
	dbPath := "test_fts5.db"
	defer os.Remove(dbPath)

	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}
	defer db.Close()

	// Insert test messages
	m1 := Message{
		ID:        "msg_1",
		ChatJID:   "user1@s.whatsapp.net",
		SenderJID: "user1@s.whatsapp.net",
		Content:   "Hey, are you free for the meeting tomorrow?",
		Type:      "text",
		Timestamp: time.Now().Add(-10 * time.Minute),
		Status:    "delivered",
	}
	m2 := Message{
		ID:        "msg_2",
		ChatJID:   "user1@s.whatsapp.net",
		SenderJID: "user1@s.whatsapp.net",
		Content:   "Let's review the quarterly report and budget numbers.",
		Type:      "text",
		Timestamp: time.Now().Add(-5 * time.Minute),
		Status:    "delivered",
	}
	m3 := Message{
		ID:        "msg_3",
		ChatJID:   "user2@s.whatsapp.net",
		SenderJID: "user2@s.whatsapp.net",
		Content:   "Unrelated chit chat about weather.",
		Type:      "text",
		Timestamp: time.Now(),
		Status:    "delivered",
	}

	if err := db.SaveMessage(m1); err != nil {
		t.Fatalf("Failed to save m1: %v", err)
	}
	if err := db.SaveMessage(m2); err != nil {
		t.Fatalf("Failed to save m2: %v", err)
	}
	if err := db.SaveMessage(m3); err != nil {
		t.Fatalf("Failed to save m3: %v", err)
	}

	// Search in specific chat
	results, err := db.SearchMessagesInChat([]string{"user1@s.whatsapp.net"}, "meeting", 10)
	if err != nil {
		t.Fatalf("SearchMessagesInChat failed: %v", err)
	}
	if len(results) != 1 || results[0].ID != "msg_1" {
		t.Fatalf("Expected 1 result with msg_1, got %v", results)
	}

	// Search across all chats
	resultsAll, err := db.SearchMessages(0, "quarterly", 10)
	if err != nil {
		t.Fatalf("SearchMessages failed: %v", err)
	}
	if len(resultsAll) != 1 || resultsAll[0].ID != "msg_2" {
		t.Fatalf("Expected 1 result with msg_2, got %v", resultsAll)
	}
}
