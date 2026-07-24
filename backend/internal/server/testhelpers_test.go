package server

import "github.com/drishti/hypershift/internal/config"

func mustCfg() config.Config {
	return config.Config{HTTPAddr: ":0", Mode: config.ModeMock, LogLevel: "info"}
}