// Command hypershift is the DRISHTI HyperShift API orchestrator.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/drishti/hypershift/internal/api"
	"github.com/drishti/hypershift/internal/config"
	"github.com/drishti/hypershift/internal/logging"
	"github.com/drishti/hypershift/internal/mock"
	"github.com/drishti/hypershift/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "hypershift: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	log := logging.New(os.Stdout, cfg.LogLevel)
	log.Info("starting DRISHTI HyperShift orchestrator",
		logging.Field{Key: "mode", Value: string(cfg.Mode)},
		logging.Field{Key: "addr", Value: cfg.HTTPAddr},
	)
	if cfg.Mode == config.ModeMock {
		log.Info("running in MOCK mode: no platform connections will be attempted")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	provider := mock.New()
	plans := api.NewPlanStore()
	audit := api.NewAuditStore()
	handlers := api.NewHandlers(provider, plans, audit)

	srv := server.New(cfg, log)
	handlers.Register(srv.Router())

	return srv.Run(ctx)
}
