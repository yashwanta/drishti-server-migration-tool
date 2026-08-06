package server

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/drishti/hypershift/internal/logging"
)

var startedAt = time.Now()

type healthResp struct {
	Status    string `json:"status"`
	Mode      string `json:"mode"`
	Uptime    string `json:"uptime"`
	GoVersion string `json:"go_version"`
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResp{
		Status:    "ok",
		Mode:      string(s.cfg.Mode),
		Uptime:    time.Since(startedAt).Round(time.Second).String(),
		GoVersion: runtime.Version(),
	})
}

// live is a cheaper liveness probe with no allocations.
func (s *Server) live(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// recovery catches panics so a single handler fault never crashes the process.
func recovery(log *logging.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if log != nil {
					log.Error("http panic recovered", logging.Field{Key: "panic", Value: rec}, logging.Field{Key: "path", Value: r.URL.Path})
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"internal server error"}`))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// cors permits same-origin requests by default and one explicitly configured
// browser origin when credentials are used.
func cors(allowedOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowedOrigin != "" && origin == allowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token")
		if r.Method == http.MethodOptions {
			if origin != "" && origin != allowedOrigin {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
