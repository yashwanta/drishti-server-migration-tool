// Package server wires HTTP handlers, middleware, and graceful shutdown.
package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/drishti/hypershift/internal/auth"
	"github.com/drishti/hypershift/internal/config"
	"github.com/drishti/hypershift/internal/logging"
)

// Server is the HTTP entrypoint. It owns its router and lifecycle.
type Server struct {
	cfg    config.Config
	log    *logging.Logger
	router *http.ServeMux
	root   *http.ServeMux
	auth   *auth.Service
}

func New(cfg config.Config, log *logging.Logger, authService ...*auth.Service) *Server {
	var service *auth.Service
	if len(authService) > 0 {
		service = authService[0]
	}
	s := &Server{cfg: cfg, log: log, router: http.NewServeMux(), root: http.NewServeMux(), auth: service}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return recovery(s.log, cors(s.cfg.AllowedOrigin, s.root)) }

// Run blocks until the server stops. It performs graceful shutdown on ctx done.
func (s *Server) Run(ctx context.Context) error {
	httpd := &http.Server{
		Addr:         s.cfg.HTTPAddr,
		Handler:      s.Handler(),
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
	}
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("http server starting", logging.Field{Key: "addr", Value: s.cfg.HTTPAddr}, logging.Field{Key: "mode", Value: string(s.cfg.Mode)})
		if err := httpd.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		s.log.Info("shutdown requested")
	case err := <-errCh:
		return err
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTime)
	defer cancel()
	return httpd.Shutdown(shutCtx)
}

func (s *Server) routes() {
	s.root.HandleFunc("GET /healthz", s.health)
	s.root.HandleFunc("GET /livez", s.live)
	if s.auth == nil {
		s.registerUnavailableAuthRoutes()
		return
	}
	s.root.HandleFunc("POST /api/v1/auth/login", s.auth.LoginHandler)
	s.root.Handle("GET /api/v1/auth/session", s.auth.RequireSession(http.HandlerFunc(s.auth.SessionHandler)))
	s.root.Handle("POST /api/v1/auth/logout", s.auth.RequireSession(http.HandlerFunc(s.auth.LogoutHandler)))
	s.root.Handle("POST /api/v1/auth/change-password", s.auth.RequireSession(http.HandlerFunc(s.auth.ChangePasswordHandler)))
	s.router.HandleFunc("GET /api/v1/users", s.auth.ListUsersHandler)
	s.router.HandleFunc("POST /api/v1/users", s.auth.CreateUserHandler)
	s.router.HandleFunc("POST /api/v1/users/{id}/deactivate", s.auth.DeactivateUserHandler)
	s.protectAPI()
}

func (s *Server) registerUnavailableAuthRoutes() {
	deny := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication unavailable", "code": "unauthorized"})
	})
	for _, pattern := range protectedPatterns() {
		s.root.Handle(pattern.pattern, deny)
	}
}
