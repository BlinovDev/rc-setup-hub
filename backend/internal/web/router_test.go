package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouter(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
		status             int
	}{
		{"health", "GET", "/health", 200},
		{"missing", "GET", "/missing", 404},
		{"wrong method", "POST", "/health", 405},
		{"panic", "GET", "/panic", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			router := NewRouter(slog.New(slog.NewJSONHandler(&logs, nil)), func(context.Context) error { return nil })
			router.Get("/panic", func(http.ResponseWriter, *http.Request) { panic("private detail") })
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d", response.Code, tc.status)
			}
			if tc.path == "/health" && tc.method == "GET" {
				if response.Header().Get("Content-Type") != "application/json" || response.Body.String() != "{\"status\":\"ok\"}\n" {
					t.Fatalf("unexpected health response: %v %s", response.Header(), response.Body)
				}
			}
			if tc.path == "/panic" && strings.Contains(response.Body.String(), "private detail") {
				t.Fatal("panic detail leaked in response")
			}
			lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
			var entry struct {
				Method   string `json:"method"`
				Path     string `json:"path"`
				Status   int    `json:"status"`
				Duration int64  `json:"duration"`
			}
			if err := json.Unmarshal([]byte(lines[len(lines)-1]), &entry); err != nil {
				t.Fatal(err)
			}
			if entry.Method != tc.method || entry.Path != tc.path || entry.Status != tc.status || entry.Duration <= 0 {
				t.Fatalf("unexpected request log: %+v", entry)
			}
		})
	}
}

func TestHealthUnavailable(t *testing.T) {
	var logs bytes.Buffer
	router := NewRouter(slog.New(slog.NewJSONHandler(&logs, nil)), func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("health ping has no deadline")
		}
		return errors.New("private database detail")
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/health", nil))
	if w.Code != 503 || w.Body.String() != "{\"status\":\"unavailable\"}\n" || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected health failure: %d %s", w.Code, w.Body)
	}
}
