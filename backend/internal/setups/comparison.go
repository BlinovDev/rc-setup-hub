package setups

import "context"

// Comparison preserves request order and the complete existing Setup contract.
// No values are defaulted, normalized or calculated by a read-only comparison.
type Comparison struct {
	Left  Setup `json:"left"`
	Right Setup `json:"right"`
}

func comparisonIDs(left, right string) ([]string, error) {
	a, err := canonicalUUID(&left, "left_id")
	if err != nil {
		return nil, err
	}
	b, err := canonicalUUID(&right, "right_id")
	if err != nil {
		return nil, err
	}
	if *a == *b {
		return nil, invalid("comparison requires two distinct setup IDs")
	}
	return []string{*a, *b}, nil
}

// Compare authorizes both sides in one statement snapshot, not two independent
// detail reads. Missing or inaccessible on either side produces no partial result.
func (s *Service) Compare(ctx context.Context, viewerID, left, right string) (Comparison, error) {
	ids, err := comparisonIDs(left, right)
	if err != nil {
		return Comparison{}, err
	}
	return s.repo.compare(ctx, viewerID, ids)
}

func (r *Repository) compare(ctx context.Context, viewerID string, ids []string) (Comparison, error) {
	rows, err := r.pool.Query(ctx, `WITH permitted AS MATERIALIZED (
 SELECT s.id FROM setups s WHERE s.id=ANY($1::uuid[]) AND `+visibleSetupPredicate("$2")+`
 ) SELECT `+displayColumns+` FROM setups s`+displayJoins+`
 JOIN permitted p ON p.id=s.id WHERE (SELECT count(*) FROM permitted)=2
 ORDER BY array_position($1::uuid[],s.id)`, ids, viewerID)
	if err != nil {
		return Comparison{}, err
	}
	defer rows.Close()
	result := []Setup{}
	for rows.Next() {
		setup, err := scan(rows)
		if err != nil {
			return Comparison{}, err
		}
		result = append(result, setup)
	}
	if err := rows.Err(); err != nil {
		return Comparison{}, err
	}
	if len(result) != 2 {
		return Comparison{}, ErrNotFound
	}
	return Comparison{Left: result[0], Right: result[1]}, nil
}
