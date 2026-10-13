package database

type CommunitySubGroup struct {
	CommunityJID string
	GroupJID     string
	GroupName    string
	IsDefaultSub bool
}

// SaveCommunitySubGroups caches the sub-groups belonging to a community.
func (a *AppDB) SaveCommunitySubGroups(communityJID string, groups []CommunitySubGroup) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("DELETE FROM community_subgroups WHERE community_jid = ?", communityJID)
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO community_subgroups (community_jid, group_jid, group_name, is_default_sub) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, g := range groups {
		_, _ = stmt.Exec(communityJID, g.GroupJID, g.GroupName, g.IsDefaultSub)
	}

	return tx.Commit()
}

// GetCommunitySubGroups retrieves all cached sub-groups for a community.
func (a *AppDB) GetCommunitySubGroups(communityJID string) ([]CommunitySubGroup, error) {
	rows, err := a.db.Query("SELECT community_jid, group_jid, group_name, is_default_sub FROM community_subgroups WHERE community_jid = ? ORDER BY is_default_sub DESC, group_name ASC", communityJID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CommunitySubGroup
	for rows.Next() {
		var g CommunitySubGroup
		if err := rows.Scan(&g.CommunityJID, &g.GroupJID, &g.GroupName, &g.IsDefaultSub); err == nil {
			result = append(result, g)
		}
	}
	return result, nil
}

// SetContactCommunity updates the community flags on a contact.
func (a *AppDB) SetContactCommunity(jid string, isCommunity bool, parentJID string) error {
	_, err := a.db.Exec("UPDATE contacts SET is_community = ?, parent_group_jid = ? WHERE jid = ?", isCommunity, parentJID, jid)
	return err
}

// IsCommunity checks whether a chat is a community parent group.
func (a *AppDB) IsCommunity(jid string) bool {
	var isComm bool
	_ = a.db.QueryRow("SELECT COALESCE(is_community, 0) FROM contacts WHERE jid = ?", jid).Scan(&isComm)
	return isComm
}

// GetParentCommunity returns the parent community JID of a group, if any.
func (a *AppDB) GetParentCommunity(jid string) string {
	var parentJID string
	_ = a.db.QueryRow("SELECT COALESCE(parent_group_jid, '') FROM contacts WHERE jid = ?", jid).Scan(&parentJID)
	return parentJID
}

// GetAvailableGroupsForCommunity returns groups that can be linked to the specified community.
func (a *AppDB) GetAvailableGroupsForCommunity(communityJID string) ([]Contact, error) {
	query := `SELECT c.jid, c.lid, c.saved_name, c.push_name, c.avatar_path, c.is_group, c.last_message_at, c.unread_count, c.is_pinned, c.is_archived, c.muted_until 
	          FROM contacts c
	          WHERE c.is_group = 1 
	            AND (c.is_community IS NULL OR c.is_community = 0)
	            AND c.jid != ?
	            AND c.jid NOT IN (SELECT group_jid FROM community_subgroups WHERE community_jid = ?)
	          ORDER BY COALESCE(NULLIF(c.saved_name, ''), c.push_name, c.jid) ASC`
	rows, err := a.db.Query(query, communityJID, communityJID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Contact
	for rows.Next() {
		if c, err := scanContact(rows); err == nil {
			list = append(list, c)
		}
	}
	return list, nil
}
