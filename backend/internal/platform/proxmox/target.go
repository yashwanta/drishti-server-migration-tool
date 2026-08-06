package proxmox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/platform"
	"github.com/drishti/hypershift/internal/safety"
)

var pveIDRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
var vmNameRE = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

const ownershipPrefix = "drishti:idempotency:"

// Target implements platform.TargetAdapter using the Proxmox VE API.
// Construction performs no network access.
type Target struct {
	conn         domain.Connection
	client       *apiClient
	mutations    safety.MutationPolicy
	importRoot   string
	pollInterval time.Duration
	resolve      endpointResolver
	isolated     map[string]bool
}

// NewTarget constructs a real Proxmox target adapter. The import root must be
// an absolute POSIX path visible at the same location on the worker and PVE node.
func NewTarget(conn domain.Connection, mutations safety.MutationPolicy, importRoot string, isolatedBridges []string) (*Target, error) {
	if conn.Kind != domain.PlatformProxmox || conn.Role != domain.RoleTarget {
		return nil, fmt.Errorf("Proxmox target adapter requires a target-role Proxmox connection")
	}
	client, err := newAPIClient(conn)
	if err != nil {
		return nil, err
	}
	root, err := normalizeImportRoot(importRoot)
	if err != nil {
		return nil, err
	}
	isolated := make(map[string]bool, len(isolatedBridges))
	for _, bridge := range isolatedBridges {
		bridge = strings.TrimSpace(bridge)
		if err := validatePVEID("isolated bridge", bridge); err != nil {
			return nil, err
		}
		isolated[bridge] = true
	}
	if len(isolated) == 0 {
		return nil, fmt.Errorf("at least one DRISHTI_PROXMOX_ISOLATED_BRIDGES entry is required")
	}
	return &Target{conn: conn, client: client, mutations: mutations, importRoot: root, pollInterval: 2 * time.Second, resolve: defaultEndpointResolver, isolated: isolated}, nil
}

func (t *Target) Kind() domain.PlatformKind { return domain.PlatformProxmox }

func (t *Target) Inventory(ctx context.Context) (domain.InventoryRoot, error) {
	var resources []resource
	if err := t.client.get(ctx, "/api2/json/cluster/resources", &resources); err != nil {
		return domain.InventoryRoot{}, err
	}
	return targetInventory(ctx, t.client, t.conn.ID, resources)
}

func (t *Target) Node(ctx context.Context, nodeID string) (domain.TargetNode, error) {
	if err := validatePVEID("node", nodeID); err != nil {
		return domain.TargetNode{}, err
	}
	inv, err := t.Inventory(ctx)
	if err != nil {
		return domain.TargetNode{}, err
	}
	for _, node := range inv.Nodes {
		if node.ID == nodeID {
			return node, nil
		}
	}
	return domain.TargetNode{}, fmt.Errorf("target node %q not found", nodeID)
}

func (t *Target) ReserveVMID(ctx context.Context, nodeID string) (int, error) {
	if err := validatePVEID("node", nodeID); err != nil {
		return 0, err
	}
	if err := t.requireOnlineNode(ctx, nodeID); err != nil {
		return 0, err
	}
	return readNextVMID(ctx, t.client)
}

// CreateVM creates a protected, stopped VM with every NIC link-down. Repeated
// calls with the same VMID and idempotency key verify and return the same VM.
func (t *Target) CreateVM(ctx context.Context, spec platform.CreateVMSpec) (int, error) {
	if err := t.authorizeMutation(ctx); err != nil {
		return 0, err
	}
	if err := validateCreateSpec(spec); err != nil {
		return 0, err
	}
	if !t.isolated[spec.IsolatedBridge] {
		return 0, fmt.Errorf("target bridge %q is not in the isolated-bridge allowlist", spec.IsolatedBridge)
	}
	if err := t.requireOnlineNode(ctx, spec.NodeID); err != nil {
		return 0, err
	}
	if err := t.requireBridge(ctx, spec.NodeID, spec.IsolatedBridge); err != nil {
		return 0, err
	}
	if spec.Firmware == domain.FirmwareUEFI {
		if err := t.requireImageStorage(ctx, spec.NodeID, spec.StorageID, 0); err != nil {
			return 0, err
		}
	}

	marker := ownershipMarker(spec.IdempotencyKey)
	existingNode, found, err := t.findVMNode(ctx, spec.VMID)
	if err != nil {
		return 0, err
	}
	if found {
		if existingNode != spec.NodeID {
			return 0, fmt.Errorf("VMID %d already exists on node %q", spec.VMID, existingNode)
		}
		cfg, _, err := t.readVMConfig(ctx, spec.NodeID, spec.VMID)
		if err != nil {
			return 0, err
		}
		if err := verifyExistingCreate(cfg, spec, marker); err != nil {
			return 0, err
		}
		if err := t.requireStopped(ctx, spec.NodeID, spec.VMID); err != nil {
			return 0, err
		}
		return spec.VMID, nil
	}

	values := url.Values{
		"vmid":        {strconv.Itoa(spec.VMID)},
		"name":        {spec.Name},
		"cores":       {strconv.Itoa(spec.CPU)},
		"memory":      {strconv.FormatInt(spec.MemoryMB, 10)},
		"bios":        {pveBIOS(spec.Firmware)},
		"scsihw":      {"virtio-scsi-single"},
		"net0":        {"virtio,bridge=" + spec.IsolatedBridge + ",firewall=1,link_down=1"},
		"onboot":      {"0"},
		"protection":  {"1"},
		"description": {marker},
	}
	if spec.Firmware == domain.FirmwareUEFI {
		values.Set("efidisk0", spec.StorageID+":1,efitype=4m,pre-enrolled-keys=1")
	}
	var upid string
	apiPath := "/api2/json/nodes/" + url.PathEscape(spec.NodeID) + "/qemu"
	if err := t.client.postForm(ctx, apiPath, values, &upid); err != nil {
		return 0, fmt.Errorf("create protected target VM: %w", err)
	}
	if upid == "" {
		return 0, fmt.Errorf("Proxmox create VM did not return a task ID")
	}
	if err := t.waitTask(ctx, spec.NodeID, upid); err != nil {
		return 0, fmt.Errorf("create target VM: %w", err)
	}
	cfg, exists, err := t.readVMConfig(ctx, spec.NodeID, spec.VMID)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, fmt.Errorf("target VM %d was not found after create task", spec.VMID)
	}
	if err := verifyExistingCreate(cfg, spec, marker); err != nil {
		return 0, fmt.Errorf("verify created target VM: %w", err)
	}
	if err := t.requireStopped(ctx, spec.NodeID, spec.VMID); err != nil {
		return 0, err
	}
	return spec.VMID, nil
}

func (t *Target) StartVM(ctx context.Context, vmid int) error {
	if err := t.authorizeMutation(ctx); err != nil {
		return err
	}
	node, err := t.ownedVMNode(ctx, vmid)
	if err != nil {
		return err
	}
	cfg, _, err := t.readVMConfig(ctx, node, vmid)
	if err != nil {
		return err
	}
	for key, raw := range cfg {
		if strings.HasPrefix(key, "net") && isNumericSuffix(key, 3) {
			value := rawString(raw)
			parts := strings.Split(value, ",")
			opts := options(parts[1:])
			if opts["link_down"] != "1" || !t.isolated[opts["bridge"]] {
				return fmt.Errorf("target start refused: network %s is not link-down on an approved isolation bridge", key)
			}
		}
	}
	var upid string
	if err := t.client.postForm(ctx, vmStatusPath(node, vmid, "start"), url.Values{}, &upid); err != nil {
		return fmt.Errorf("start isolated target VM: %w", err)
	}
	if upid == "" {
		return fmt.Errorf("Proxmox start did not return a task ID")
	}
	if err := t.waitTask(ctx, node, upid); err != nil {
		return err
	}
	state, err := t.vmStateOnNode(ctx, node, vmid)
	if err != nil {
		return err
	}
	if state != domain.PowerOn {
		return fmt.Errorf("target VM did not reach running state")
	}
	return nil
}

func (t *Target) StopVM(ctx context.Context, vmid int) error {
	if err := t.authorizeMutation(ctx); err != nil {
		return err
	}
	node, err := t.ownedVMNode(ctx, vmid)
	if err != nil {
		return err
	}
	state, err := t.vmStateOnNode(ctx, node, vmid)
	if err != nil {
		return err
	}
	if state == domain.PowerOff {
		return nil
	}
	var upid string
	if err := t.client.postForm(ctx, vmStatusPath(node, vmid, "shutdown"), url.Values{}, &upid); err != nil {
		return fmt.Errorf("request graceful target shutdown: %w", err)
	}
	if upid == "" {
		return fmt.Errorf("Proxmox shutdown did not return a task ID")
	}
	if err := t.waitTask(ctx, node, upid); err != nil {
		return err
	}
	return t.requireStopped(ctx, node, vmid)
}

// SetNetwork currently supports isolation only. Production-network activation
// remains deliberately unavailable in this phase.
func (t *Target) SetNetwork(ctx context.Context, vmid int, bridge string, vlanID int, isolated bool) error {
	if !isolated {
		return fmt.Errorf("non-isolated target networking is not enabled in this phase")
	}
	if err := validatePVEID("isolated bridge", bridge); err != nil {
		return err
	}
	if !t.isolated[bridge] {
		return fmt.Errorf("target bridge %q is not in the isolated-bridge allowlist", bridge)
	}
	if vlanID < 0 || vlanID > 4094 {
		return fmt.Errorf("VLAN ID must be between 0 and 4094")
	}
	if err := t.authorizeMutation(ctx); err != nil {
		return err
	}
	node, err := t.ownedVMNode(ctx, vmid)
	if err != nil {
		return err
	}
	if err := t.requireBridge(ctx, node, bridge); err != nil {
		return err
	}
	net0 := "virtio,bridge=" + bridge + ",firewall=1,link_down=1"
	if vlanID > 0 {
		net0 += ",tag=" + strconv.Itoa(vlanID)
	}
	var result json.RawMessage
	configPath := fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/config", url.PathEscape(node), vmid)
	if err := t.client.postForm(ctx, configPath, url.Values{"net0": {net0}}, &result); err != nil {
		return fmt.Errorf("isolate target network: %w", err)
	}
	cfg, _, err := t.readVMConfig(ctx, node, vmid)
	if err != nil {
		return err
	}
	if !strings.Contains(rawString(cfg["net0"]), "link_down=1") {
		return fmt.Errorf("target network isolation verification failed")
	}
	return nil
}

func (t *Target) VMState(ctx context.Context, vmid int) (domain.PowerState, error) {
	node, found, err := t.findVMNode(ctx, vmid)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("target VMID %d not found", vmid)
	}
	return t.vmStateOnNode(ctx, node, vmid)
}

func (t *Target) authorizeMutation(ctx context.Context) error {
	if err := t.mutations.Authorize(t.conn.Endpoint); err != nil {
		return fmt.Errorf("Proxmox target mutation refused: %w", err)
	}
	if err := rejectResolvedDenylistAliases(ctx, t.conn.Endpoint, t.mutations.Denylist, t.resolve); err != nil {
		return fmt.Errorf("Proxmox target mutation refused: %w", err)
	}
	return nil
}

func (t *Target) requireOnlineNode(ctx context.Context, nodeID string) error {
	var resources []resource
	if err := t.client.get(ctx, "/api2/json/cluster/resources", &resources); err != nil {
		return err
	}
	for _, item := range resources {
		if item.Type == "node" && item.Node == nodeID {
			if item.Status != "online" {
				return fmt.Errorf("target node %q is not online", nodeID)
			}
			return nil
		}
	}
	return fmt.Errorf("target node %q not found", nodeID)
}

func (t *Target) requireImageStorage(ctx context.Context, nodeID, storageID string, requiredBytes int64) error {
	if err := validatePVEID("storage", storageID); err != nil {
		return err
	}
	var stores []struct {
		Storage string `json:"storage"`
		Content string `json:"content"`
		Active  int    `json:"active"`
		Enabled int    `json:"enabled"`
		Avail   int64  `json:"avail"`
	}
	apiPath := "/api2/json/nodes/" + url.PathEscape(nodeID) + "/storage"
	if err := t.client.get(ctx, apiPath, &stores); err != nil {
		return err
	}
	for _, store := range stores {
		if store.Storage == storageID && store.Active != 0 && store.Enabled != 0 && containsCSV(store.Content, "images") {
			if requiredBytes > 0 && store.Avail < requiredBytes {
				return fmt.Errorf("target storage %q has %d bytes available but import requires %d", storageID, store.Avail, requiredBytes)
			}
			return nil
		}
	}
	return fmt.Errorf("image-capable target storage %q is not active on node %q", storageID, nodeID)
}

func (t *Target) requireBridge(ctx context.Context, nodeID, bridge string) error {
	if err := validatePVEID("bridge", bridge); err != nil {
		return err
	}
	var bridges []struct {
		Iface string `json:"iface"`
		Type  string `json:"type"`
	}
	apiPath := "/api2/json/nodes/" + url.PathEscape(nodeID) + "/network?type=bridge"
	if err := t.client.get(ctx, apiPath, &bridges); err != nil {
		return err
	}
	for _, item := range bridges {
		if item.Type == "bridge" && item.Iface == bridge {
			return nil
		}
	}
	return fmt.Errorf("target bridge %q not found on node %q", bridge, nodeID)
}

func (t *Target) findVMNode(ctx context.Context, vmid int) (string, bool, error) {
	if vmid < 100 {
		return "", false, fmt.Errorf("target VMID must be at least 100")
	}
	var resources []resource
	if err := t.client.get(ctx, "/api2/json/cluster/resources", &resources); err != nil {
		return "", false, err
	}
	for _, item := range resources {
		if item.Type == "qemu" && item.VMID == vmid {
			return item.Node, true, nil
		}
	}
	return "", false, nil
}

func (t *Target) ownedVMNode(ctx context.Context, vmid int) (string, error) {
	node, found, err := t.findVMNode(ctx, vmid)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("target VMID %d not found", vmid)
	}
	cfg, _, err := t.readVMConfig(ctx, node, vmid)
	if err != nil {
		return "", err
	}
	if !strings.Contains(rawString(cfg["description"]), ownershipPrefix) {
		return "", fmt.Errorf("target VMID %d is not owned by a DRISHTI idempotency marker", vmid)
	}
	return node, nil
}

func (t *Target) readVMConfig(ctx context.Context, node string, vmid int) (map[string]json.RawMessage, bool, error) {
	var cfg map[string]json.RawMessage
	apiPath := fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/config", url.PathEscape(node), vmid)
	err := t.client.get(ctx, apiPath, &cfg)
	if err == nil {
		return cfg, true, nil
	}
	var apiErr *apiError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	return nil, false, err
}

func (t *Target) vmStateOnNode(ctx context.Context, node string, vmid int) (domain.PowerState, error) {
	var status struct {
		Status string `json:"status"`
	}
	apiPath := fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/current", url.PathEscape(node), vmid)
	if err := t.client.get(ctx, apiPath, &status); err != nil {
		return "", err
	}
	return powerState(status.Status), nil
}

func (t *Target) requireStopped(ctx context.Context, node string, vmid int) error {
	state, err := t.vmStateOnNode(ctx, node, vmid)
	if err != nil {
		return err
	}
	if state != domain.PowerOff {
		return fmt.Errorf("target VMID %d is not stopped", vmid)
	}
	return nil
}

func (t *Target) waitTask(ctx context.Context, node, upid string) error {
	if strings.TrimSpace(upid) == "" {
		return fmt.Errorf("empty Proxmox task ID")
	}
	ticker := time.NewTicker(t.pollInterval)
	defer ticker.Stop()
	for {
		var status struct {
			Status     string `json:"status"`
			ExitStatus string `json:"exitstatus"`
		}
		apiPath := "/api2/json/nodes/" + url.PathEscape(node) + "/tasks/" + url.PathEscape(upid) + "/status"
		if err := t.client.get(ctx, apiPath, &status); err != nil {
			return fmt.Errorf("poll Proxmox task: %w", err)
		}
		if status.Status == "stopped" {
			if status.ExitStatus != "OK" {
				return fmt.Errorf("Proxmox task failed: %s", status.ExitStatus)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func validateCreateSpec(spec platform.CreateVMSpec) error {
	if err := validatePVEID("node", spec.NodeID); err != nil {
		return err
	}
	if spec.VMID < 100 || spec.VMID > 999999999 {
		return fmt.Errorf("target VMID must be between 100 and 999999999")
	}
	if !vmNameRE.MatchString(spec.Name) {
		return fmt.Errorf("target VM name must be a valid Proxmox DNS name")
	}
	if spec.CPU < 1 || spec.CPU > 768 {
		return fmt.Errorf("target CPU count is outside the supported range")
	}
	if spec.MemoryMB < 16 {
		return fmt.Errorf("target memory must be at least 16 MiB")
	}
	if spec.Firmware != domain.FirmwareBIOS && spec.Firmware != domain.FirmwareUEFI {
		return fmt.Errorf("target firmware must be bios or uefi")
	}
	if spec.IdempotencyKey == "" {
		return fmt.Errorf("target create requires an idempotency key")
	}
	return validatePVEID("isolated bridge", spec.IsolatedBridge)
}

func validatePVEID(kind, value string) error {
	if !pveIDRE.MatchString(value) {
		return fmt.Errorf("invalid Proxmox %s %q", kind, value)
	}
	return nil
}

func ownershipMarker(key string) string {
	sum := sha256.Sum256([]byte(key))
	return ownershipPrefix + hex.EncodeToString(sum[:16])
}

func verifyExistingCreate(cfg map[string]json.RawMessage, spec platform.CreateVMSpec, marker string) error {
	if !strings.Contains(rawString(cfg["description"]), marker) {
		return fmt.Errorf("VMID %d exists without the matching DRISHTI idempotency marker", spec.VMID)
	}
	if rawString(cfg["name"]) != spec.Name || rawInt(cfg["cores"]) != spec.CPU || int64(rawInt(cfg["memory"])) != spec.MemoryMB || rawString(cfg["bios"]) != pveBIOS(spec.Firmware) {
		return fmt.Errorf("VMID %d exists but its configuration does not match the approved create request", spec.VMID)
	}
	if rawString(cfg["protection"]) != "1" {
		return fmt.Errorf("VMID %d exists without deletion protection", spec.VMID)
	}
	if spec.Firmware == domain.FirmwareUEFI && rawString(cfg["efidisk0"]) == "" {
		return fmt.Errorf("VMID %d exists without its required EFI disk", spec.VMID)
	}
	for key, raw := range cfg {
		if strings.HasPrefix(key, "net") && isNumericSuffix(key, 3) {
			parts := strings.Split(rawString(raw), ",")
			opts := options(parts[1:])
			if opts["link_down"] != "1" || opts["bridge"] != spec.IsolatedBridge {
				return fmt.Errorf("VMID %d network %s is not isolated on bridge %s", spec.VMID, key, spec.IsolatedBridge)
			}
		}
	}
	return nil
}

func pveBIOS(firmware domain.Firmware) string {
	if firmware == domain.FirmwareUEFI {
		return "ovmf"
	}
	return "seabios"
}

func vmStatusPath(node string, vmid int, action string) string {
	return fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/%s", url.PathEscape(node), vmid, action)
}

var _ platform.TargetAdapter = (*Target)(nil)
