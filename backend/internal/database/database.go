package database

import (
	"context"
	"errors"

	"github.com/BlinovDev/rc-setup-hub/backend/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Open creates a UTC-configured pool and verifies connectivity before returning.
// Connection errors are deliberately sanitized because DSNs can contain secrets.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.New("invalid DATABASE_URL")
	}
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("create database pool failed")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.New("database connection failed")
	}
	return pool, nil
}

// MigrationProvider shares the pool through pgx's database/sql adapter.
// The caller must close the provider before closing the pool.
func MigrationProvider(pool *pgxpool.Pool) (*goose.Provider, error) {
	db := stdlib.OpenDBFromPool(pool)
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return provider, nil
}
