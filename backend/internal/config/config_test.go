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
	if cfg.WorkspaceRoot == "" {
		t.Error("workspace root should have a safe temporary default")
	}
}

func TestLoadRequiresDBInEveryRealMode(t *testing.T) {
	for _, mode := range []RunMode{ModeLab, ModeLive, ModeProduction} {
		t.Run(string(mode), func(t *testing.T) {
			t.Setenv("DRISHTI_MODE", string(mode))
			t.Setenv("DRISHTI_DB_URL", "")
			t.Setenv("DRISHTI_WORKER_URL", "http://worker.invalid:8090")
			if _, err := Load(); err == nil {
				t.Fatalf("expected DB_URL requirement in %s mode", mode)
			}
		})
	}
}

func TestLoadRequiresDBInProductionMode(t *testing.T) {
	t.Setenv("DRISHTI_MODE", "production")
	t.Setenv("DRISHTI_DB_URL", "")
	t.Setenv("DRISHTI_WORKER_URL", "http://worker.invalid:8090")
	if _, err := Load(); err == nil {
		t.Fatal("expected error requiring DB_URL in production mode")
	}
}

func TestLoadRequiresWorkerInEveryRealMode(t *testing.T) {
	for _, mode := range []RunMode{ModeLab, ModeLive, ModeProduction} {
		t.Run(string(mode), func(t *testing.T) {
			t.Setenv("DRISHTI_MODE", string(mode))
			t.Setenv("DRISHTI_DB_URL", "postgres://example.invalid/drishti")
			t.Setenv("DRISHTI_WORKER_URL", "")
			if _, err := Load(); err == nil {
				t.Fatalf("expected worker URL requirement in %s mode", mode)
			}
		})
	}
}

func TestProductionRejectsPlatformMutation(t *testing.T) {
	t.Setenv("DRISHTI_MODE", "production")
	t.Setenv("DRISHTI_DB_URL", "postgres://example.invalid/drishti")
	t.Setenv("DRISHTI_WORKER_URL", "http://worker.invalid:8090")
	t.Setenv("DRISHTI_ENABLE_PLATFORM_MUTATION", "true")
	if _, err := Load(); err == nil {
		t.Fatal("expected production platform mutation to be rejected")
	}
}

func TestProductionRequiresSecureSessionCookie(t *testing.T) {
	t.Setenv("DRISHTI_MODE", "production")
	t.Setenv("DRISHTI_DB_URL", "postgres://example.invalid/drishti")
	t.Setenv("DRISHTI_WORKER_URL", "http://worker.invalid:8090")
	t.Setenv("DRISHTI_SESSION_SECURE", "false")
	if _, err := Load(); err == nil {
		t.Fatal("expected production to require secure session cookies")
	}
}

func TestMutationDenylist(t *testing.T) {
	t.Setenv("DRISHTI_MODE", "lab")
	t.Setenv("DRISHTI_DB_URL", "postgres://example.invalid/drishti")
	t.Setenv("DRISHTI_WORKER_URL", "http://worker.invalid:8090")
	t.Setenv("DRISHTI_MUTATION_DENYLIST", "prod-vcenter.example, 10.0.0.2:443")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MutationDenylist) != 2 {
		t.Fatalf("denylist = %#v", cfg.MutationDenylist)
	}
}

func TestProxmoxTargetSafetyConfiguration(t *testing.T) {
	t.Setenv("DRISHTI_MODE", "lab")
	t.Setenv("DRISHTI_DB_URL", "postgres://example.invalid/drishti")
	t.Setenv("DRISHTI_WORKER_URL", "http://worker.invalid:8090")
	t.Setenv("DRISHTI_PROXMOX_IMPORT_ROOT", "/mnt/drishti-import")
	t.Setenv("DRISHTI_PROXMOX_ISOLATED_BRIDGES", "vmbr-lab, vmbr-quarantine")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProxmoxImportRoot != "/mnt/drishti-import" || len(cfg.ProxmoxIsolatedBridges) != 2 {
		t.Fatalf("unexpected Proxmox safety config: %#v", cfg)
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
