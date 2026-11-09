package bridge

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestGroupPendingReads(t *testing.T) {
	chatA := types.NewJID("111", types.DefaultUserServer)
	senderA := types.NewJID("222", types.DefaultUserServer)
	chatB := types.NewJID("333", types.DefaultUserServer)

	first := time.Unix(100, 0)
	second := time.Unix(200, 0)

	pending := []pendingRead{
		{chat: chatA, sender: senderA, ids: []string{"a"}, timestamp: first},
		{chat: chatA, sender: senderA, ids: []string{"b"}, timestamp: second},
		{chat: chatB, ids: []string{"c"}, timestamp: first},
	}

	groups := groupPendingReads(pending)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	if groups[0].chat != chatA || groups[0].sender != senderA {
		t.Fatalf("unexpected first group target: %v/%v", groups[0].chat, groups[0].sender)
	}
	if len(groups[0].ids) != 2 || groups[0].ids[0] != "a" || groups[0].ids[1] != "b" {
		t.Fatalf("unexpected first group ids: %v", groups[0].ids)
	}
	if !groups[0].timestamp.Equal(second) {
		t.Fatalf("expected newest timestamp %v, got %v", second, groups[0].timestamp)
	}

	if groups[1].chat != chatB || len(groups[1].ids) != 1 || groups[1].ids[0] != "c" {
		t.Fatalf("unexpected second group: %+v", groups[1])
	}
}
