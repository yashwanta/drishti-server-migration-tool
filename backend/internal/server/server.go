// Package server wires HTTP handlers, middleware, and graceful shutdown.
package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/drishti/hypershift/internal/config"
	"github.com/drishti/hypershift/internal/logging"
)

// Server is the HTTP entrypoint. It owns its router and lifecycle.
type Server struct {
	cfg    config.Config
	log    *logging.Logger
	router *http.ServeMux
}

func New(cfg config.Config, log *logging.Logger) *Server {
	s := &Server{cfg: cfg, log: log, router: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.router }

// Run blocks until the server stops. It performs graceful shutdown on ctx done.
func (s *Server) Run(ctx context.Context) error {
	httpd := &http.Server{
		Addr:         s.cfg.HTTPAddr,
		Handler:      recovery(s.log, cors(s.router)),
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
	s.router.HandleFunc("GET /healthz", s.health)
	s.router.HandleFunc("GET /livez", s.live)
}
