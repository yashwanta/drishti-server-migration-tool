package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr          string
	WorkspaceRoot     string
	MaxWorkspaceGB    int64
	Mode              string
	EnableConversion  bool
	ConversionTimeout time.Duration
	ShutdownTime      time.Duration
	LogLevel          string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:          envOrDefault("DRISHTI_WORKER_ADDR", ":8090"),
		WorkspaceRoot:     strings.TrimSpace(os.Getenv("DRISHTI_WORKSPACE_ROOT")),
		MaxWorkspaceGB:    int64Env("DRISHTI_MAX_WORKSPACE_GB", 100),
		Mode:              strings.ToLower(strings.TrimSpace(os.Getenv("DRISHTI_MODE"))),
		EnableConversion:  strings.EqualFold(strings.TrimSpace(os.Getenv("DRISHTI_ENABLE_WORKER_CONVERSION")), "true"),
		ConversionTimeout: durationEnv("DRISHTI_CONVERSION_TIMEOUT", 24*time.Hour),
		ShutdownTime:      10 * time.Second,
		LogLevel:          strings.ToLower(strings.TrimSpace(os.Getenv("DRISHTI_LOG_LEVEL"))),
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.Mode == "" {
		cfg.Mode = "mock"
	}
	switch cfg.Mode {
	case "mock", "lab", "live", "production":
	default:
		return cfg, fmt.Errorf("invalid DRISHTI_MODE %q", cfg.Mode)
	}
	if cfg.EnableConversion && cfg.Mode != "lab" && cfg.Mode != "live" {
		return cfg, fmt.Errorf("worker conversion can only be enabled in lab or live mode")
	}
	if cfg.WorkspaceRoot == "" {
		cfg.WorkspaceRoot = os.TempDir()
	}
	if cfg.MaxWorkspaceGB <= 0 {
		return cfg, fmt.Errorf("invalid MaxWorkspaceGB %d", cfg.MaxWorkspaceGB)
	}
	return cfg, nil
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return fallback
	}
	return duration
}

func int64Env(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envOrDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
