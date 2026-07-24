// Package mockplatform provides mock implementations of the platform adapter
// interfaces. They fully simulate a migration lifecycle so the entire workflow
// can be tested without real infrastructure.
package mockplatform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/mock"
	"github.com/drishti/hypershift/internal/platform"
)

// inv returns the generated mock inventory for a connection id, or an error.
func inv(connID string) (domain.InventoryRoot, error) {
	root, ok := mock.New().Inventory(connID)
	if !ok {
		return domain.InventoryRoot{}, fmt.Errorf("inventory not found for connection %q", connID)
	}
	return root, nil
}

// ---- Source adapter ----

type MockSource struct {
	conn  domain.Connection
	mu    sync.Mutex
	power map[string]domain.PowerState
	disks map[string][]domain.Disk
}

func NewMockSource(conn domain.Connection) *MockSource {
	p := mock.New()
	src, _ := p.Inventory(conn.ID)
	power := map[string]domain.PowerState{}
	disks := map[string][]domain.Disk{}
	for _, dc := range src.Datacenters {
		for _, cl := range dc.Clusters {
			for _, h := range cl.Hosts {
				for _, vm := range h.VMs {
					power[vm.ID] = vm.PowerState
					disks[vm.ID] = vm.Disks
				}
			}
		}
	}
	return &MockSource{conn: conn, power: power, disks: disks}
}

func (s *MockSource) Kind() domain.PlatformKind { return s.conn.Kind }

func (s *MockSource) Inventory(ctx context.Context) (domain.InventoryRoot, error) {
	return inv(s.conn.ID)
}

func (s *MockSource) VM(ctx context.Context, vmID string) (domain.VM, error) {
	src, err := inv(s.conn.ID)
	if err != nil {
		return domain.VM{}, err
	}
	for _, dc := range src.Datacenters {
		for _, cl := range dc.Clusters {
			for _, h := range cl.Hosts {
				for _, vm := range h.VMs {
					if vm.ID == vmID {
						return vm, nil
					}
				}
			}
		}
	}
	return domain.VM{}, fmt.Errorf("vm %q not found", vmID)
}

func (s *MockSource) ExportDisk(ctx context.Context, vmID, diskID, destPath string) (platform.ExportResult, error) {
	start := time.Now()
	disks, ok := s.disks[vmID]
	if !ok {
		return platform.ExportResult{}, fmt.Errorf("vm %q not found for export", vmID)
	}
	var disk domain.Disk
	found := false
	for _, d := range disks {
		if d.ID == diskID {
			disk = d
			found = true
			break
		}
	}
	if !found {
		return platform.ExportResult{}, fmt.Errorf("disk %q not found on vm %q", diskID, vmID)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return platform.ExportResult{}, fmt.Errorf("create export dir: %w", err)
	}
	payload := fmt.Sprintf("mock-export-%s-%s-%d", vmID, diskID, disk.CapacityBytes)
	h := sha256.Sum256([]byte(payload))
	if err := os.WriteFile(destPath, []byte(payload), 0o644); err != nil {
		return platform.ExportResult{}, fmt.Errorf("write export: %w", err)
	}
	return platform.ExportResult{
		Path: destPath, SizeBytes: int64(len(payload)),
		Format: disk.Format, SHA256: hex.EncodeToString(h[:]),
		Duration: time.Since(start).Round(time.Millisecond).String(),
	}, nil
}

func (s *MockSource) PowerOff(ctx context.Context, vmID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.power[vmID]; !ok {
		return fmt.Errorf("vm %q not found", vmID)
	}
	s.power[vmID] = domain.PowerOff
	return nil
}

func (s *MockSource) PowerOn(ctx context.Context, vmID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.power[vmID]; !ok {
		return fmt.Errorf("vm %q not found", vmID)
	}
	s.power[vmID] = domain.PowerOn
	return nil
}

func (s *MockSource) PowerState(ctx context.Context, vmID string) (domain.PowerState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.power[vmID]
	if !ok {
		return "", fmt.Errorf("vm %q not found", vmID)
	}
	return p, nil
}

// ---- Target adapter ----

type MockTarget struct {
	conn   domain.Connection
	mu     sync.Mutex
	vms    map[int]*mockVM
	nextID int
}

type mockVM struct {
	id        int
	name      string
	nodeID    string
	state     domain.PowerState
	disks     []platform.AttachDiskSpec
	bridge    string
	vlan      int
	isolated  bool
	createdAt time.Time
}

func NewMockTarget(conn domain.Connection) *MockTarget {
	return &MockTarget{conn: conn, vms: map[int]*mockVM{}, nextID: 100}
}

func (t *MockTarget) Kind() domain.PlatformKind { return t.conn.Kind }

func (t *MockTarget) Inventory(ctx context.Context) (domain.InventoryRoot, error) {
	return inv(t.conn.ID)
}

func (t *MockTarget) Node(ctx context.Context, nodeID string) (domain.TargetNode, error) {
	src, err := inv(t.conn.ID)
	if err != nil {
		return domain.TargetNode{}, err
	}
	for _, n := range src.Nodes {
		if n.ID == nodeID {
			return n, nil
		}
	}
	return domain.TargetNode{}, fmt.Errorf("node %q not found", nodeID)
}

func (t *MockTarget) ReserveVMID(ctx context.Context, nodeID string) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextID++
	return t.nextID, nil
}

func (t *MockTarget) CreateVM(ctx context.Context, spec platform.CreateVMSpec) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, vm := range t.vms {
		if vm.name == spec.Name && id == spec.VMID {
			return id, nil
		}
	}
	t.vms[spec.VMID] = &mockVM{
		id: spec.VMID, name: spec.Name, nodeID: spec.NodeID,
		state: domain.PowerOff, bridge: spec.IsolatedBridge,
		isolated: true, createdAt: time.Now(),
	}
	return spec.VMID, nil
}

func (t *MockTarget) AttachDisk(ctx context.Context, vmid int, disk platform.AttachDiskSpec) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	vm, ok := t.vms[vmid]
	if !ok {
		return fmt.Errorf("target vm %d not found", vmid)
	}
	vm.disks = append(vm.disks, disk)
	return nil
}

func (t *MockTarget) StartVM(ctx context.Context, vmid int) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	vm, ok := t.vms[vmid]
	if !ok {
		return fmt.Errorf("target vm %d not found", vmid)
	}
	vm.state = domain.PowerOn
	return nil
}

func (t *MockTarget) StopVM(ctx context.Context, vmid int) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	vm, ok := t.vms[vmid]
	if !ok {
		return fmt.Errorf("target vm %d not found", vmid)
	}
	vm.state = domain.PowerOff
	return nil
}

func (t *MockTarget) SetNetwork(ctx context.Context, vmid int, bridge string, vlanID int, isolated bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	vm, ok := t.vms[vmid]
	if !ok {
		return fmt.Errorf("target vm %d not found", vmid)
	}
	vm.bridge = bridge
	vm.vlan = vlanID
	vm.isolated = isolated
	return nil
}

func (t *MockTarget) VMState(ctx context.Context, vmid int) (domain.PowerState, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	vm, ok := t.vms[vmid]
	if !ok {
		return "", fmt.Errorf("target vm %d not found", vmid)
	}
	return vm.state, nil
}

// ---- Factory ----

// MockFactory caches adapters per connection so state (power, VM creation)
// persists across API requests within a single process.
type MockFactory struct {
	mu       sync.Mutex
	sources  map[string]*MockSource
	targets  map[string]*MockTarget
}

func NewMockFactory() *MockFactory {
	return &MockFactory{sources: map[string]*MockSource{}, targets: map[string]*MockTarget{}}
}

func (f *MockFactory) Source(conn domain.Connection) (platform.SourceAdapter, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.sources[conn.ID]; ok {
		return s, nil
	}
	s := NewMockSource(conn)
	f.sources[conn.ID] = s
	return s, nil
}

func (f *MockFactory) Target(conn domain.Connection) (platform.TargetAdapter, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.targets[conn.ID]; ok {
		return t, nil
	}
	t := NewMockTarget(conn)
	f.targets[conn.ID] = t
	return t, nil
}