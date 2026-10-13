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

	// Test GetLastMessagesMap
	lastMsgs := db.GetLastMessagesMap()
	if lastMsgs["user1@s.whatsapp.net"] != "Let's review the quarterly report and budget numbers." {
		t.Errorf("Expected latest message for user1, got %q", lastMsgs["user1@s.whatsapp.net"])
	}
	if lastMsgs["user2@s.whatsapp.net"] != "Unrelated chit chat about weather." {
		t.Errorf("Expected latest message for user2, got %q", lastMsgs["user2@s.whatsapp.net"])
	}
}

func TestFormatMessagePreview(t *testing.T) {
	tests := []struct {
		content  string
		caption  string
		mType    string
		expected string
	}{
		{"Hello world", "", "text", "Hello world"},
		{"Line 1\nLine 2", "", "text", "Line 1 Line 2"},
		{"", "Check this out", "image", "📷 Check this out"},
		{"", "", "image", "📷 Foto"},
		{"", "Vacation", "video", "🎥 Vacation"},
		{"", "", "video", "🎥 Vídeo"},
		{"", "", "audio", "🎵 Áudio"},
		{"", "", "sticker", "Figurinha"},
		{"", "Contract.pdf", "document", "📄 Contract.pdf"},
		{"", "", "document", "📄 Documento"},
		{"", "", "revoked", "🚫 Mensagem apagada"},
		{"This is a very long text message that is definitely going to exceed sixty unicode characters in length to test truncation", "", "text", "This is a very long text message that is definitely going to..."},
	}

	for _, tc := range tests {
		actual := FormatMessagePreview(tc.content, tc.caption, tc.mType)
		if actual != tc.expected {
			t.Errorf("FormatMessagePreview(%q, %q, %q) = %q; expected %q", tc.content, tc.caption, tc.mType, actual, tc.expected)
		}
	}
}

