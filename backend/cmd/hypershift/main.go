// Command hypershift is the DRISHTI HyperShift API orchestrator.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/drishti/hypershift/internal/api"
	"github.com/drishti/hypershift/internal/auth"
	"github.com/drishti/hypershift/internal/config"
	"github.com/drishti/hypershift/internal/converter"
	"github.com/drishti/hypershift/internal/credential"
	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/job"
	"github.com/drishti/hypershift/internal/logging"
	"github.com/drishti/hypershift/internal/mock"
	"github.com/drishti/hypershift/internal/platform/adapterfactory"
	"github.com/drishti/hypershift/internal/platform/proxmox"
	"github.com/drishti/hypershift/internal/server"
	"github.com/drishti/hypershift/internal/store"
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
	if cfg.Mode != config.ModeMock {
		provider = mock.NewEmpty()
	}
	plans := api.NewPlanStore()
	audit := api.NewAuditStore()
	jobState := job.NewState()
	var database *store.Postgres
	if cfg.Mode != config.ModeMock {
		databaseContext, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		database, err = store.OpenPostgres(databaseContext, cfg.DBURL)
		if err != nil {
			return fmt.Errorf("initialize durable store: %w", err)
		}
		defer database.Close()
		plans = api.NewPersistentPlanStore(database)
		audit = api.NewPersistentAuditStore(database)
		jobState = job.NewPersistentState(database)
	}

	var userStore auth.UserStore
	if cfg.Mode == config.ModeMock {
		seed, err := auth.LoadUserRecords(cfg.AuthUsersFile)
		if err != nil {
			return fmt.Errorf("load mock authentication users: %w", err)
		}
		records, err := auth.RecordsFromCredentials(seed, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("validate mock authentication users: %w", err)
		}
		userStore, err = auth.NewMemoryUserStore(records, audit.AppendContext)
		if err != nil {
			return fmt.Errorf("initialize mock authentication users: %w", err)
		}
	} else {
		count, err := database.CountUsers(ctx)
		if err != nil {
			return fmt.Errorf("count durable authentication users: %w", err)
		}
		if count == 0 {
			seed, err := auth.LoadUserRecords(cfg.AuthUsersFile)
			if err != nil {
				return fmt.Errorf("load first-run authentication bootstrap: %w", err)
			}
			records, err := auth.RecordsFromCredentials(seed, time.Now().UTC())
			if err != nil {
				return fmt.Errorf("validate authentication bootstrap: %w", err)
			}
			imported, err := database.BootstrapUsers(ctx, records)
			if err != nil {
				return fmt.Errorf("bootstrap durable authentication users: %w", err)
			}
			if imported {
				log.Info("imported first-run authentication users into PostgreSQL")
			}
		}
		userStore = database
	}
	authService, err := auth.NewWithStore(userStore, cfg.SessionTTL, cfg.SessionSecure)
	if err != nil {
		return fmt.Errorf("initialize authentication: %w", err)
	}
	creds := credential.NewMemory()
	handlers := api.NewHandlersWithRuntime(provider, plans, audit, cfg.Mode, nil, creds)
	if cfg.Mode != config.ModeMock {
		handlers = api.NewHandlersWithRuntime(provider, plans, audit, cfg.Mode, proxmox.NewProbe(), creds)
	}

	// Adapter selection is real outside mock mode. Mutating HTTP execution is
	// enabled later only by the exact lab-mode, dual-interlock gate.
	workDir := cfg.WorkspaceRoot
	factory, err := adapterfactory.New(cfg, creds)
	if err != nil {
		return fmt.Errorf("select platform adapters: %w", err)
	}
	jobEng := job.NewEngine(jobState, factory, workDir, func(ctx context.Context, event domain.AuditEvent) error {
		return audit.AppendContext(ctx, event)
	})
	diskConverter := converter.DiskConverter(converter.NewMock())
	if cfg.Mode != config.ModeMock {
		diskConverter, err = converter.NewWorkerClient(cfg.WorkerURL, nil)
		if err != nil {
			return fmt.Errorf("configure conversion worker: %w", err)
		}
	}
	jobEng.SetDiskConverter(diskConverter)
	jobEng.SetPlanSaver(func(ctx context.Context, plan domain.Plan) error {
		return plans.PutContext(ctx, plan)
	})
	mh := api.NewMigrationHandlers(handlers, jobEng, factory, workDir)
	mh.ConfigureLabExecution(cfg.Mode, cfg.EnableLabMigration, cfg.EnablePlatformMutation)
	if cfg.Mode == config.ModeLab && cfg.EnableLabMigration && cfg.EnablePlatformMutation {
		log.Info("lab-only migration execute, cutover, and rollback routes enabled by both explicit interlocks")
	} else {
		log.Info("migration execute, cutover, and rollback routes remain locked for this runtime")
	}

	srv := server.New(cfg, log, authService)
	handlers.Register(srv.Router())
	mh.RegisterMigration(srv.Router())

	return srv.Run(ctx)
}
