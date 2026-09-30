package chassis

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }
func catalogError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && (pgErr.ConstraintName == "chassis_brands_name_unique" || pgErr.ConstraintName == "chassis_models_name_unique") {
		return ErrDuplicate
	}
	return err
}
func scanBrand(row pgx.Row) (Brand, error) {
	var b Brand
	err := row.Scan(&b.ID, &b.Name, &b.IsActive, &b.CreatedAt, &b.UpdatedAt)
	b.CreatedAt = b.CreatedAt.UTC()
	b.UpdatedAt = b.UpdatedAt.UTC()
	return b, catalogError(err)
}
func scanModel(row pgx.Row) (Model, error) {
	var m Model
	err := row.Scan(&m.ID, &m.BrandID, &m.Name, &m.IsActive, &m.CreatedAt, &m.UpdatedAt)
	m.CreatedAt = m.CreatedAt.UTC()
	m.UpdatedAt = m.UpdatedAt.UTC()
	return m, catalogError(err)
}
func (r *Repository) ListBrands(ctx context.Context, activeOnly bool) ([]Brand, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text,name,is_active,created_at,updated_at FROM chassis_brands
        WHERE NOT $1::boolean OR is_active ORDER BY lower(name),id`, activeOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	brands := []Brand{}
	for rows.Next() {
		b, err := scanBrand(rows)
		if err != nil {
			return nil, err
		}
		brands = append(brands, b)
	}
	return brands, rows.Err()
}
func (r *Repository) GetBrand(ctx context.Context, id string) (Brand, error) {
	return scanBrand(r.pool.QueryRow(ctx, `SELECT id::text,name,is_active,created_at,updated_at FROM chassis_brands WHERE id=$1`, id))
}
func (r *Repository) CreateBrand(ctx context.Context, name string) (Brand, error) {
	return scanBrand(r.pool.QueryRow(ctx, `INSERT INTO chassis_brands(name) VALUES($1)
        RETURNING id::text,name,is_active,created_at,updated_at`, name))
}
func (r *Repository) RenameBrand(ctx context.Context, id, name string) (Brand, error) {
	return scanBrand(r.pool.QueryRow(ctx, `UPDATE chassis_brands SET name=$2,updated_at=now() WHERE id=$1
        RETURNING id::text,name,is_active,created_at,updated_at`, id, name))
}
func (r *Repository) SetBrandActive(ctx context.Context, id string, active bool) (Brand, error) {
	return scanBrand(r.pool.QueryRow(ctx, `UPDATE chassis_brands SET is_active=$2,updated_at=now() WHERE id=$1
        RETURNING id::text,name,is_active,created_at,updated_at`, id, active))
}
func (r *Repository) ListModels(ctx context.Context) ([]ModelState, error) {
	rows, err := r.pool.Query(ctx, `SELECT m.id::text,m.brand_id::text,m.name,m.is_active,m.created_at,m.updated_at,b.name,b.is_active
        FROM chassis_models m JOIN chassis_brands b ON b.id=m.brand_id ORDER BY lower(b.name),lower(m.name),m.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ModelState{}
	for rows.Next() {
		m, err := scanModelState(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}
func scanModelState(row pgx.Row) (ModelState, error) {
	var m ModelState
	err := row.Scan(&m.ID, &m.BrandID, &m.Name, &m.IsActive, &m.CreatedAt, &m.UpdatedAt, &m.BrandName, &m.BrandActive)
	m.CreatedAt = m.CreatedAt.UTC()
	m.UpdatedAt = m.UpdatedAt.UTC()
	return m, catalogError(err)
}
func (r *Repository) GetModel(ctx context.Context, id string) (ModelState, error) {
	return scanModelState(r.pool.QueryRow(ctx, `SELECT m.id::text,m.brand_id::text,m.name,m.is_active,m.created_at,m.updated_at,b.name,b.is_active
        FROM chassis_models m JOIN chassis_brands b ON b.id=m.brand_id WHERE m.id=$1`, id))
}
func (r *Repository) ActiveModels(ctx context.Context, brandID string) ([]Model, error) {
	rows, err := r.pool.Query(ctx, `SELECT m.id::text,m.brand_id::text,m.name,m.is_active,m.created_at,m.updated_at
        FROM chassis_models m JOIN chassis_brands b ON b.id=m.brand_id
        WHERE m.brand_id=$1 AND m.is_active AND b.is_active ORDER BY lower(m.name),m.id`, brandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Model{}
	for rows.Next() {
		m, err := scanModel(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}
func (r *Repository) CreateModel(ctx context.Context, brandID, name string) (Model, error) {
	// Lock the parent against disable while checking activation and inserting the model.
	// The CTE and INSERT run atomically in one statement; no schema trigger is needed.
	m, err := scanModel(r.pool.QueryRow(ctx, `WITH parent AS (
        SELECT id FROM chassis_brands WHERE id=$1 AND is_active FOR SHARE
    ) INSERT INTO chassis_models(brand_id,name) SELECT id,$2 FROM parent
        RETURNING id::text,brand_id::text,name,is_active,created_at,updated_at`, brandID, name))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := r.GetBrand(ctx, brandID); lookupErr != nil {
			return Model{}, lookupErr
		}
		return Model{}, ErrInactiveBrand
	}
	return m, err
}
func (r *Repository) RenameModel(ctx context.Context, id, name string) (Model, error) {
	return scanModel(r.pool.QueryRow(ctx, `UPDATE chassis_models SET name=$2,updated_at=now() WHERE id=$1
        RETURNING id::text,brand_id::text,name,is_active,created_at,updated_at`, id, name))
}
func (r *Repository) SetModelActive(ctx context.Context, id string, active bool) (Model, error) {
	return scanModel(r.pool.QueryRow(ctx, `UPDATE chassis_models SET is_active=$2,updated_at=now() WHERE id=$1
        RETURNING id::text,brand_id::text,name,is_active,created_at,updated_at`, id, active))
}
