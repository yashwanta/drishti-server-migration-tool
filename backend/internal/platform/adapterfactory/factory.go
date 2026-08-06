// Package adapterfactory selects mock or real platform adapters by runtime
// mode. Real-mode selection never falls back to a mock implementation.
package adapterfactory

import (
	"fmt"

	"github.com/drishti/hypershift/internal/config"
	"github.com/drishti/hypershift/internal/credential"
	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/platform"
	"github.com/drishti/hypershift/internal/platform/mockplatform"
	"github.com/drishti/hypershift/internal/platform/proxmox"
	"github.com/drishti/hypershift/internal/platform/vmware"
	"github.com/drishti/hypershift/internal/safety"
)

// New returns a mock factory only for mock mode. Lab, live, and production
// always receive RealFactory, even when their mutation policy is disabled.
func New(cfg config.Config, creds *credential.Memory) (platform.AdapterFactory, error) {
	if cfg.Mode == config.ModeMock {
		return mockplatform.NewMockFactory(), nil
	}
	switch cfg.Mode {
	case config.ModeLab, config.ModeLive, config.ModeProduction:
		if creds == nil {
			return nil, fmt.Errorf("real adapter factory requires an ephemeral credential store")
		}
		return &RealFactory{cfg: cfg, creds: creds}, nil
	default:
		return nil, fmt.Errorf("unsupported runtime mode %q", cfg.Mode)
	}
}

// RealFactory creates only real adapters and returns explicit errors for every
// unsupported platform/role/credential combination.
type RealFactory struct {
	cfg   config.Config
	creds *credential.Memory
}

func (f *RealFactory) Source(conn domain.Connection) (platform.SourceAdapter, error) {
	if conn.Kind != domain.PlatformVMware || conn.Role != domain.RoleSource {
		return nil, fmt.Errorf("real source adapter requires a source-role VMware connection")
	}
	value, ok := f.creds.Get(conn.ID)
	if !ok || value.Username == "" || value.Password == "" {
		return nil, fmt.Errorf("VMware credentials are unavailable for connection %q; re-add the connection", conn.ID)
	}
	return vmware.NewSource(
		conn.ID,
		conn.Endpoint,
		value.Username,
		value.Password,
		conn.InsecureTLS,
		f.cfg.WorkspaceRoot,
		f.mutationPolicy(),
	), nil
}

func (f *RealFactory) Target(conn domain.Connection) (platform.TargetAdapter, error) {
	if conn.Kind != domain.PlatformProxmox || conn.Role != domain.RoleTarget {
		return nil, fmt.Errorf("real target adapter requires a target-role Proxmox connection")
	}
	return proxmox.NewTarget(conn, f.mutationPolicy(), f.cfg.ProxmoxImportRoot, f.cfg.ProxmoxIsolatedBridges)
}

func (f *RealFactory) mutationPolicy() safety.MutationPolicy {
	return safety.MutationPolicy{
		Mode:     f.cfg.Mode,
		Enabled:  f.cfg.EnablePlatformMutation,
		Denylist: append([]string(nil), f.cfg.MutationDenylist...),
	}
}

var _ platform.AdapterFactory = (*RealFactory)(nil)
