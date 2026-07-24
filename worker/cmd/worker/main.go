// Command worker is the DRISHTI HyperShift migration worker. In Phase 0 it
// exposes only a health endpoint and validates its configuration; no disk
// transfer or platform mutation is performed yet.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/drishti/hypershift-worker/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "worker: load config: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "worker starting addr=%s workspace=%s\n", cfg.HTTPAddr, cfg.WorkspaceRoot)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","service":"hypershift-worker"}`)
	})

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: mux}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		fmt.Fprintln(os.Stdout, "worker shutdown requested")
	case err := <-errCh:
		fmt.Fprintf(os.Stderr, "worker: %v\n", err)
		os.Exit(1)
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTime)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}