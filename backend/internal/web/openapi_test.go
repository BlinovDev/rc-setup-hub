package web

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/api"
)

func TestOpenAPIDelivery(t *testing.T) {
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context) error { t.Fatal("documentation must not query the database"); return nil })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/openapi.yaml", nil))
	if response.Code != 200 || response.Header().Get("Content-Type") != "application/yaml" || !bytes.Equal(response.Body.Bytes(), api.OpenAPI) {
		t.Fatalf("contract delivery: %d %v", response.Code, response.Header())
	}
}
