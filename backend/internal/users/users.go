package users

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrNotFound         = errors.New("user not found")
	ErrNicknameConflict = errors.New("nickname already in use")
	ErrEmailConflict    = errors.New("email already in use")
	ErrInvalidNickname  = errors.New("nickname must contain 1 to 64 characters")
)

// User is the application profile exposed to its owner. Google identity is not public.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Nickname  string    `json:"nickname"`
	AvatarURL *string   `json:"avatar_url"`
	CreatedAt time.Time `json:"created_at"`
}

// Identity contains verified provider claims, never client-supplied profile fields.
type Identity struct{ Subject, Email, Name, AvatarURL string }

type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service                          { return &Service{repo: repo} }
func (s *Service) Get(ctx context.Context, id string) (User, error) { return s.repo.Get(ctx, id) }

func (s *Service) Login(ctx context.Context, identity Identity) (User, error) {
	identity.Email = strings.ToLower(strings.TrimSpace(identity.Email))
	if identity.Subject == "" || identity.Email == "" {
		return User{}, errors.New("incomplete verified identity")
	}
	nickname := initialNickname(identity)
	for attempt := 0; attempt < 6; attempt++ {
		user, err := s.repo.Upsert(ctx, identity, nickname)
		if !errors.Is(err, ErrNicknameConflict) {
			return user, err
		}
		var suffix [6]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return User{}, err
		}
		nickname = initialNickname(identity) + "-" + hex.EncodeToString(suffix[:])
	}
	return User{}, ErrNicknameConflict
}

func (s *Service) UpdateNickname(ctx context.Context, id, nickname string) (User, error) {
	nickname = strings.TrimSpace(nickname)
	if !utf8.ValidString(nickname) || nickname == "" || utf8.RuneCountInString(nickname) > 64 || strings.ContainsRune(nickname, 0) {
		return User{}, ErrInvalidNickname
	}
	return s.repo.UpdateNickname(ctx, id, nickname)
}

func initialNickname(identity Identity) string {
	candidate := strings.TrimSpace(identity.Name)
	if candidate == "" {
		candidate = strings.SplitN(identity.Email, "@", 2)[0]
	}
	var result []rune
	for _, r := range candidate {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			result = append(result, r)
		} else if unicode.IsSpace(r) {
			result = append(result, '-')
		}
		if len(result) == 40 {
			break
		}
	}
	name := strings.Trim(string(result), "-_")
	if name == "" {
		return "driver"
	}
	return name
}
