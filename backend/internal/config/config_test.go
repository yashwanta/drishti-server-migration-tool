package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DRISHTI_MODE", "mock")
	t.Setenv("DRISHTI_HTTP_ADDR", ":9090")
	t.Setenv("DRISHTI_LOG_LEVEL", "debug")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Mode != ModeMock {
		t.Errorf("mode = %q, want mock", cfg.Mode)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Errorf("addr = %q, want :9090", cfg.HTTPAddr)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("level = %q, want debug", cfg.LogLevel)
	}
	if cfg.DBURL != "" {
		t.Errorf("db url should be empty in mock, got %q", cfg.DBURL)
	}
}

func TestLoadRequiresDBInLabMode(t *testing.T) {
	t.Setenv("DRISHTI_MODE", "lab")
	t.Setenv("DRISHTI_DB_URL", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error requiring DB_URL in lab mode")
	}
}

func TestLoadRejectsInvalidMode(t *testing.T) {
	t.Setenv("DRISHTI_MODE", "danger")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestParseSeconds(t *testing.T) {
	t.Setenv("DRISHTI_MODE", "mock")
	t.Setenv("DRISHTI_READ_TIMEOUT", "5")
	cfg, _ := Load()
	if cfg.ReadTimeout.Seconds() != 5 {
		t.Errorf("read timeout = %v, want 5s", cfg.ReadTimeout)
	}
}