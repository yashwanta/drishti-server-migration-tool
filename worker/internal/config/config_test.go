package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DRISHTI_WORKER_ADDR", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":8090" {
		t.Errorf("addr = %q", cfg.HTTPAddr)
	}
	if cfg.MaxWorkspaceGB <= 0 {
		t.Error("expected positive workspace cap")
	}
}