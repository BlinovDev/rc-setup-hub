package setups

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestClonePublicSetupIndependenceAndReceipts(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	source := f.public[0]
	// Preserve the original JSONB, including legacy unknowns, explicit zero and precision.
	raw := `{"suspension":{"front":{"toe_deg":0,"camber_deg":-3.123456789,"link_lengths":[{"name":"Legacy unknown","length_mm":null}]}},"electronics":{"motor":"Motor"},"running_gear":{"rear":{"wheel":{"offset_mm":0}}}}`
	if _, err := f.pool.Exec(ctx, `UPDATE setups SET title=$2,data=$3,notes='Notes' WHERE id=$1`, source.ID, strings.Repeat("界", 150), raw); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO setup_photos(setup_id,storage_key,width,height,thumbnail_width,thumbnail_height,is_cover) VALUES($1,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',20,10,20,10,true)`, source.ID); err != nil {
		t.Fatal(err)
	}
	approveFixture(t, f.pool, source.ID)
	if _, err := f.catalog.SetBrandActive(ctx, f.yokomo.ID, false); err != nil {
		t.Fatal(err)
	}
	custom, err := f.service.Clone(ctx, f.owner.ID, f.public[3].ID, CloneInput{"00000000-0000-4000-8000-000000000088"})
	if err != nil || custom.ChassisModelID != nil || custom.Chassis != nil {
		t.Fatal("custom chassis/own public clone", custom, err)
	}
	request := "00000000-0000-4000-8000-000000000001"
	clone, err := f.service.Clone(ctx, f.other.ID, source.ID, CloneInput{request})
	if err != nil {
		t.Fatal(err)
	}
	if clone.ID == source.ID || clone.OwnerID != f.other.ID || clone.Visibility != Private || clone.ModerationStatus != Pending || clone.CoverPhoto != nil || clone.SourceSetupID == nil || *clone.SourceSetupID != source.ID || clone.Title != strings.Repeat("界", 143)+" (Copy)" || clone.ChassisModelID == nil || *clone.ChassisModelID != *source.ChassisModelID || clone.Notes == nil || *clone.Notes != "Notes" {
		t.Fatal(clone)
	}
	var equal bool
	var photos int
	if err := f.pool.QueryRow(ctx, `SELECT a.data=b.data,(SELECT count(*) FROM setup_photos WHERE setup_id=a.id) FROM setups a,setups b WHERE a.id=$1 AND b.id=$2`, clone.ID, source.ID).Scan(&equal, &photos); err != nil || !equal || photos != 0 {
		t.Fatal(equal, photos, err)
	}
	retry, err := f.service.Clone(ctx, f.other.ID, source.ID, CloneInput{request})
	if err != nil || retry.ID != clone.ID {
		t.Fatal("duplicate retry", retry, err)
	}
	if _, err := f.service.Clone(ctx, f.other.ID, f.public[1].ID, CloneInput{request}); !errors.Is(err, ErrConflict) {
		t.Fatal("request reuse", err)
	}
	// Distinct deliberate requests remain independent; concurrent identical requests make one clone.
	var wg sync.WaitGroup
	ids := make(chan string, 5)
	errs := make(chan error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := f.service.Clone(ctx, f.other.ID, source.ID, CloneInput{"00000000-0000-4000-8000-000000000002"})
			ids <- c.ID
			errs <- err
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	var concurrent string
	for id := range ids {
		if concurrent != "" && id != concurrent {
			t.Fatal("duplicate clones", id, concurrent)
		}
		concurrent = id
	}
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if concurrent == clone.ID {
		t.Fatal("distinct request deduplicated")
	}
	if _, err := f.pool.Exec(ctx, `UPDATE setups SET visibility='private',title='Changed',data='{}' WHERE id=$1`, source.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Clone(ctx, f.other.ID, source.ID, CloneInput{request}); !errors.Is(err, ErrNotFound) {
		t.Fatal("replay bypassed eligibility", err)
	}
	current, err := f.service.Get(ctx, f.other.ID, clone.ID)
	if err != nil || current.SourceSetupID != nil || !reflect.DeepEqual(current.Data, clone.Data) {
		t.Fatal("source metadata/independence", current, err)
	}
	if err := f.service.Delete(ctx, f.owner.ID, source.ID); err != nil {
		t.Fatal(err)
	}
	current, err = f.service.Get(ctx, f.other.ID, clone.ID)
	if err != nil || current.SourceSetupID != nil || current.Notes == nil || *current.Notes != "Notes" {
		t.Fatal("source deletion", current, err)
	}
	if _, err := f.pool.Exec(ctx, `SELECT source_setup_id FROM setups WHERE id=$1`, clone.ID); err != nil {
		t.Fatal(err)
	}
	// Deleting a copy leaves a receipt tombstone, so a lost-response retry cannot resurrect it.
	if err := f.service.Delete(ctx, f.other.ID, concurrent); err != nil {
		t.Fatal(err)
	}
}

func TestCloneEligibilityAndConcurrentRevocation(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO friendships(requester_id,addressee_id,status) VALUES($1,$2,'accepted')`, f.owner.ID, f.friend.ID); err != nil {
		t.Fatal(err)
	}
	for i, tc := range []struct {
		visibility Visibility
		status     ModerationStatus
	}{{Private, Approved}, {Friends, Approved}, {Public, Pending}, {Public, Rejected}} {
		source, err := f.service.Create(ctx, f.owner.ID, CreateInput{Title: "Ineligible", Visibility: tc.visibility, Data: &DataV1{}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `UPDATE setups SET moderation_status=$2 WHERE id=$1`, source.ID, tc.status); err != nil {
			t.Fatal(err)
		}
		for _, caller := range []string{f.owner.ID, f.friend.ID, f.other.ID} {
			if _, err := f.service.Clone(ctx, caller, source.ID, CloneInput{fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1)}); !errors.Is(err, ErrNotFound) {
				t.Fatal(tc, caller, err)
			}
		}
	}
	source := f.public[0]
	// A deleted clone's request is a tombstone even while its source is still eligible.
	tombstone := CloneInput{"00000000-0000-4000-8000-000000000099"}
	copy, err := f.service.Clone(ctx, f.other.ID, source.ID, tombstone)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.Delete(ctx, f.other.ID, copy.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Clone(ctx, f.other.ID, source.ID, tombstone); !errors.Is(err, ErrConflict) {
		t.Fatal("resurrected clone", err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM setup_clone_requests`); err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE setups SET moderation_status='pending' WHERE id=$1`, source.ID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := f.service.Clone(ctx, f.other.ID, source.ID, CloneInput{"00000000-0000-4000-8000-000000000009"})
		result <- err
	}()
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, ErrNotFound) {
		t.Fatal("concurrent approval revocation", err)
	}
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM setup_clone_requests`).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed clone persisted", count, err)
	}
}
