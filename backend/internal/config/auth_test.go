package config

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
)

func authEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ADMIN_EMAILS", "")
	for key, value := range map[string]string{
		"GOOGLE_CLIENT_ID": "test-client", "GOOGLE_CLIENT_SECRET": "test-secret",
		"GOOGLE_REDIRECT_URL":   "http://localhost:8080/auth/google/callback",
		"SESSION_SECRET":        base64.StdEncoding.EncodeToString([]byte(strings.Repeat("s", 32))),
		"SESSION_COOKIE_SECURE": "false", "SESSION_SAME_SITE": "lax", "ALLOWED_ORIGINS": "http://localhost:5173",
	} {
		t.Setenv(key, value)
	}
}
func TestAuthConfiguration(t *testing.T) {
	authEnv(t)
	cfg, err := LoadAuth()
	if err != nil || cfg.CookieSecure || cfg.SameSite != http.SameSiteLaxMode || len(cfg.AllowedOrigins) != 1 {
		t.Fatalf("local auth config: %+v %v", cfg, err)
	}
	for _, tc := range []struct{ key, value string }{
		{"GOOGLE_CLIENT_ID", ""}, {"GOOGLE_CLIENT_SECRET", ""}, {"GOOGLE_REDIRECT_URL", ""},
		{"GOOGLE_REDIRECT_URL", "http://example.com/auth/google/callback"},
		{"GOOGLE_REDIRECT_URL", "http://localhost:8080/wrong"},
		{"GOOGLE_REDIRECT_URL", "http://localhost:8080/auth/google/callback?x=y"},
		{"SESSION_SECRET", "secret"}, {"SESSION_SECRET", base64.StdEncoding.EncodeToString([]byte("short"))},
		{"SESSION_COOKIE_SECURE", "invalid"}, {"SESSION_SAME_SITE", "invalid"}, {"SESSION_SAME_SITE", "none"},
		{"ALLOWED_ORIGINS", "*"}, {"ALLOWED_ORIGINS", "http://*.example.com"}, {"ALLOWED_ORIGINS", "http://localhost:5173/path"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			authEnv(t)
			t.Setenv(tc.key, tc.value)
			if _, err := LoadAuth(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	t.Run("secure cross site", func(t *testing.T) {
		authEnv(t)
		t.Setenv("SESSION_COOKIE_SECURE", "true")
		t.Setenv("SESSION_SAME_SITE", "none")
		t.Setenv("GOOGLE_REDIRECT_URL", "https://api.example.com/auth/google/callback")
		cfg, err := LoadAuth()
		if err != nil || !cfg.CookieSecure || cfg.SameSite != http.SameSiteNoneMode {
			t.Fatal("secure settings", err)
		}
	})
}
