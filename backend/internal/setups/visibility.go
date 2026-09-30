package setups

// VisibleVisibilities is the central read policy for an authenticated caller.
// An empty caller fails closed; friendship must already be confirmed accepted.
func VisibleVisibilities(callerID, ownerID string, acceptedFriend bool) []Visibility {
	if callerID == "" {
		return nil
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
