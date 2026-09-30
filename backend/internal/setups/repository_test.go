package setups

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/chassis"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
)

func TestSetupPersistence(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	catalog := chassis.NewService(chassis.NewRepository(pool))
	service := NewService(repo, catalog)
	userService := users.NewService(users.NewRepository(pool))
	owner, err := userService.Login(ctx, users.Identity{Subject: "owner", Email: "owner@example.com", Name: "Owner"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := userService.Login(ctx, users.Identity{Subject: "other", Email: "other@example.com", Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	brand, err := catalog.CreateBrand(ctx, "Yokomo")
	if err != nil {
		t.Fatal(err)
	}
	model, err := catalog.CreateModel(ctx, brand.ID, "RD2.0")
	if err != nil {
		t.Fatal(err)
	}
	input := CreateInput{Title: " Setup ", ChassisModelID: &model.ID, Visibility: Public, Data: realisticData(t), Notes: stringPtr("Notes")}
	first, err := service.Create(ctx, owner.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if first.OwnerID != owner.ID || first.SchemaVersion != 1 || first.Title != "Setup" || first.Data.Suspension.Rear.ToeDeg == nil || *first.Data.Suspension.Rear.ToeDeg != 0 {
		t.Fatal("persisted fields", first)
	}
	loaded, err := repo.GetOwned(ctx, owner.ID, first.ID)
	if err != nil || !reflect.DeepEqual(first.Data, loaded.Data) {
		t.Fatal("JSONB round trip", loaded, err)
	}
	var version int
	var zero float64
	if err := pool.QueryRow(ctx, `SELECT schema_version,(data #>> '{suspension,rear,toe_deg}')::float8 FROM setups WHERE id=$1`, first.ID).Scan(&version, &zero); err != nil || version != 1 || zero != 0 {
		t.Fatal("stored JSON/version", version, zero, err)
	}
	input.ChassisModelID = nil
	input.Title = "Custom"
	input.Data = &DataV1{Suspension: &Suspension{Rear: &AxleSuspension{}}}
	second, err := service.Create(ctx, owner.ID, input)
	if err != nil || second.ChassisModelID != nil || second.Data.Suspension.Rear.ToeDeg != nil {
		t.Fatal("nullable or omitted values", second, err)
	}
	foreign, err := service.Create(ctx, other.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetOwned(ctx, other.ID, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-owner read", err)
	}
	if _, err := repo.UpdateOwned(ctx, other.ID, first.ID, first, first.UpdatedAt); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-owner update", err)
	}
	if err := repo.DeleteOwned(ctx, other.ID, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-owner delete", err)
	}
	old := first
	first, err = service.Patch(ctx, owner.ID, first.ID, PatchInput{Title: NullableString{Present: true, Value: stringPtr("Changed")}})
	if err != nil || first.Title != "Changed" || first.Notes == nil || *first.Notes != "Notes" || first.ChassisModelID == nil || !first.UpdatedAt.After(old.UpdatedAt) {
		t.Fatal("partial update", first, err)
	}
	if _, err := repo.UpdateOwned(ctx, owner.ID, first.ID, old, old.UpdatedAt); !errors.Is(err, ErrConflict) {
		t.Fatal("stale update not rejected", err)
	}
	replacement := DataV1{Electronics: &Electronics{Motor: " New motor "}}
	first, err = service.Patch(ctx, owner.ID, first.ID, PatchInput{ChassisModelID: NullableString{Present: true}, Notes: NullableString{Present: true}, Data: OptionalData{Present: true, Value: &replacement}})
	if err != nil || first.ChassisModelID != nil || first.Notes != nil || first.Data.Suspension != nil || first.Data.Electronics.Motor != "New motor" || first.SchemaVersion != 1 {
		t.Fatal("null clearing/data replacement", first, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE setups SET created_at='2020-01-01T00:00:00Z' WHERE id=$1", first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE setups SET created_at='2021-01-01T00:00:00Z' WHERE id=$1", second.ID); err != nil {
		t.Fatal(err)
	}
	list, err := service.List(ctx, owner.ID)
	if err != nil || len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatal("own list ordering", list, err)
	}
	for _, s := range list {
		if s.ID == foreign.ID {
			t.Fatal("foreign setup listed")
		}
	}
	if err := service.Delete(ctx, owner.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, owner.ID, second.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err := service.Delete(ctx, owner.ID, second.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	// Future versions must not be silently reinterpreted as v1.
	if _, err := pool.Exec(ctx, "UPDATE setups SET schema_version=2 WHERE id=$1", first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetOwned(ctx, owner.ID, first.ID); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatal("unsupported version", err)
	}
}
func TestHistoricalChassis(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	catalog := chassis.NewService(chassis.NewRepository(pool))
	service := NewService(NewRepository(pool), catalog)
	owner, err := users.NewService(users.NewRepository(pool)).Login(ctx, users.Identity{Subject: "owner", Email: "owner@example.com", Name: "Owner"})
	if err != nil {
		t.Fatal(err)
	}
	for _, disableBrand := range []bool{false, true} {
		name := "model"
		if disableBrand {
			name = "brand"
		}
		t.Run(name, func(t *testing.T) {
			brand, err := catalog.CreateBrand(ctx, name)
			if err != nil {
				t.Fatal(err)
			}
			model, err := catalog.CreateModel(ctx, brand.ID, "Model")
			if err != nil {
				t.Fatal(err)
			}
			input := CreateInput{Title: "History", ChassisModelID: &model.ID, Visibility: Private, Data: realisticData(t)}
			original, err := service.Create(ctx, owner.ID, input)
			if err != nil {
				t.Fatal(err)
			}
			if disableBrand {
				_, err = catalog.SetBrandActive(ctx, brand.ID, false)
			} else {
				_, err = catalog.SetModelActive(ctx, model.ID, false)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Get(ctx, owner.ID, original.ID); err != nil {
				t.Fatal("history unreadable", err)
			}
			updated, err := service.Patch(ctx, owner.ID, original.ID, PatchInput{Title: NullableString{Present: true, Value: stringPtr("Historical title")}, Notes: NullableString{Present: true, Value: stringPtr("Note")}, Data: OptionalData{Present: true, Value: &DataV1{}}})
			if err != nil || updated.ChassisModelID == nil || *updated.ChassisModelID != model.ID {
				t.Fatal("historical reference lost", updated, err)
			}
			if _, err := service.Patch(ctx, owner.ID, original.ID, PatchInput{ChassisModelID: NullableString{Present: true, Value: &model.ID}}); err != nil {
				t.Fatal("unchanged inactive model rejected", err)
			}
			if _, err := service.Create(ctx, owner.ID, input); !errors.Is(err, ErrInvalidChassis) {
				t.Fatal("new inactive selection allowed", err)
			}
			input.ChassisModelID = nil
			another, err := service.Create(ctx, owner.ID, input)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Patch(ctx, owner.ID, another.ID, PatchInput{ChassisModelID: NullableString{Present: true, Value: &model.ID}}); !errors.Is(err, ErrInvalidChassis) {
				t.Fatal("changed selection allowed inactive model", err)
			}
			if _, err := service.Patch(ctx, owner.ID, original.ID, PatchInput{ChassisModelID: NullableString{Present: true}}); err != nil {
				t.Fatal("clear inactive reference", err)
			}
		})
	}
}
