package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	cfg := mustCfg()
	srv := New(cfg, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", srv.health)
	mux.HandleFunc("GET /livez", srv.live)

	r := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %v", resp["status"])
	}
}

func TestLivezEndpoint(t *testing.T) {
	srv := New(mustCfg(), nil)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", srv.live)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/livez", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestRecoveryCatchesPanic(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /boom", func(w http.ResponseWriter, r *http.Request) {
		panic("kaboom")
	})
	handler := recovery(nil, mux)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/boom", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if !strings.Contains(w.Body.String(), "internal server error") {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestCorsSetsHeaders(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /x", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	w := httptest.NewRecorder()
	cors(mux).ServeHTTP(w, httptest.NewRequest("OPTIONS", "/x", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("options status = %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("missing CORS header")
	}
}