package users

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

var ErrInvalidProfile = errors.New("invalid profile update")

// PatchProfile uses RawMessage to distinguish omission from an explicit null clear.
// Only nickname rejects null; social links accept null or an empty string to clear.
type PatchProfile struct {
	Nickname     json.RawMessage `json:"nickname"`
	InstagramURL json.RawMessage `json:"instagram_url"`
	TikTokURL    json.RawMessage `json:"tiktok_url"`
	YouTubeURL   json.RawMessage `json:"youtube_url"`
}

var instagramPath = regexp.MustCompile(`^/[A-Za-z0-9._]{1,30}$`)
var tiktokPath = regexp.MustCompile(`^/@[A-Za-z0-9._]{1,24}$`)
var youtubePath = regexp.MustCompile(`^/(@[\p{L}\p{N}._-]{1,100}|(channel|c|user)/[A-Za-z0-9._-]{1,100})$`)

// ValidateSocialURL accepts profile URLs only, never platform redirect/short-link
// endpoints. No URL is fetched, and query/fragment/escaped path tricks are rejected.
func ValidateSocialURL(platform, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !utf8.ValidString(value) || len(value) > 2048 || strings.ContainsAny(value, "\\%\r\n\t") {
		return "", ErrInvalidProfile
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Opaque != "" || u.Port() != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(value, "#") {
		return "", ErrInvalidProfile
	}
	host := strings.ToLower(u.Host)
	if host != platform+".com" && host != "www."+platform+".com" {
		return "", ErrInvalidProfile
	}
	path := strings.TrimSuffix(u.Path, "/")
	valid := false
	switch platform {
	case "instagram":
		valid = instagramPath.MatchString(path)
		switch strings.ToLower(path) {
		case "/accounts", "/explore", "/direct", "/p", "/reel", "/reels", "/stories", "/about", "/redirect", "/l.php", "/..", "/.":
			valid = false
		}
	case "tiktok":
		valid = tiktokPath.MatchString(path)
	case "youtube":
		valid = youtubePath.MatchString(path)
	}
	if !valid {
		return "", ErrInvalidProfile
	}
	return "https://" + host + path, nil
}

func socialValue(raw json.RawMessage, platform string) (*string, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	var value *string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, true, ErrInvalidProfile
	}
	if value == nil {
		return nil, true, nil
	}
	normalized, err := ValidateSocialURL(platform, *value)
	if err != nil {
		return nil, true, err
	}
	if normalized == "" {
		return nil, true, nil
	}
	return &normalized, true, nil
}

func (s *Service) UpdateProfile(ctx context.Context, id string, input PatchProfile) (User, error) {
	var nickname *string
	if input.Nickname != nil {
		if err := json.Unmarshal(input.Nickname, &nickname); err != nil || nickname == nil {
			return User{}, ErrInvalidNickname
		}
		trimmed := strings.TrimSpace(*nickname)
		if !utf8.ValidString(trimmed) || trimmed == "" || utf8.RuneCountInString(trimmed) > 64 || strings.ContainsRune(trimmed, 0) {
			return User{}, ErrInvalidNickname
		}
		nickname = &trimmed
	}
	instagram, setInstagram, err := socialValue(input.InstagramURL, "instagram")
	if err != nil {
		return User{}, err
	}
	tiktok, setTikTok, err := socialValue(input.TikTokURL, "tiktok")
	if err != nil {
		return User{}, err
	}
	youtube, setYouTube, err := socialValue(input.YouTubeURL, "youtube")
	if err != nil {
		return User{}, err
	}
	if nickname == nil && !setInstagram && !setTikTok && !setYouTube {
		return User{}, ErrInvalidProfile
	}
	return s.repo.updateProfile(ctx, id, nickname, setInstagram, instagram, setTikTok, tiktok, setYouTube, youtube)
}

// Filter even persisted links before exposing them (including imported/legacy data).
func safeSocial(platform string, value *string) *string {
	if value == nil {
		return nil
	}
	normalized, err := ValidateSocialURL(platform, *value)
	if err != nil || normalized == "" {
		return nil
	}
	return &normalized
}
