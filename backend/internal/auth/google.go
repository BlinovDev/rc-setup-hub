package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Provider is the narrow verified identity boundary substituted in tests.
type Provider interface {
	AuthorizationURL(state, nonce, verifier string) string
	Exchange(ctx context.Context, code, nonce, verifier string) (users.Identity, error)
}

type Google struct {
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
}

func NewGoogle(c config.Auth) *Google {
	keyContext := oidc.ClientContext(context.Background(), &http.Client{Timeout: 10 * time.Second})
	keys := oidc.NewRemoteKeySet(keyContext, "https://www.googleapis.com/oauth2/v3/certs")
	return &Google{
		oauth: oauth2.Config{ClientID: c.GoogleClientID, ClientSecret: c.GoogleClientSecret, RedirectURL: c.GoogleRedirectURL,
			Endpoint: google.Endpoint, Scopes: []string{"openid", "email", "profile"}},
		verifier: oidc.NewVerifier("https://accounts.google.com", keys, &oidc.Config{ClientID: c.GoogleClientID}),
	}
}
func (g *Google) AuthorizationURL(state, nonce, verifier string) string {
	return g.oauth.AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce), oauth2.S256ChallengeOption(verifier))
}
func (g *Google) Exchange(ctx context.Context, code, nonce, verifier string) (users.Identity, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	token, err := g.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return users.Identity{}, errors.New("Google code exchange failed")
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return users.Identity{}, errors.New("Google ID token missing")
	}
	verified, err := g.verifier.Verify(ctx, raw)
	if err != nil {
		return users.Identity{}, errors.New("Google ID token invalid")
	}
	var claims struct {
		Email           string `json:"email"`
		EmailVerified   bool   `json:"email_verified"`
		Name            string `json:"name"`
		Picture         string `json:"picture"`
		AuthorizedParty string `json:"azp"`
	}
	if err := verified.Claims(&claims); err != nil {
		return users.Identity{}, errors.New("Google identity claims invalid")
	}
	if verified.Subject == "" || claims.Email == "" || !claims.EmailVerified ||
		subtle.ConstantTimeCompare([]byte(verified.Nonce), []byte(nonce)) != 1 ||
		(claims.AuthorizedParty != "" && claims.AuthorizedParty != g.oauth.ClientID) ||
		(len(verified.Audience) > 1 && claims.AuthorizedParty != g.oauth.ClientID) {
		return users.Identity{}, errors.New("Google identity claims invalid")
	}
	if verified.AccessTokenHash != "" {
		if err := verified.VerifyAccessToken(token.AccessToken); err != nil {
			return users.Identity{}, errors.New("Google access token binding invalid")
		}
	}
	return users.Identity{Subject: verified.Subject, Email: claims.Email, Name: claims.Name, AvatarURL: claims.Picture}, nil
}
