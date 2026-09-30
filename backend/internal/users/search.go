package users

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

var ErrInvalidSearchQuery = errors.New("query must contain at most 64 characters without null bytes")

// PublicProfile is safe to expose to other application users.
type PublicProfile struct {
	ID        string  `json:"id"`
	Nickname  string  `json:"nickname"`
	AvatarURL *string `json:"avatar_url"`
}

func (s *Service) Search(ctx context.Context, callerID, query string) ([]PublicProfile, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []PublicProfile{}, nil
	}
	if !utf8.ValidString(query) || strings.ContainsRune(query, 0) || utf8.RuneCountInString(query) > 64 {
		return nil, ErrInvalidSearchQuery
	}
	return s.repo.Search(ctx, callerID, query)
}

// Search matches a literal case-insensitive nickname substring, never email.
func (r *Repository) Search(ctx context.Context, callerID, query string) ([]PublicProfile, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text,nickname,avatar_url FROM users WHERE id<>$1 AND strpos(lower(nickname),lower($2))>0 ORDER BY lower(nickname),id LIMIT 20`, callerID, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PublicProfile{}
	for rows.Next() {
		var profile PublicProfile
		if err := rows.Scan(&profile.ID, &profile.Nickname, &profile.AvatarURL); err != nil {
			return nil, err
		}
		result = append(result, profile)
	}
	return result, rows.Err()
}
