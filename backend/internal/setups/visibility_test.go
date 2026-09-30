package setups

import "testing"

func TestVisibilityPolicy(t *testing.T) {
	for _, caller := range []struct {
		name, id string
		accepted bool
		allowed  [3]bool
	}{
		{"owner", "owner", false, [3]bool{true, true, true}},
		{"accepted friend", "other", true, [3]bool{true, true, false}},
		{"unrelated", "other", false, [3]bool{true, false, false}},
		{"pending friend", "other", false, [3]bool{true, false, false}},
		{"unauthenticated", "", true, [3]bool{false, false, false}},
	} {
		for i, visibility := range []Visibility{Public, Friends, Private} {
			t.Run(caller.name+"/"+string(visibility), func(t *testing.T) {
				if got := CanView(caller.id, "owner", visibility, caller.accepted); got != caller.allowed[i] {
					t.Fatalf("got %v want %v", got, caller.allowed[i])
				}
			})
		}
	}
	if CanView("owner", "owner", Visibility("unknown"), true) {
		t.Fatal("unknown visibility allowed")
	}
}
