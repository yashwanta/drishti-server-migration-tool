// Package mock provides a development-only inventory provider that returns
// realistic VMware and Proxmox data without contacting any real platform.
//
// It implements a mutable connection store so that connections added through
// the API also produce generated sample inventory, letting the UI be exercised
// end-to-end without real infrastructure.
package mock

import (
	"fmt"
	"sync"
	"time"

	"github.com/drishti/hypershift/internal/domain"
)

// Provider is a thread-safe, mutable store of platform connections that also
// generates deterministic sample inventory for each connection.
type Provider struct {
	mu          sync.RWMutex
	connections []domain.Connection
}

func New() *Provider {
	p := &Provider{}
	p.seed()
	return p
}

// NewEmpty creates a connection registry without development fixtures. Real
// lab, live, and production modes must never display synthetic connections.
func NewEmpty() *Provider {
	return &Provider{}
}

// seed populates the two default lab connections.
func (p *Provider) seed() {
	now := time.Now().UTC()
	p.connections = []domain.Connection{
		{
			ID: "conn-vmware-lab", Name: "vCenter Lab", Kind: domain.PlatformVMware, Role: domain.RoleSource,
			Endpoint: "vcenter-lab.example.local", Status: domain.ConnConnected, SecretRef: "vmw/lab-admin",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "conn-proxmox-lab", Name: "Proxmox Lab", Kind: domain.PlatformProxmox, Role: domain.RoleTarget,
			Endpoint: "pve-lab.example.local", Status: domain.ConnConnected, SecretRef: "pve/lab-token",
			CreatedAt: now, UpdatedAt: now,
		},
	}
}

// Connections returns a snapshot of all configured connections.
func (p *Provider) Connections() []domain.Connection {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]domain.Connection, len(p.connections))
	copy(out, p.connections)
	return out
}

// Connection returns a single connection by id.
func (p *Provider) Connection(id string) (domain.Connection, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, c := range p.connections {
		if c.ID == id {
			return c, true
		}
	}
	return domain.Connection{}, false
}

// Add appends a new connection. It validates the kind/role and returns the
// stored connection with server-assigned fields filled in.
func (p *Provider) Add(c domain.Connection) (domain.Connection, error) {
	switch c.Kind {
	case domain.PlatformVMware, domain.PlatformProxmox, "hyperv":
	default:
		return domain.Connection{}, fmt.Errorf("unsupported platform kind %q (supported: vmware, proxmox, hyperv)", c.Kind)
	}
	switch c.Role {
	case domain.RoleSource, domain.RoleTarget:
	default:
		return domain.Connection{}, fmt.Errorf("unsupported role %q (supported: source, target)", c.Role)
	}
	if c.Endpoint == "" {
		return domain.Connection{}, fmt.Errorf("endpoint is required")
	}
	if c.Name == "" {
		c.Name = string(c.Kind) + " " + string(c.Role)
	}
	// Derive a stable, readable id from kind + endpoint.
	id := slugify(string(c.Kind), c.Endpoint)
	if _, exists := p.Connection(id); exists {
		return domain.Connection{}, fmt.Errorf("a connection for %s (%s) already exists", c.Endpoint, c.Kind)
	}
	now := time.Now().UTC()
	c.ID = id
	c.Status = domain.ConnConnected
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = now
	}
	p.mu.Lock()
	p.connections = append(p.connections, c)
	p.mu.Unlock()
	return c, nil
}

// Remove deletes a connection by id. Returns false if not found.
func (p *Provider) Remove(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, c := range p.connections {
		if c.ID == id {
			p.connections = append(p.connections[:i], p.connections[i+1:]...)
			return true
		}
	}
	return false
}

// Inventory returns generated sample inventory for the given connection id.
func (p *Provider) Inventory(connID string) (domain.InventoryRoot, bool) {
	c, ok := p.Connection(connID)
	if !ok {
		return domain.InventoryRoot{}, false
	}
	return generateInventory(c), true
}

// generateInventory produces deterministic sample inventory shaped by the
// connection kind and role. It does not contact any real platform.
func generateInventory(c domain.Connection) domain.InventoryRoot {
	switch c.Kind {
	case domain.PlatformVMware:
		return vmwareInventory(c.ID)
	case domain.PlatformProxmox:
		return proxmoxInventory(c.ID)
	case "hyperv":
		return hypervInventory(c.ID)
	default:
		return domain.InventoryRoot{ConnectionID: c.ID, GeneratedAt: time.Now().UTC()}
	}
}

// slugify builds a stable connection id from kind and endpoint.
func slugify(kind, endpoint string) string {
	endpoint = sanitize(endpoint)
	if endpoint == "" {
		return "conn-" + kind
	}
	return "conn-" + kind + "-" + endpoint
}

// sanitize strips characters that are unsafe for ids and display.
func sanitize(s string) string {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, byte(r))
		case r >= 'A' && r <= 'Z':
			out = append(out, byte(r-'A'+'a'))
		case r == '.' || r == '-':
			out = append(out, byte(r))
		}
	}
	return string(out)
}
