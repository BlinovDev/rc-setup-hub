package chassis

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
)

func TestNameValidation(t *testing.T) {
	for _, tc := range []struct {
		input, want string
		err         error
	}{
		{" Yokomo ", "Yokomo", nil}, {"", "", ErrInvalidName}, {"  ", "", ErrInvalidName},
		{strings.Repeat("a", 101), "", ErrInvalidName}, {"bad\x00name", "", ErrInvalidName}, {" Custom ", "", ErrCustomName},
	} {
		got, err := validateName(tc.input)
		if got != tc.want || !errors.Is(err, tc.err) {
			t.Fatalf("%q: %q %v", tc.input, got, err)
		}
	}
}
func TestCatalogPersistence(t *testing.T) {
	pool := dbtest.New(t)
	repo := NewRepository(pool)
	service := NewService(repo)
	ctx := context.Background()
	brand, err := service.CreateBrand(ctx, " Yokomo ")
	if err != nil || brand.Name != "Yokomo" || !brand.IsActive {
		t.Fatal(brand, err)
	}
	if _, err := repo.CreateBrand(ctx, "YOKOMO"); !errors.Is(err, ErrDuplicate) {
		t.Fatal("duplicate brand", err)
	}
	second, err := service.CreateBrand(ctx, "Other")
	if err != nil {
		t.Fatal(err)
	}
	model, err := service.CreateModel(ctx, brand.ID, " RD2.0 ")
	if err != nil || model.Name != "RD2.0" || !model.IsActive {
		t.Fatal(model, err)
	}
	if _, err := repo.CreateModel(ctx, brand.ID, "rd2.0"); !errors.Is(err, ErrDuplicate) {
		t.Fatal("same-brand duplicate", err)
	}
	if _, err := repo.CreateModel(ctx, second.ID, "rd2.0"); err != nil {
		t.Fatal("different-brand name", err)
	}
	brand, err = service.RenameBrand(ctx, brand.ID, " Yokomo Racing ")
	if err != nil || brand.Name != "Yokomo Racing" {
		t.Fatal(brand, err)
	}
	if _, err := service.RenameBrand(ctx, second.ID, "YOKOMO RACING"); !errors.Is(err, ErrDuplicate) {
		t.Fatal("rename duplicate brand", err)
	}
	model, err = service.RenameModel(ctx, model.ID, " RD2.0 Limited ")
	if err != nil || model.BrandID != brand.ID || model.Name != "RD2.0 Limited" {
		t.Fatal(model, err)
	}
	duplicate, err := service.CreateModel(ctx, brand.ID, "Other model")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RenameModel(ctx, duplicate.ID, "rd2.0 limited"); !errors.Is(err, ErrDuplicate) {
		t.Fatal("rename duplicate model", err)
	}
	if _, err := service.SetModelActive(ctx, model.ID, false); err != nil {
		t.Fatal(err)
	}
	active, err := service.ActiveModels(ctx, brand.ID)
	if err != nil || len(active) != 1 || active[0].ID != duplicate.ID {
		t.Fatal("inactive model leaked", active, err)
	}
	state, err := service.GetModel(ctx, model.ID)
	if err != nil || state.IsActive || !state.BrandActive || state.BrandName != "Yokomo Racing" {
		t.Fatal("inactive record lost", state, err)
	}
	if _, err := service.SetModelActive(ctx, model.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetBrandActive(ctx, brand.ID, false); err != nil {
		t.Fatal(err)
	}
	brands, err := service.ActiveBrands(ctx)
	if err != nil || len(brands) != 1 || brands[0].ID != second.ID {
		t.Fatal("inactive brand leaked", brands, err)
	}
	if _, err := service.ActiveModels(ctx, brand.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("inactive brand client result", err)
	}
	direct, err := repo.ActiveModels(ctx, brand.ID)
	if err != nil || len(direct) != 0 {
		t.Fatal("SQL parent filter missing", direct, err)
	}
	state, err = service.GetModel(ctx, model.ID)
	if err != nil || !state.IsActive || state.BrandActive {
		t.Fatal("cascade-disable or record lost", state, err)
	}
	if _, err := service.CreateModel(ctx, brand.ID, "Not allowed"); !errors.Is(err, ErrInactiveBrand) {
		t.Fatal("inactive parent accepted", err)
	}
	if _, err := repo.CreateModel(ctx, brand.ID, "Not allowed"); !errors.Is(err, ErrInactiveBrand) {
		t.Fatal("repository active guard missing", err)
	}
	all, err := service.ListModels(ctx)
	if err != nil || len(all) != 3 {
		t.Fatal("historical models lost", all, err)
	}
	if _, err := service.SetBrandActive(ctx, brand.ID, true); err != nil {
		t.Fatal(err)
	}
	active, err = service.ActiveModels(ctx, brand.ID)
	if err != nil || len(active) != 2 {
		t.Fatal("enable did not restore selection", active, err)
	}
	unknown := "00000000-0000-0000-0000-000000000000"
	for _, err := range []error{func() error { _, e := service.GetModel(ctx, unknown); return e }(), func() error { _, e := service.CreateModel(ctx, unknown, "Model"); return e }(), func() error { _, e := service.RenameBrand(ctx, unknown, "Brand"); return e }(), func() error { _, e := service.SetModelActive(ctx, unknown, false); return e }()} {
		if !errors.Is(err, ErrNotFound) {
			t.Fatal("unknown catalog result", err)
		}
	}
	if _, err := service.GetModel(ctx, "invalid"); !errors.Is(err, ErrInvalidID) {
		t.Fatal(err)
	}
	if _, err := service.CreateBrand(ctx, "Custom"); !errors.Is(err, ErrCustomName) {
		t.Fatal(err)
	}
	var custom int
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM chassis_brands WHERE lower(name)='custom')+(SELECT count(*) FROM chassis_models WHERE lower(name)='custom')").Scan(&custom); err != nil || custom != 0 {
		t.Fatal("custom record present", custom, err)
	}
}
