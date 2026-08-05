package database

import (
	"database/sql"
	"os"
	"testing"
)

func TestProfilesDB(t *testing.T) {
	dbPath := "test_profiles.db"
	defer os.Remove(dbPath)

	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}
	defer db.Close()

	// 1. Initial Active Profile should be 0 (All Chats)
	activeID, err := db.GetActiveProfileID()
	if err != nil {
		t.Fatalf("Failed to get initial active profile: %v", err)
	}
	if activeID != 0 {
		t.Errorf("Expected initial active profile 0, got %d", activeID)
	}

	// 2. Create contacts
	c1 := Contact{JID: "5516999990001@s.whatsapp.net", SavedName: sql.NullString{String: "Member 1", Valid: true}}
	c2 := Contact{JID: "5516999990002@s.whatsapp.net", SavedName: sql.NullString{String: "Member 2", Valid: true}}
	c3 := Contact{JID: "1203630000000@g.us", SavedName: sql.NullString{String: "Bateria Group", Valid: true}, IsGroup: sql.NullBool{Bool: true, Valid: true}}

	_ = db.SaveContact(c1)
	_ = db.SaveContact(c2)
	_ = db.SaveContact(c3)

	// GetAllContacts with profile 0 should return all 3 contacts
	all, err := db.GetAllContacts(0, 100)
	if err != nil {
		t.Fatalf("GetAllContacts failed: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("Expected 3 contacts for profile 0, got %d", len(all))
	}

	// 3. Create a profile "Bateria UFSCar"
	prof, err := db.CreateProfile("Bateria UFSCar")
	if err != nil {
		t.Fatalf("Failed to create profile: %v", err)
	}
	if prof.Name != "Bateria UFSCar" {
		t.Errorf("Expected profile name 'Bateria UFSCar', got '%s'", prof.Name)
	}

	// 4. Add c1 and c3 (group) to "Bateria UFSCar" profile
	err = db.AddContactToProfile(prof.ID, c1.JID)
	if err != nil {
		t.Fatalf("Failed to add contact c1 to profile: %v", err)
	}
	err = db.AddContactToProfile(prof.ID, c3.JID)
	if err != nil {
		t.Fatalf("Failed to add contact c3 to profile: %v", err)
	}

	// 5. Query contacts for profile prof.ID
	filtered, err := db.GetAllContacts(prof.ID, 100)
	if err != nil {
		t.Fatalf("Failed to get contacts for profile %d: %v", prof.ID, err)
	}
	if len(filtered) != 2 {
		t.Fatalf("Expected 2 contacts in profile, got %d", len(filtered))
	}

	// 6. Test active profile setting
	err = db.SetActiveProfileID(prof.ID)
	if err != nil {
		t.Fatalf("Failed to set active profile: %v", err)
	}
	activeID, err = db.GetActiveProfileID()
	if err != nil || activeID != prof.ID {
		t.Fatalf("Expected active profile %d, got %d (err: %v)", prof.ID, activeID, err)
	}

	// 7. Test SearchContacts with profile filtering
	searchRes, err := db.SearchContacts(prof.ID, "Member", 100)
	if err != nil {
		t.Fatalf("SearchContacts failed: %v", err)
	}
	if len(searchRes) != 1 || searchRes[0].JID != c1.JID {
		t.Errorf("Expected 1 result for 'Member' in profile (c1), got %d", len(searchRes))
	}

	// Search for Member 2 (which is NOT in profile) should return 0 results
	searchRes2, err := db.SearchContacts(prof.ID, "Member 2", 100)
	if err != nil {
		t.Fatalf("SearchContacts failed: %v", err)
	}
	if len(searchRes2) != 0 {
		t.Errorf("Expected 0 results for 'Member 2' in profile, got %d", len(searchRes2))
	}

	// 8. Delete Profile
	err = db.DeleteProfile(prof.ID)
	if err != nil {
		t.Fatalf("Failed to delete profile: %v", err)
	}

	// Active profile should reset to 0 after deleting active profile
	activeID, _ = db.GetActiveProfileID()
	if activeID != 0 {
		t.Errorf("Expected active profile to reset to 0, got %d", activeID)
	}
}
