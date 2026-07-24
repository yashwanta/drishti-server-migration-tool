package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr       string
	WorkspaceRoot  string
	MaxWorkspaceGB int64
	ShutdownTime   time.Duration
	LogLevel       string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:       envOrDefault("DRISHTI_WORKER_ADDR", ":8090"),
		WorkspaceRoot:  strings.TrimSpace(os.Getenv("DRISHTI_WORKSPACE_ROOT")),
		MaxWorkspaceGB: 100,
		ShutdownTime:   10 * time.Second,
		LogLevel:       strings.ToLower(strings.TrimSpace(os.Getenv("DRISHTI_LOG_LEVEL"))),
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.WorkspaceRoot == "" {
		cfg.WorkspaceRoot = os.TempDir()
	}
	if cfg.MaxWorkspaceGB <= 0 {
		return cfg, fmt.Errorf("invalid MaxWorkspaceGB %d", cfg.MaxWorkspaceGB)
	}
	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}