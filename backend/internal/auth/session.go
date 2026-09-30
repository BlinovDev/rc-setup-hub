package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/gorilla/securecookie"
)

const sessionCookie = "rc_session"
const stateCookie = "rc_oauth_state"
const sessionTTL = 24 * time.Hour
const stateTTL = 10 * time.Minute
const maxEntries = 10000

type session struct {
	userID  string
	expires time.Time
}
type flow struct {
	nonce, verifier string
	expires         time.Time
}

// Sessions uses established signed cookies containing only opaque random handles.
// User IDs and single-use OAuth flow data stay in this process; logout revokes the handle.
type Sessions struct {
	mu                       sync.Mutex
	sessions                 map[string]session
	flows                    map[string]flow
	sessionCodec, stateCodec *securecookie.SecureCookie
	secure                   bool
	sameSite                 http.SameSite
	now                      func() time.Time
}

func NewSessions(c config.Auth) *Sessions {
	return &Sessions{sessions: make(map[string]session), flows: make(map[string]flow),
		sessionCodec: securecookie.New(c.SessionSecret, nil).MaxAge(int(sessionTTL.Seconds())),
		stateCodec:   securecookie.New(c.SessionSecret, nil).MaxAge(int(stateTTL.Seconds())),
		secure:       c.CookieSecure, sameSite: c.SameSite, now: time.Now}
}
func randomToken() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}
func (s *Sessions) prune() {
	now := s.now()
	for key, value := range s.sessions {
		if !now.Before(value.expires) {
			delete(s.sessions, key)
		}
	}
	for key, value := range s.flows {
		if !now.Before(value.expires) {
			delete(s.flows, key)
		}
	}
}
func (s *Sessions) cookie(w http.ResponseWriter, name, value string, maxAge int) {
	path, sameSite := "/", s.sameSite
	if name == stateCookie {
		path = "/auth/google"
		sameSite = http.SameSiteLaxMode
	}
	expires := s.now().Add(time.Duration(maxAge) * time.Second)
	if maxAge < 0 {
		expires = time.Unix(1, 0)
	}
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: path, MaxAge: maxAge, Expires: expires,
		HttpOnly: true, Secure: s.secure, SameSite: sameSite})
}
func decode(r *http.Request, name string, codec *securecookie.SecureCookie) (string, error) {
	cookie, err := r.Cookie(name)
	if err != nil {
		return "", err
	}
	var token string
	err = codec.Decode(name, cookie.Value, &token)
	return token, err
}
func (s *Sessions) Start(w http.ResponseWriter, nonce, verifier string) (string, error) {
	state, err := randomToken()
	if err != nil {
		return "", err
	}
	value, err := s.stateCodec.Encode(stateCookie, state)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	if len(s.flows) >= maxEntries {
		return "", errors.New("OAuth flow capacity reached")
	}
	s.flows[state] = flow{nonce: nonce, verifier: verifier, expires: s.now().Add(stateTTL)}
	s.cookie(w, stateCookie, value, int(stateTTL.Seconds()))
	return state, nil
}
func (s *Sessions) Consume(w http.ResponseWriter, r *http.Request, state string) (flow, error) {
	s.cookie(w, stateCookie, "", -1)
	token, err := decode(r, stateCookie, s.stateCodec)
	if err != nil || !equal(token, state) || state == "" {
		return flow{}, errors.New("invalid OAuth state")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	f, ok := s.flows[token]
	delete(s.flows, token)
	if !ok {
		return flow{}, errors.New("expired or reused OAuth state")
	}
	return f, nil
}
func (s *Sessions) Issue(w http.ResponseWriter, r *http.Request, userID string) error {
	token, err := randomToken()
	if err != nil {
		return err
	}
	value, err := s.sessionCodec.Encode(sessionCookie, token)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	if len(s.sessions) >= maxEntries {
		return errors.New("session capacity reached")
	}
	if old, err := decode(r, sessionCookie, s.sessionCodec); err == nil {
		delete(s.sessions, old)
	}
	s.sessions[token] = session{userID: userID, expires: s.now().Add(sessionTTL)}
	s.cookie(w, sessionCookie, value, int(sessionTTL.Seconds()))
	return nil
}
func (s *Sessions) UserID(r *http.Request) (string, error) {
	token, err := decode(r, sessionCookie, s.sessionCodec)
	if err != nil {
		return "", errors.New("invalid session")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.sessions[token]
	if !ok || !s.now().Before(record.expires) {
		delete(s.sessions, token)
		return "", errors.New("expired session")
	}
	return record.userID, nil
}
func (s *Sessions) Logout(w http.ResponseWriter, r *http.Request) {
	if token, err := decode(r, sessionCookie, s.sessionCodec); err == nil {
		s.mu.Lock()
		delete(s.sessions, token)
		s.mu.Unlock()
	}
	s.cookie(w, sessionCookie, "", -1)
}
