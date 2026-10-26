// Package httpjson owns no-store JSON response encoding, not domain error policy.
package httpjson

import (
	"encoding/json"
	"net/http"
)

// Write preserves the API's JSON content type, no-store policy and encoder newline.
// NoContent never writes a body, including when used with a recorder in tests.
func Write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(value)
	}
}

// Error encodes a caller-selected safe message; domain status mapping stays local.
func Error(w http.ResponseWriter, status int, message string) {
	Write(w, status, struct {
		Error string `json:"error"`
	}{message})
}
