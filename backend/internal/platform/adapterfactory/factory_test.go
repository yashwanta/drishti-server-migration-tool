package adapterfactory

import (
	"testing"

	"github.com/drishti/hypershift/internal/config"
	"github.com/drishti/hypershift/internal/credential"
	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/platform/mockplatform"
	"github.com/drishti/hypershift/internal/platform/proxmox"
	"github.com/drishti/hypershift/internal/platform/vmware"
)

func TestFactoryModeMatrixNeverFallsBackToMock(t *testing.T) {
	t.Setenv("DRISHTI_SECRET_PVE_TOKEN_ID", "root@pam!drishti")
	t.Setenv("DRISHTI_SECRET_PVE_TOKEN_SECRET", "fixture-secret")
	creds := credential.NewMemory()
	creds.Put("vmware", credential.Value{Username: "fixture-user", Password: "fixture-password"})
	sourceConn := domain.Connection{ID: "vmware", Kind: domain.PlatformVMware, Role: domain.RoleSource, Endpoint: "https://127.0.0.1/sdk"}
	targetConn := domain.Connection{ID: "proxmox", Kind: domain.PlatformProxmox, Role: domain.RoleTarget, Endpoint: "https://127.0.0.1:8006", SecretRef: "pve"}

	mockFactory, err := New(config.Config{Mode: config.ModeMock}, creds)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := mockFactory.(*mockplatform.MockFactory); !ok {
		t.Fatalf("mock mode factory type = %T", mockFactory)
	}

	for _, mode := range []config.RunMode{config.ModeLab, config.ModeLive, config.ModeProduction} {
		t.Run(string(mode), func(t *testing.T) {
			factory, err := New(realFactoryConfig(mode, t.TempDir()), creds)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := factory.(*RealFactory); !ok {
				t.Fatalf("real mode factory type = %T", factory)
			}
			source, err := factory.Source(sourceConn)
			if err != nil {
				t.Fatalf("Source: %v", err)
			}
			if _, ok := source.(*vmware.Source); !ok {
				t.Fatalf("source type = %T; real mode must not return mock", source)
			}
			target, err := factory.Target(targetConn)
			if err != nil {
				t.Fatalf("Target: %v", err)
			}
			if _, ok := target.(*proxmox.Target); !ok {
				t.Fatalf("target type = %T; real mode must not return mock", target)
			}
		})
	}
}

func TestRealFactoryFailsClosedForUnsupportedOrMissingInputs(t *testing.T) {
	t.Setenv("DRISHTI_SECRET_PVE_TOKEN_ID", "root@pam!drishti")
	t.Setenv("DRISHTI_SECRET_PVE_TOKEN_SECRET", "fixture-secret")
	factoryRaw, err := New(realFactoryConfig(config.ModeLab, t.TempDir()), credential.NewMemory())
	if err != nil {
		t.Fatal(err)
	}
	factory := factoryRaw.(*RealFactory)

	tests := []struct {
		name   string
		source *domain.Connection
		target *domain.Connection
	}{
		{name: "missing VMware credentials", source: &domain.Connection{ID: "missing", Kind: domain.PlatformVMware, Role: domain.RoleSource}},
		{name: "Proxmox cannot be source", source: &domain.Connection{ID: "pve", Kind: domain.PlatformProxmox, Role: domain.RoleSource}},
		{name: "VMware source role required", source: &domain.Connection{ID: "vmw", Kind: domain.PlatformVMware, Role: domain.RoleTarget}},
		{name: "VMware cannot be target", target: &domain.Connection{ID: "vmw", Kind: domain.PlatformVMware, Role: domain.RoleTarget}},
		{name: "Proxmox target role required", target: &domain.Connection{ID: "pve", Kind: domain.PlatformProxmox, Role: domain.RoleSource}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.source != nil {
				adapter, err := factory.Source(*tt.source)
				if err == nil || adapter != nil {
					t.Fatalf("Source returned adapter=%T err=%v; expected fail-closed error", adapter, err)
				}
			}
			if tt.target != nil {
				adapter, err := factory.Target(*tt.target)
				if err == nil || adapter != nil {
					t.Fatalf("Target returned adapter=%T err=%v; expected fail-closed error", adapter, err)
				}
			}
		})
	}
}

func TestRealFactoryRejectsMockAndUnknownModes(t *testing.T) {
	if factory, err := New(config.Config{Mode: "danger"}, credential.NewMemory()); err == nil || factory != nil {
		t.Fatalf("unknown mode returned factory=%T err=%v", factory, err)
	}
}

func realFactoryConfig(mode config.RunMode, workspace string) config.Config {
	return config.Config{
		Mode:                   mode,
		WorkspaceRoot:          workspace,
		ProxmoxImportRoot:      "/mnt/drishti-import",
		ProxmoxIsolatedBridges: []string{"vmbr-lab"},
	}
}
