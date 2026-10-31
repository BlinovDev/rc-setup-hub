package setups

import (
	"context"
	"encoding/json"
	"testing"
)

// Keep lineage redaction across the shared full projection (detail, list and
// Compare), including when the viewer owns the hidden source.
func TestCloneLineageProjectionAcrossReads(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	source := f.public[0]
	clone, err := f.service.Clone(ctx, f.owner.ID, source.ID, CloneInput{"00000000-0000-4000-8000-000000000001"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		visibility Visibility
		status     ModerationStatus
		exposed    bool
	}{
		{Public, Approved, true}, {Public, Pending, false}, {Public, Rejected, false}, {Friends, Approved, false}, {Private, Approved, false},
	} {
		if _, err := f.pool.Exec(ctx, `UPDATE setups SET visibility=$2,moderation_status=$3 WHERE id=$1`, source.ID, tc.visibility, tc.status); err != nil {
			t.Fatal(err)
		}
		detail, err := f.service.Get(ctx, f.owner.ID, clone.ID)
		if err != nil {
			t.Fatal(err)
		}
		list, err := f.service.List(ctx, f.owner.ID)
		if err != nil {
			t.Fatal(err)
		}
		pair, err := f.service.Compare(ctx, f.owner.ID, clone.ID, f.public[1].ID)
		if err != nil {
			t.Fatal(err)
		}
		var listed Setup
		for _, s := range list {
			if s.ID == clone.ID {
				listed = s
			}
		}
		for _, s := range []Setup{detail, listed, pair.Left} {
			if s.ID != clone.ID || (s.SourceSetupID != nil) != tc.exposed {
				t.Fatal("lineage visibility", tc, s)
			}
			raw, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			var response map[string]json.RawMessage
			if err := json.Unmarshal(raw, &response); err != nil {
				t.Fatal(err)
			}
			if _, exposed := response["source_setup_id"]; exposed != tc.exposed {
				t.Fatal("lineage JSON disclosure", tc, string(raw))
			}
		}
	}
	// The FK clears stored lineage as well as omitting its transport projection.
	if err := f.service.Delete(ctx, f.owner.ID, source.ID); err != nil {
		t.Fatal(err)
	}
	var cleared bool
	if err := f.pool.QueryRow(ctx, `SELECT source_setup_id IS NULL FROM setups WHERE id=$1`, clone.ID).Scan(&cleared); err != nil || !cleared {
		t.Fatal("source deletion lineage", cleared, err)
	}
}
