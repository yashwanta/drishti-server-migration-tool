// Package config loads, validates, and provides typed application configuration.
// All secrets remain in memory only; nothing is logged at this layer.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the resolved, validated application configuration.
type Config struct {
	HTTPAddr      string
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	ShutdownTime  time.Duration
	WorkspaceRoot string
	WorkerURL     string

	// Mode controls runtime behavior. "mock" never connects to any platform.
	// "lab" may connect to explicitly configured lab endpoints only.
	// "production" requires all safety interlocks to be explicitly enabled.
	Mode RunMode

	// DBURL is the PostgreSQL connection string. May be empty in mock mode
	// where an in-memory store is used for development.
	DBURL string

	// LogLevel controls structured log verbosity.
	LogLevel string

	// AuthUsersFile seeds mock mode and bootstraps an empty real-mode users
	// table once. PostgreSQL is authoritative after that first import.
	AuthUsersFile string
	SessionTTL    time.Duration
	SessionSecure bool
	AllowedOrigin string

	// EnableLabMigration unlocks real mutating migration calls in lab mode.
	// It is ignored in mock and production modes.
	EnableLabMigration bool

	// EnablePlatformMutation unlocks narrowly scoped source/target mutations in
	// lab and live modes. Production mutations remain prohibited.
	EnablePlatformMutation bool

	// MutationDenylist contains protected endpoint hostnames or host:port pairs.
	MutationDenylist []string

	// ProxmoxImportRoot is an absolute POSIX path mounted at the same location
	// on the worker and target Proxmox node. ImportDisk refuses paths outside it.
	ProxmoxImportRoot string

	// ProxmoxIsolatedBridges is the explicit allowlist of bridges on which a
	// migrated target may be created or booted before validation.
	ProxmoxIsolatedBridges []string
}

// RunMode names a deployment safety tier.
type RunMode string

const (
	ModeMock       RunMode = "mock"
	ModeLab        RunMode = "lab"
	ModeLive       RunMode = "live"
	ModeProduction RunMode = "production"
)

// Load reads configuration from the environment and validates it. Unknown
// variables are ignored; required ones are enforced.
func Load() (Config, error) {
	mode := RunMode(strings.ToLower(strings.TrimSpace(os.Getenv("DRISHTI_MODE"))))
	if mode == "" {
		mode = ModeMock
	}
	switch mode {
	case ModeMock, ModeLab, ModeLive, ModeProduction:
	default:
		return Config{}, fmt.Errorf("invalid DRISHTI_MODE %q: must be one of mock, lab, live, production", mode)
	}

	addr := strings.TrimSpace(os.Getenv("DRISHTI_HTTP_ADDR"))
	if addr == "" {
		addr = ":8080"
	}

	cfg := Config{
		HTTPAddr:               addr,
		ReadTimeout:            durationEnv("DRISHTI_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:           durationEnv("DRISHTI_WRITE_TIMEOUT", 30*time.Second),
		ShutdownTime:           durationEnv("DRISHTI_SHUTDOWN_TIMEOUT", 10*time.Second),
		WorkspaceRoot:          strings.TrimSpace(os.Getenv("DRISHTI_WORKSPACE_ROOT")),
		WorkerURL:              strings.TrimSpace(os.Getenv("DRISHTI_WORKER_URL")),
		Mode:                   mode,
		DBURL:                  strings.TrimSpace(os.Getenv("DRISHTI_DB_URL")),
		LogLevel:               strings.ToLower(strings.TrimSpace(os.Getenv("DRISHTI_LOG_LEVEL"))),
		AuthUsersFile:          strings.TrimSpace(os.Getenv("DRISHTI_AUTH_USERS_FILE")),
		SessionTTL:             durationEnv("DRISHTI_SESSION_TTL", 8*time.Hour),
		SessionSecure:          strings.EqualFold(strings.TrimSpace(os.Getenv("DRISHTI_SESSION_SECURE")), "true"),
		AllowedOrigin:          strings.TrimSpace(os.Getenv("DRISHTI_ALLOWED_ORIGIN")),
		EnableLabMigration:     strings.EqualFold(strings.TrimSpace(os.Getenv("DRISHTI_ENABLE_LAB_MIGRATION")), "true"),
		EnablePlatformMutation: strings.EqualFold(strings.TrimSpace(os.Getenv("DRISHTI_ENABLE_PLATFORM_MUTATION")), "true"),
		MutationDenylist:       splitCSV(os.Getenv("DRISHTI_MUTATION_DENYLIST")),
		ProxmoxImportRoot:      strings.TrimSpace(os.Getenv("DRISHTI_PROXMOX_IMPORT_ROOT")),
		ProxmoxIsolatedBridges: splitCSV(os.Getenv("DRISHTI_PROXMOX_ISOLATED_BRIDGES")),
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.WorkspaceRoot == "" {
		cfg.WorkspaceRoot = os.TempDir()
	}
	if cfg.Mode != ModeMock && cfg.DBURL == "" {
		return cfg, fmt.Errorf("DRISHTI_DB_URL is required when DRISHTI_MODE is %q", mode)
	}
	if cfg.Mode != ModeMock && cfg.WorkerURL == "" {
		return cfg, fmt.Errorf("DRISHTI_WORKER_URL is required when DRISHTI_MODE is %q", mode)
	}
	if cfg.Mode == ModeProduction && cfg.EnablePlatformMutation {
		return cfg, fmt.Errorf("DRISHTI_ENABLE_PLATFORM_MUTATION cannot be enabled in production mode")
	}
	if (cfg.Mode == ModeLive || cfg.Mode == ModeProduction) && !cfg.SessionSecure {
		return cfg, fmt.Errorf("DRISHTI_SESSION_SECURE=true is required in %s mode", cfg.Mode)
	}
	return cfg, nil
}

func splitCSV(raw string) []string {
	var values []string
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func durationEnv(key string, def time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		return time.Duration(secs) * time.Second
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}
	return def
}
