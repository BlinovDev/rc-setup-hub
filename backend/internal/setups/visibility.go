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
