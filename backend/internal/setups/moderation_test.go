package setups

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
)

func TestModerationPolicy(t *testing.T) {
	for _, status := range []ModerationStatus{Pending, Approved, Rejected} {
		for _, visibility := range []Visibility{Public, Friends, Private} {
			for _, viewer := range []struct {
				id     string
				friend bool
			}{{"owner", false}, {"friend", true}, {"stranger", false}, {"", true}} {
				want := viewer.id == "owner" || (viewer.id != "" && viewer.friend && visibility != Private) || (visibility == Public && status == Approved)
				if got := CanViewModerated(viewer.id, "owner", visibility, status, viewer.friend); got != want {
					t.Fatal(status, visibility, viewer, got, want)
				}
			}
		}
	}
}

func TestModerationTransitionsAndAtomicPhotoMutations(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	s := f.service
	r := f.repo
	setup, err := s.Create(ctx, f.owner.ID, CreateInput{Title: "Moderation", Visibility: Public, Data: &DataV1{}})
	if err != nil {
		t.Fatal(err)
	}
	current := func() Setup {
		t.Helper()
		got, err := r.GetOwned(ctx, f.owner.ID, setup.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	status := func(want ModerationStatus) {
		t.Helper()
		if got := current().ModerationStatus; got != want {
			t.Fatal("status", got, want)
		}
	}
	decide := func(decision ModerationStatus) {
		t.Helper()
		if err := s.Review(ctx, setup.ID, f.other.ID, current().UpdatedAt, decision); err != nil {
			t.Fatal(err)
		}
	}
	patchVisibility := func(v Visibility) {
		t.Helper()
		if _, err := s.Patch(ctx, f.owner.ID, setup.ID, PatchInput{Visibility: NullableString{Present: true, Value: stringPtr(string(v))}}); err != nil {
			t.Fatal(err)
		}
	}
	status(Pending)
	decide(Approved) // zero photos may be approved
	for _, v := range []Visibility{Private, Friends, Public} {
		patchVisibility(v)
		status(Approved)
	}
	before := current()
	for _, patch := range []PatchInput{
		{Title: NullableString{Present: true, Value: stringPtr("Edited")}},
		{Notes: NullableString{Present: true, Value: stringPtr("Notes")}},
		{ChassisModelID: NullableString{Present: true, Value: &f.rd.ID}},
		{Data: OptionalData{Present: true, Value: &DataV1{Suspension: &Suspension{Rear: &AxleSuspension{ToeDeg: floatPtr(0)}}}}},
	} {
		if _, err := s.Patch(ctx, f.owner.ID, setup.ID, patch); err != nil {
			t.Fatal(err)
		}
		status(Approved)
	}
	if err := s.Review(ctx, setup.ID, f.other.ID, before.UpdatedAt, Rejected); !errors.Is(err, ErrConflict) {
		t.Fatal("stale decision", err)
	}
	image := func(n int) photos.Image {
		return photos.Image{Key: fmt.Sprintf("%032x", n), Width: 20, Height: 10, ThumbnailWidth: 10, ThumbnailHeight: 10, Crop: photos.Crop{X: 5, Size: 10}}
	}
	add := func(n int) Photo {
		t.Helper()
		p, _, err := r.addPhoto(ctx, f.owner.ID, setup.ID, fmt.Sprintf("00000000-0000-4000-8000-%012d", n), "digest", image(n))
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := add(1)
	status(Pending)
	decide(Approved)
	approved := current()
	if _, _, err := r.addPhoto(ctx, f.owner.ID, setup.ID, "00000000-0000-4000-8000-000000000001", "different", image(2)); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if _, _, err := r.addPhoto(ctx, f.owner.ID, setup.ID, "00000000-0000-4000-8000-000000000001", "digest", image(2)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.mutatePhoto(ctx, f.owner.ID, setup.ID, p.ID, false); err != nil {
		t.Fatal(err)
	}
	status(Approved)
	if !current().UpdatedAt.Equal(approved.UpdatedAt) {
		t.Fatal("retry/same cover changed revision")
	}
	revision := 1
	generateFailure := func(Photo, photos.Crop) (photos.Image, error) { return photos.Image{}, errors.New("generation failed") }
	body := CropInput{ExpectedRevision: &revision, Crop: &photos.Crop{Size: 5}}
	if _, _, err := r.cropPhoto(ctx, f.owner.ID, setup.ID, p.ID, body, generateFailure); err == nil {
		t.Fatal("failed crop accepted")
	}
	status(Approved)
	if _, _, err := r.cropPhoto(ctx, f.owner.ID, setup.ID, p.ID, body, func(Photo, photos.Crop) (photos.Image, error) { return image(90), nil }); err != nil {
		t.Fatal(err)
	}
	status(Pending)
	if err := s.Review(ctx, setup.ID, f.other.ID, approved.UpdatedAt, Approved); !errors.Is(err, ErrConflict) {
		t.Fatal("photo change accepted stale approval", err)
	}
	decide(Approved)
	if _, _, err := r.cropPhoto(ctx, f.owner.ID, setup.ID, p.ID, body, generateFailure); err != nil {
		t.Fatal("identical retry", err)
	}
	status(Approved)
	q := add(2)
	status(Pending)
	decide(Rejected)
	if _, err := s.Patch(ctx, f.owner.ID, setup.ID, PatchInput{Title: NullableString{Present: true, Value: stringPtr("Rejected text edit")}}); err != nil {
		t.Fatal(err)
	}
	status(Rejected)
	if _, err := r.mutatePhoto(ctx, f.owner.ID, setup.ID, q.ID, false); err != nil {
		t.Fatal(err)
	}
	status(Pending)
	decide(Approved)
	patchVisibility(Private)
	if _, err := r.mutatePhoto(ctx, f.owner.ID, setup.ID, q.ID, true); err != nil {
		t.Fatal(err)
	}
	status(Pending)
	patchVisibility(Public)
	status(Pending)
	decide(Rejected)
	patchVisibility(Friends)
	patchVisibility(Public)
	status(Pending)
	// Concurrent decisions consume only one revision and retain a reviewer/time.
	expected := current().UpdatedAt
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, d := range []ModerationStatus{Approved, Rejected} {
		wg.Add(1)
		go func(d ModerationStatus) { defer wg.Done(); results <- s.Review(ctx, setup.ID, f.other.ID, expected, d) }(d)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	var reviewer string
	var recorded bool
	if err := f.pool.QueryRow(ctx, `SELECT reviewed_by::text,reviewed_at IS NOT NULL FROM setups WHERE id=$1`, setup.ID).Scan(&reviewer, &recorded); err != nil || reviewer != f.other.ID || !recorded {
		t.Fatal(reviewer, recorded, err)
	}
	// Photo changes resubmit rejected records; many-photo limit failures preserve approval.
	for n := 3; n <= 11; n++ {
		add(n)
	}
	status(Pending)
	decide(Approved)
	if _, _, err := r.addPhoto(ctx, f.owner.ID, setup.ID, "00000000-0000-4000-8000-000000000012", "digest", image(12)); err == nil {
		t.Fatal("eleventh photo accepted")
	}
	status(Approved)
	if _, err := r.mutatePhoto(ctx, f.owner.ID, setup.ID, "00000000-0000-0000-0000-000000000000", true); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	status(Approved)
	// A transaction failure after the invalidation trigger must roll back both.
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM setup_photos WHERE id=$1`, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE setups SET moderation_status='bad' WHERE id=$1`, setup.ID); err == nil {
		t.Fatal("constraint missing")
	}
	_ = tx.Rollback(ctx)
	status(Approved)
	// A deferred cover constraint failure also rolls back the trigger's state reset.
	failed, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := failed.Exec(ctx, `DELETE FROM setup_photos WHERE id=$1`, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := failed.Commit(ctx); err == nil {
		t.Fatal("invalid gallery commit succeeded")
	}
	status(Approved)

	if _, err := r.mutatePhoto(ctx, f.owner.ID, setup.ID, p.ID, true); err != nil {
		t.Fatal(err)
	}
	status(Pending)
}

func TestPhotoMutationSerializesWithReview(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	setup, err := f.service.Create(ctx, f.owner.ID, CreateInput{Title: "Concurrent photo review", Visibility: Public, Data: &DataV1{}})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := lockPhotoSetup(ctx, tx, f.owner.ID, setup.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO setup_photos(setup_id,storage_key,width,height,thumbnail_width,thumbnail_height,is_cover) VALUES($1,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',20,10,20,10,true)`, setup.ID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- f.service.Review(ctx, setup.ID, f.other.ID, setup.UpdatedAt, Approved) }()
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, ErrConflict) {
		t.Fatal("review approved unseen photo", err)
	}
	current, err := f.repo.GetOwned(ctx, f.owner.ID, setup.ID)
	if err != nil || current.ModerationStatus != Pending || current.CoverPhoto == nil {
		t.Fatal(current, err)
	}
}
