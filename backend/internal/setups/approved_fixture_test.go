package setups

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// approveFixture explicitly models already-reviewed public records in older
// visibility/search tests. New moderation tests always use the real pending path.
func approveFixture(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE setups SET moderation_status='approved' WHERE id=$1 AND visibility='public'`, id); err != nil {
		t.Fatal(err)
	}
}
func createReviewedFixture(t *testing.T, s *Service, ctx context.Context, owner string, input CreateInput) (Setup, error) {
	t.Helper()
	setup, err := s.Create(ctx, owner, input)
	if err != nil {
		return setup, err
	}
	approveFixture(t, s.repo.pool, setup.ID)
	return s.repo.GetOwned(ctx, owner, setup.ID)
}
