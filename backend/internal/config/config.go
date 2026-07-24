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
	HTTPAddr     string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	ShutdownTime time.Duration

	// Mode controls runtime behavior. "mock" never connects to any platform.
	// "lab" may connect to explicitly configured lab endpoints only.
	// "production" requires all safety interlocks to be explicitly enabled.
	Mode RunMode

	// DBURL is the PostgreSQL connection string. May be empty in mock mode
	// where an in-memory store is used for development.
	DBURL string

	// LogLevel controls structured log verbosity.
	LogLevel string
}

// RunMode names a deployment safety tier.
type RunMode string

const (
	ModeMock       RunMode = "mock"
	ModeLab        RunMode = "lab"
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
	case ModeMock, ModeLab, ModeProduction:
	default:
		return Config{}, fmt.Errorf("invalid DRISHTI_MODE %q: must be one of mock, lab, production", mode)
	}

	addr := strings.TrimSpace(os.Getenv("DRISHTI_HTTP_ADDR"))
	if addr == "" {
		addr = ":8080"
	}

	cfg := Config{
		HTTPAddr:     addr,
		ReadTimeout:  durationEnv("DRISHTI_READ_TIMEOUT", 15*time.Second),
		WriteTimeout: durationEnv("DRISHTI_WRITE_TIMEOUT", 30*time.Second),
		ShutdownTime: durationEnv("DRISHTI_SHUTDOWN_TIMEOUT", 10*time.Second),
		Mode:         mode,
		DBURL:        strings.TrimSpace(os.Getenv("DRISHTI_DB_URL")),
		LogLevel:     strings.ToLower(strings.TrimSpace(os.Getenv("DRISHTI_LOG_LEVEL"))),
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.Mode != ModeMock && cfg.DBURL == "" {
		return cfg, fmt.Errorf("DRISHTI_DB_URL is required when DRISHTI_MODE is %q", mode)
	}
	return cfg, nil
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
