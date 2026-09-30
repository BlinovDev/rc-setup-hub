package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"golang.org/x/oauth2"
)

func TestGoogleAuthorizationURL(t *testing.T) {
	cfg := testConfig()
	cfg.GoogleClientID = "test-client"
	google := NewGoogle(cfg)
	u, err := url.Parse(google.AuthorizationURL("state", "nonce", "verifier"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Host != "accounts.google.com" || q.Get("response_type") != "code" || q.Get("scope") != "openid email profile" || q.Get("state") != "state" || q.Get("nonce") != "nonce" || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		t.Fatal("authorization params", u)
	}
}
func TestGoogleIDTokenVerification(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "test-key"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, field string
		value       any
		valid       bool
	}{
		{name: "valid", valid: true},
		{name: "invalid signature", field: "invalid_signature", value: true},
		{name: "wrong audience", field: "aud", value: "evil-client"},
		{name: "wrong issuer", field: "iss", value: "https://evil.example"},
		{name: "expired", field: "exp", value: time.Now().Add(-time.Hour).Unix()},
		{name: "wrong nonce", field: "nonce", value: "evil"},
		{name: "missing subject", field: "sub", value: ""},
		{name: "unverified email", field: "email_verified", value: false},
		{name: "wrong authorized party", field: "azp", value: "evil"},
		{name: "multiple audience no authorized party", field: "aud", value: []string{"test-client", "other-client"}},
		{name: "wrong access token hash", field: "at_hash", value: "wrong"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := map[string]any{"iss": "https://accounts.google.com", "aud": "test-client", "sub": "verified-subject",
				"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": "bound-nonce",
				"email": "verified@example.com", "email_verified": true, "name": "Verified Driver", "picture": "https://example.com/avatar"}
			if tc.field != "" {
				claims[tc.field] = tc.value
			}
			payload, _ := json.Marshal(claims)
			signed, err := signer.Sign(payload)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := signed.CompactSerialize()
			if err != nil {
				t.Fatal(err)
			}
			if tc.field == "invalid_signature" {
				parts := strings.Split(raw, ".")
				parts[2] = strings.Repeat("a", len(parts[2]))
				raw = strings.Join(parts, ".")
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/keys" {
					_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test-key", Algorithm: "RS256", Use: "sig"}}})
					return
				}
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				if r.Form.Get("code") != "test-code" || r.Form.Get("code_verifier") != "pkce-verifier" || r.Form.Get("grant_type") != "authorization_code" {
					t.Error("incorrect code exchange")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "private-access-token", "token_type": "Bearer", "id_token": raw})
			}))
			defer server.Close()
			g := &Google{oauth: oauth2.Config{ClientID: "test-client", ClientSecret: "secret", RedirectURL: "http://localhost:8080/auth/google/callback",
				Endpoint: oauth2.Endpoint{TokenURL: server.URL + "/token", AuthStyle: oauth2.AuthStyleInHeader}},
				verifier: oidc.NewVerifier("https://accounts.google.com", oidc.NewRemoteKeySet(context.Background(), server.URL+"/keys"), &oidc.Config{ClientID: "test-client"})}
			identity, err := g.Exchange(context.Background(), "test-code", "bound-nonce", "pkce-verifier")
			if tc.valid {
				if err != nil || identity.Subject != "verified-subject" || identity.Email != "verified@example.com" {
					t.Fatal(identity, err)
				}
			} else if err == nil {
				t.Fatal("invalid identity accepted")
			}
			if err != nil && strings.Contains(err.Error(), "private-access-token") {
				t.Fatal("token in error")
			}
		})
	}
}
