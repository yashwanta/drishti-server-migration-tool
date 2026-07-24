// Package api implements REST handlers for connections, inventory, and draft
// migration plans. Phase 0/1 exposes read-only endpoints backed by the mock
// provider and an in-memory plan store.
package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/drishti/hypershift/internal/domain"
)

// errorBody is the canonical JSON error shape.
type errorBody struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

func writeErr(w http.ResponseWriter, code int, e errorBody) {
	writeJSON(w, code, e)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// decodeJSON decodes a request body, limiting size to prevent abuse.
func decodeJSON(r *http.Request, dst any, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	r.Body = http.MaxBytesReader(nil, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// resolveMode is true when the app runs in mock mode for safety assertions.
func isRole(role domain.Role, want domain.Role) bool { return role == want }

// normalizeID trims and lowercases trailing non-id characters from user input
// used only for lookups; it does not transform stored values.
func normalizeID(in string) string { return strings.TrimSpace(in) }
