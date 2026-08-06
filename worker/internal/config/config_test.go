package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DRISHTI_WORKER_ADDR", "")
	t.Setenv("DRISHTI_MODE", "")
	t.Setenv("DRISHTI_ENABLE_WORKER_CONVERSION", "")
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
	if cfg.EnableConversion {
		t.Fatal("conversion must default to disabled")
	}
}

func TestConversionEnablementIsLabOrLiveOnly(t *testing.T) {
	for _, mode := range []string{"mock", "production"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("DRISHTI_MODE", mode)
			t.Setenv("DRISHTI_ENABLE_WORKER_CONVERSION", "true")
			if _, err := Load(); err == nil {
				t.Fatalf("conversion enabled in %s mode", mode)
			}
		})
	}
	for _, mode := range []string{"lab", "live"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("DRISHTI_MODE", mode)
			t.Setenv("DRISHTI_ENABLE_WORKER_CONVERSION", "true")
			cfg, err := Load()
			if err != nil || !cfg.EnableConversion {
				t.Fatalf("conversion not enabled in %s: cfg=%#v err=%v", mode, cfg, err)
			}
		})
	}
}

func TestWorkspaceCapIsConfigurableAndPositive(t *testing.T) {
	t.Setenv("DRISHTI_MAX_WORKSPACE_GB", "25")
	cfg, err := Load()
	if err != nil || cfg.MaxWorkspaceGB != 25 {
		t.Fatalf("workspace cap: cfg=%#v err=%v", cfg, err)
	}
	t.Setenv("DRISHTI_MAX_WORKSPACE_GB", "0")
	if _, err := Load(); err == nil {
		t.Fatal("zero workspace cap accepted")
	}
}
