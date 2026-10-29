package setups

// VisibleVisibilities is the central read policy, including anonymous readers.
// Guests receive only public even if a caller supplies an accepted-friend flag.
func VisibleVisibilities(callerID, ownerID string, acceptedFriend bool) []Visibility {
	if callerID == "" {
		return []Visibility{Public}
	}
	if callerID == ownerID {
		return []Visibility{Public, Friends, Private}
	}
	if acceptedFriend {
		return []Visibility{Public, Friends}
	}
	return []Visibility{Public}
}

// CanView applies the same policy used to restrict listing SQL.
func CanView(callerID, ownerID string, visibility Visibility, acceptedFriend bool) bool {
	for _, allowed := range VisibleVisibilities(callerID, ownerID, acceptedFriend) {
		if visibility == allowed {
			return true
		}
	}
	return false
}

// visibleSetupPredicate is the SQL counterpart of CanView for alias s. The
// placeholder is a fixed repository parameter, never request-supplied SQL.
// NULL represents a guest; the indexed unordered friendship pair grants access
// only while accepted in the same statement snapshot as the protected row.
func visibleSetupPredicate(placeholder string) string {
	viewer := "NULLIF(" + placeholder + "::text,'')::uuid"
	return `((s.visibility='public' AND s.moderation_status='approved') OR s.owner_id=` + viewer + ` OR
 (s.visibility IN ('public','friends') AND EXISTS (SELECT 1 FROM friendships f
 WHERE LEAST(f.requester_id,f.addressee_id)=LEAST(` + viewer + `,s.owner_id)
 AND GREATEST(f.requester_id,f.addressee_id)=GREATEST(` + viewer + `,s.owner_id)
 AND ` + viewer + ` IS NOT NULL AND f.status='accepted')))`
}

// CanViewModerated separates the requested audience from effective public access.
func CanViewModerated(callerID, ownerID string, visibility Visibility, status ModerationStatus, acceptedFriend bool) bool {
	if visibility == Public && status != Approved {
		return callerID != "" && (callerID == ownerID || acceptedFriend)
	}
	return CanView(callerID, ownerID, visibility, acceptedFriend)
}
