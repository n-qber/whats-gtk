package notifications

import (
	"testing"
)

func TestNotifierDNDAndSoundState(t *testing.T) {
	n := &Notifier{
		soundEnabled: true,
		dndEnabled:   false,
		chatMap:      make(map[uint32]string),
		dndSubs:      make(map[int]func(bool)),
		soundSubs:    make(map[int]func(bool)),
	}

	if n.IsDNDEnabled() {
		t.Errorf("Expected DND to be disabled by default")
	}
	if !n.IsSoundEnabled() {
		t.Errorf("Expected Sound to be enabled by default")
	}

	// Test DND listener
	dndChanges := []bool{}
	unsubDND := n.OnDNDChange(func(enabled bool) {
		dndChanges = append(dndChanges, enabled)
	})

	n.SetDNDEnabled(true)
	if !n.IsDNDEnabled() {
		t.Errorf("Expected DND to be enabled")
	}
	// Calling SetDNDEnabled with same value should not trigger listener again
	n.SetDNDEnabled(true)

	n.SetDNDEnabled(false)
	if n.IsDNDEnabled() {
		t.Errorf("Expected DND to be disabled")
	}

	if len(dndChanges) != 2 || dndChanges[0] != true || dndChanges[1] != false {
		t.Errorf("Unexpected DND changes: %v", dndChanges)
	}

	// Unsubscribe DND
	unsubDND()
	n.SetDNDEnabled(true)
	if len(dndChanges) != 2 {
		t.Errorf("Expected no more notifications after unsubscribe, got %d", len(dndChanges))
	}

	// Test Sound listener
	soundChanges := []bool{}
	unsubSound := n.OnSoundChange(func(enabled bool) {
		soundChanges = append(soundChanges, enabled)
	})

	n.SetSoundEnabled(false)
	if n.IsSoundEnabled() {
		t.Errorf("Expected Sound to be disabled")
	}
	n.SetSoundEnabled(false) // duplicate call
	n.SetSoundEnabled(true)
	if !n.IsSoundEnabled() {
		t.Errorf("Expected Sound to be enabled")
	}

	if len(soundChanges) != 2 || soundChanges[0] != false || soundChanges[1] != true {
		t.Errorf("Unexpected Sound changes: %v", soundChanges)
	}

	unsubSound()
	n.SetSoundEnabled(false)
	if len(soundChanges) != 2 {
		t.Errorf("Expected no more sound notifications after unsubscribe, got %d", len(soundChanges))
	}
}
