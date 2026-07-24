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

// cors adds permissive dev CORS headers. Tighten for production in Phase 9.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
