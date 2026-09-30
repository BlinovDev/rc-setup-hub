// Package dbtest provides disposable, migrated PostgreSQL for integration tests.
package dbtest

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// New starts a real PostgreSQL container and applies all migrations.
// Docker must be running. Only -short explicitly skips integration tests.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("PostgreSQL integration test skipped with -short")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "run", "--detach", "--rm",
		"-e", "POSTGRES_USER=rc_setup_test", "-e", "POSTGRES_PASSWORD=rc_setup_test",
		"-e", "POSTGRES_DB=rc_setup_test", "-p", "127.0.0.1::5432", "postgres:17-alpine").CombinedOutput()
	if err != nil {
		t.Fatalf("start PostgreSQL (is Docker running?): %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	id := lines[len(lines)-1]
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "docker", "rm", "--force", id).CombinedOutput(); err != nil {
			t.Errorf("remove test container: %v: %s", err, out)
		}
	})
	out, err = exec.CommandContext(ctx, "docker", "port", id, "5432/tcp").Output()
	if err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("postgres://rc_setup_test:rc_setup_test@%s/rc_setup_test?sslmode=disable", strings.TrimSpace(string(out)))
	var pool *pgxpool.Pool
	for {
		attemptCtx, attemptCancel := context.WithTimeout(ctx, time.Second)
		pool, err = database.Open(attemptCtx, url)
		attemptCancel()
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("PostgreSQL readiness: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Cleanup(pool.Close)
	provider, err := database.MigrationProvider(pool)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	return pool
}
