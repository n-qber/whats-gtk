package database

import (
	"database/sql"
	"os"
	"testing"
)

func TestCommunityDB(t *testing.T) {
	dbPath := "test_community.db"
	defer os.Remove(dbPath)

	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}
	defer db.Close()

	commJID := "120363999999999999@g.us"
	sub1JID := "120363111111111111@g.us"
	sub2JID := "120363222222222222@g.us"
	otherGroupJID := "120363333333333333@g.us"

	// 1. Save community and group contacts
	_ = db.SaveContact(Contact{JID: commJID, SavedName: sql.NullString{String: "Minha Comunidade", Valid: true}, IsGroup: sql.NullBool{Bool: true, Valid: true}})
	_ = db.SaveContact(Contact{JID: sub1JID, SavedName: sql.NullString{String: "Avisos", Valid: true}, IsGroup: sql.NullBool{Bool: true, Valid: true}})
	_ = db.SaveContact(Contact{JID: sub2JID, SavedName: sql.NullString{String: "Grupo Geral", Valid: true}, IsGroup: sql.NullBool{Bool: true, Valid: true}})
	_ = db.SaveContact(Contact{JID: otherGroupJID, SavedName: sql.NullString{String: "Grupo Outro", Valid: true}, IsGroup: sql.NullBool{Bool: true, Valid: true}})

	// 2. Set contact community flags
	if err := db.SetContactCommunity(commJID, true, ""); err != nil {
		t.Fatalf("Failed to set community: %v", err)
	}
	if !db.IsCommunity(commJID) {
		t.Errorf("Expected commJID to be recognized as community")
	}

	if err := db.SetContactCommunity(sub1JID, false, commJID); err != nil {
		t.Fatalf("Failed to set sub-group: %v", err)
	}
	if parent := db.GetParentCommunity(sub1JID); parent != commJID {
		t.Errorf("Expected parent community %s, got %s", commJID, parent)
	}

	// 3. Save sub-groups
	subs := []CommunitySubGroup{
		{
			CommunityJID: commJID,
			GroupJID:     sub1JID,
			GroupName:    "Avisos",
			IsDefaultSub: true,
		},
		{
			CommunityJID: commJID,
			GroupJID:     sub2JID,
			GroupName:    "Grupo Geral",
			IsDefaultSub: false,
		},
	}
	if err := db.SaveCommunitySubGroups(commJID, subs); err != nil {
		t.Fatalf("Failed to save sub groups: %v", err)
	}

	loadedSubs, err := db.GetCommunitySubGroups(commJID)
	if err != nil {
		t.Fatalf("Failed to get sub groups: %v", err)
	}
	if len(loadedSubs) != 2 {
		t.Fatalf("Expected 2 sub-groups, got %d", len(loadedSubs))
	}
	if !loadedSubs[0].IsDefaultSub {
		t.Errorf("Expected first sub-group to be default announcement subgroup")
	}

	// 4. Test GetAvailableGroupsForCommunity (should include otherGroupJID, but NOT sub1JID, sub2JID, or commJID)
	available, err := db.GetAvailableGroupsForCommunity(commJID)
	if err != nil {
		t.Fatalf("Failed to get available groups: %v", err)
	}
	if len(available) != 1 {
		t.Fatalf("Expected 1 available group to link, got %d", len(available))
	}
	if available[0].JID != otherGroupJID {
		t.Errorf("Expected available group %s, got %s", otherGroupJID, available[0].JID)
	}
}
