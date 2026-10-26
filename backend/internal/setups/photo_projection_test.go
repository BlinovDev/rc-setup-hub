package setups

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
)

// Compare public transport values through real PostgreSQL joins. Decoder/file IO
// is deliberately outside this metadata-only test; native photo tests cover it.
func TestCoverPhotoProjectionParity(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	s := NewService(repo, nil, nil)
	owner, err := users.NewService(users.NewRepository(pool)).Login(ctx, users.Identity{Subject: "projection", Email: "projection@example.test", Name: "Projection"})
	if err != nil {
		t.Fatal(err)
	}
	setup, err := s.Create(ctx, owner.ID, CreateInput{Title: "Projection", Visibility: Public, Data: &DataV1{}})
	if err != nil {
		t.Fatal(err)
	}
	check := func(t *testing.T) {
		t.Helper()
		gallery, err := repo.listPhotos(ctx, owner.ID, setup.ID)
		if err != nil {
			t.Fatal(err)
		}
		var expected *Photo
		if len(gallery) > 0 {
			expected = &gallery[0]
		}
		want, err := json.Marshal(expected)
		if err != nil {
			t.Fatal(err)
		}
		assertPhoto := func(name string, p *Photo) {
			t.Helper()
			got, err := json.Marshal(p)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("%s photo = %s, want %s: %v", name, got, want, err)
			}
			for _, private := range []string{"storage_key", "thumbnail_key", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"} {
				if bytes.Contains(got, []byte(private)) {
					t.Fatalf("%s exposed private photo storage", name)
				}
			}
		}
		for name, read := range map[string]func() (Setup, error){
			"detail": func() (Setup, error) { return s.Get(ctx, owner.ID, setup.ID) },
			"owned":  func() (Setup, error) { return repo.GetOwned(ctx, owner.ID, setup.ID) },
			"patch": func() (Setup, error) {
				return s.Patch(ctx, owner.ID, setup.ID, PatchInput{Title: NullableString{Present: true, Value: stringPtr("Projection")}})
			},
		} {
			result, err := read()
			if err != nil {
				t.Fatal(name, err)
			}
			assertPhoto(name, result.CoverPhoto)
		}
		for name, read := range map[string]func() ([]Setup, error){
			"mine":    func() ([]Setup, error) { return s.List(ctx, owner.ID) },
			"profile": func() ([]Setup, error) { return s.ListUser(ctx, owner.ID, owner.ID) },
		} {
			result, err := read()
			if err != nil || len(result) != 1 {
				t.Fatalf("%s: count=%d, error=%v", name, len(result), err)
			}
			assertPhoto(name, result[0].CoverPhoto)
		}
		page, err := s.Search(ctx, SearchInput{})
		if err != nil || len(page.Items) != 1 {
			t.Fatal("search", page, err)
		}
		assertPhoto("search", page.Items[0].CoverPhoto)
		if expected != nil {
			cover, err := repo.mutatePhoto(ctx, owner.ID, setup.ID, expected.ID, false)
			if err != nil {
				t.Fatal(err)
			}
			assertPhoto("cover mutation", &cover)
		}
	}
	t.Run("no cover", check)
	var photoID string
	if err := pool.QueryRow(ctx, `INSERT INTO setup_photos(setup_id,storage_key,width,height,thumbnail_width,thumbnail_height,is_cover) VALUES($1,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',20,10,20,10,true) RETURNING id::text`, setup.ID).Scan(&photoID); err != nil {
		t.Fatal(err)
	}
	t.Run("legacy crop null and revision zero", check)
	if _, err := pool.Exec(ctx, `UPDATE setup_photos SET crop='{"x":0,"y":0,"size":5}',thumbnail_revision=2,thumbnail_key='bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',thumbnail_width=5,thumbnail_height=5 WHERE id=$1`, photoID); err != nil {
		t.Fatal(err)
	}
	t.Run("cropped version", check)
	if _, err := repo.mutatePhoto(ctx, owner.ID, setup.ID, photoID, true); err != nil {
		t.Fatal(err)
	}
	t.Run("deleted cover", check)
}
