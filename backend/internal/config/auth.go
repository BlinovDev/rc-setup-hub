package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Auth contains server-only OAuth and cookie configuration.
type Auth struct {
	GoogleClientID, GoogleClientSecret, GoogleRedirectURL string
	SessionSecret                                         []byte
	CookieSecure                                          bool
	SameSite                                              http.SameSite
	AllowedOrigins                                        []string
}

// LoadAuth is separate so database migration commands do not require OAuth credentials.
func LoadAuth() (Auth, error) {
	var c Auth
	c.GoogleClientID = strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	c.GoogleClientSecret = strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET"))
	c.GoogleRedirectURL = strings.TrimSpace(os.Getenv("GOOGLE_REDIRECT_URL"))
	for _, field := range []struct{ name, value string }{{"GOOGLE_CLIENT_ID", c.GoogleClientID}, {"GOOGLE_CLIENT_SECRET", c.GoogleClientSecret}, {"GOOGLE_REDIRECT_URL", c.GoogleRedirectURL}} {
		if field.value == "" {
			return c, fmt.Errorf("%s is required", field.name)
		}
	}
	u, err := url.Parse(c.GoogleRedirectURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/auth/google/callback" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
		return c, errors.New("GOOGLE_REDIRECT_URL must be HTTPS (or loopback HTTP) with path /auth/google/callback")
	}
	c.SessionSecret, err = base64.StdEncoding.DecodeString(os.Getenv("SESSION_SECRET"))
	if err != nil || len(c.SessionSecret) < 32 {
		return c, errors.New("SESSION_SECRET must be base64 encoding of at least 32 random bytes")
	}
	if value := os.Getenv("SESSION_COOKIE_SECURE"); value != "" {
		c.CookieSecure, err = strconv.ParseBool(value)
		if err != nil {
			return c, errors.New("SESSION_COOKIE_SECURE must be a boolean")
		}
	}
	c.SameSite = http.SameSiteLaxMode
	switch os.Getenv("SESSION_SAME_SITE") {
	case "", "lax":
	case "none":
		c.SameSite = http.SameSiteNoneMode
	default:
		return c, errors.New("SESSION_SAME_SITE must be lax or none")
	}
	if c.SameSite == http.SameSiteNoneMode && !c.CookieSecure {
		return c, errors.New("SameSite=None requires SESSION_COOKIE_SECURE=true")
	}
	if u.Scheme == "https" && !c.CookieSecure {
		return c, errors.New("HTTPS Google redirect requires SESSION_COOKIE_SECURE=true")
	}
	for _, value := range strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",") {
		origin := strings.TrimSpace(value)
		if origin == "" {
			continue
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || strings.Contains(origin, "*") || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return c, errors.New("ALLOWED_ORIGINS must contain exact HTTP(S) origins")
		}
		c.AllowedOrigins = append(c.AllowedOrigins, origin)
	}
	return c, nil
}
