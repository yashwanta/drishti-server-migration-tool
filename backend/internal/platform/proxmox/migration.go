package proxmox

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/drishti/hypershift/internal/domain"
)

type RemoteMigrationRequest struct {
	Plan       domain.Plan
	Source     domain.Connection
	Target     domain.Connection
	TargetVMID int
}

type RemoteMigrationResult struct {
	UPID       string `json:"upid"`
	SourceNode string `json:"source_node"`
	TargetNode string `json:"target_node"`
	SourceVMID int    `json:"source_vmid"`
	TargetVMID int    `json:"target_vmid"`
}

type RemoteMigrationProgress struct {
	Percent float64
	Message string
}

var percentRE = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)%`)

// RemotePreflight performs only read operations and returns migration blockers.
func (p *Probe) RemotePreflight(ctx context.Context, plan domain.Plan, source, target domain.Connection) ([]domain.PreflightCheck, error) {
	checks := []domain.PreflightCheck{}
	add := func(id, message string, pass bool, detail string) {
		status := domain.CheckPass
		severity := domain.SeverityInfo
		if !pass {
			status, severity = domain.CheckFail, domain.SeverityError
		}
		checks = append(checks, domain.PreflightCheck{ID: id, Code: id, Status: status, Severity: severity, Message: message, Detail: detail})
	}
	add("lab_mode_scope", "Both connections are Proxmox VE with source/target roles", source.Kind == domain.PlatformProxmox && target.Kind == domain.PlatformProxmox && source.Role == domain.RoleSource && target.Role == domain.RoleTarget, "Remote migration supports Proxmox QEMU guests only.")
	add("separate_endpoints", "Source and target endpoints are different", normalizeCompare(source.Endpoint) != normalizeCompare(target.Endpoint), "Source and destination must not be the same API endpoint.")

	sourceInv, err := p.Inventory(ctx, source)
	if err != nil {
		return nil, fmt.Errorf("source inventory: %w", err)
	}
	targetInv, err := p.Inventory(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("target inventory: %w", err)
	}
	vm, sourceNode, found := findSourceVM(sourceInv, plan.SourceVMID)
	add("source_vm_exists", "Source QEMU VM exists", found, plan.SourceVMID)
	if !found {
		return checks, nil
	}
	add("source_powered_off", "Source VM is powered off", vm.PowerState == domain.PowerOff, fmt.Sprintf("Current state: %s", vm.PowerState))
	node, found := findTargetNode(targetInv, plan.TargetNodeID)
	add("target_node_online", "Target node exists and is online", found && node.Online, plan.TargetNodeID)
	if !found {
		return checks, nil
	}
	add("storage_mapped", "Every source disk has a valid target storage mapping", validStorageMaps(vm, node, plan.StorageMaps), "Mappings must reference image-capable storage visible on the target node.")
	add("network_mapped", "Every source NIC has a valid target bridge mapping", validNetworkMaps(vm, node, plan.NetworkMaps), "Mappings must reference bridges visible on the target node.")
	add("uniform_target_storage", "Storage mapping is compatible with Proxmox remote migration", uniformTargetStorage(plan.StorageMaps), "This lab implementation requires all disks to use one target storage.")
	add("uniform_target_bridge", "Network mapping is compatible with Proxmox remote migration", uniformTargetBridge(plan.NetworkMaps), "This lab implementation requires all NICs to use one target bridge.")
	_ = sourceNode
	return checks, nil
}

// StartRemoteMigration starts an offline cross-cluster migration. delete=0 is
// non-negotiable: the stopped source VM remains intact for rollback.
func (p *Probe) StartRemoteMigration(ctx context.Context, req RemoteMigrationRequest) (RemoteMigrationResult, error) {
	checks, err := p.RemotePreflight(ctx, req.Plan, req.Source, req.Target)
	if err != nil {
		return RemoteMigrationResult{}, err
	}
	for _, check := range checks {
		if check.Status == domain.CheckFail {
			return RemoteMigrationResult{}, fmt.Errorf("preflight failed: %s", check.Message)
		}
	}
	sourceInv, err := p.Inventory(ctx, req.Source)
	if err != nil {
		return RemoteMigrationResult{}, err
	}
	_, sourceNode, found := findSourceVM(sourceInv, req.Plan.SourceVMID)
	if !found {
		return RemoteMigrationResult{}, fmt.Errorf("source VM not found")
	}
	sourceVMID, err := parseQEMUVMID(req.Plan.SourceVMID)
	if err != nil {
		return RemoteMigrationResult{}, err
	}
	if req.TargetVMID <= 0 {
		targetClient, err := newAPIClient(req.Target)
		if err != nil {
			return RemoteMigrationResult{}, err
		}
		req.TargetVMID, err = readNextVMID(ctx, targetClient)
		if err != nil {
			return RemoteMigrationResult{}, fmt.Errorf("reserve target VMID: %w", err)
		}
	}
	remoteEndpoint, err := buildRemoteEndpoint(ctx, req.Target)
	if err != nil {
		return RemoteMigrationResult{}, err
	}
	values := url.Values{
		"target-endpoint": {remoteEndpoint},
		"target-vmid":     {strconv.Itoa(req.TargetVMID)},
		"target-storage":  {req.Plan.StorageMaps[0].TargetStorageID},
		"target-bridge":   {req.Plan.NetworkMaps[0].TargetBridge},
		"online":          {"0"},
		"delete":          {"0"},
	}
	sourceClient, err := newAPIClient(req.Source)
	if err != nil {
		return RemoteMigrationResult{}, err
	}
	path := fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/remote_migrate", url.PathEscape(sourceNode), sourceVMID)
	var upid string
	if err := sourceClient.postForm(ctx, path, values, &upid); err != nil {
		return RemoteMigrationResult{}, fmt.Errorf("start remote migration: %w", err)
	}
	if upid == "" {
		return RemoteMigrationResult{}, fmt.Errorf("Proxmox did not return a migration task ID")
	}
	return RemoteMigrationResult{UPID: upid, SourceNode: sourceNode, TargetNode: req.Plan.TargetNodeID, SourceVMID: sourceVMID, TargetVMID: req.TargetVMID}, nil
}

func (p *Probe) WaitRemoteMigration(ctx context.Context, source domain.Connection, result RemoteMigrationResult, progress func(RemoteMigrationProgress)) error {
	client, err := newAPIClient(source)
	if err != nil {
		return err
	}
	path := "/api2/json/nodes/" + url.PathEscape(result.SourceNode) + "/tasks/" + url.PathEscape(result.UPID) + "/status"
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		var status struct{ Status, ExitStatus string }
		if err := client.get(ctx, path, &status); err != nil {
			return fmt.Errorf("poll migration task: %w", err)
		}
		if status.Status == "stopped" {
			if status.ExitStatus != "OK" {
				return fmt.Errorf("Proxmox migration task failed: %s", status.ExitStatus)
			}
			return nil
		}
		if progress != nil {
			if current, ok := readTaskProgress(ctx, client, result); ok {
				progress(current)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func readTaskProgress(ctx context.Context, client *apiClient, result RemoteMigrationResult) (RemoteMigrationProgress, bool) {
	path := "/api2/json/nodes/" + url.PathEscape(result.SourceNode) + "/tasks/" + url.PathEscape(result.UPID) + "/log?start=0&limit=500"
	var lines []struct {
		N int    `json:"n"`
		T string `json:"t"`
	}
	if client.get(ctx, path, &lines) != nil {
		return RemoteMigrationProgress{}, false
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(apiTokenRE.ReplaceAllString(lines[i].T, "PVEAPIToken=[REDACTED]"))
		match := percentRE.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		value, err := strconv.ParseFloat(match[1], 64)
		if err == nil {
			return RemoteMigrationProgress{Percent: value, Message: line}, true
		}
	}
	return RemoteMigrationProgress{}, false
}

// VerifyRemoteMigration enforces the core rollback invariant after Proxmox
// reports success: source and target configurations both exist and are stopped.
func (p *Probe) VerifyRemoteMigration(ctx context.Context, source, target domain.Connection, result RemoteMigrationResult) error {
	sourceClient, err := newAPIClient(source)
	if err != nil {
		return err
	}
	targetClient, err := newAPIClient(target)
	if err != nil {
		return err
	}
	var sourceStatus struct {
		Status string `json:"status"`
	}
	if err := sourceClient.get(ctx, fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/current", url.PathEscape(result.SourceNode), result.SourceVMID), &sourceStatus); err != nil {
		return fmt.Errorf("source retention verification failed: %w", err)
	}
	if sourceStatus.Status != "stopped" {
		return fmt.Errorf("source VM was not retained in stopped state (state=%s)", sourceStatus.Status)
	}
	var targetStatus struct {
		Status string `json:"status"`
	}
	if err := targetClient.get(ctx, fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/current", url.PathEscape(result.TargetNode), result.TargetVMID), &targetStatus); err != nil {
		return fmt.Errorf("target verification failed: %w", err)
	}
	if targetStatus.Status != "stopped" {
		return fmt.Errorf("target VM is not isolated in stopped state (state=%s)", targetStatus.Status)
	}
	return nil
}

func buildRemoteEndpoint(ctx context.Context, target domain.Connection) (string, error) {
	u, err := url.Parse(normalizeCompare(target.Endpoint))
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("invalid target endpoint")
	}
	port := u.Port()
	if port == "" {
		port = "8006"
	}
	tokenID, tokenSecret, err := credentials(target.SecretRef)
	if err != nil {
		return "", err
	}
	fingerprint, err := tlsFingerprint(ctx, u.Hostname(), port)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("host=%s,port=%s,apitoken=PVEAPIToken=%s=%s,fingerprint=%s", u.Hostname(), port, tokenID, tokenSecret, fingerprint), nil
}

func tlsFingerprint(ctx context.Context, host, port string) (string, error) {
	dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 10 * time.Second}, Config: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}} // #nosec G402 -- fingerprint is pinned into the migration request
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return "", fmt.Errorf("read target TLS certificate: %w", err)
	}
	defer conn.Close()
	certs := conn.(*tls.Conn).ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return "", fmt.Errorf("target returned no TLS certificate")
	}
	sum := sha256.Sum256(certs[0].Raw)
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, ":"), nil
}

func parseQEMUVMID(id string) (int, error) {
	value := strings.TrimPrefix(id, "qemu-")
	vmid, err := strconv.Atoi(value)
	if err != nil || vmid < 100 {
		return 0, fmt.Errorf("invalid QEMU VM ID %q", id)
	}
	return vmid, nil
}
func normalizeCompare(endpoint string) string {
	value, err := normalizeEndpoint(endpoint)
	if err != nil {
		return endpoint
	}
	return value
}
func findSourceVM(inv domain.InventoryRoot, id string) (domain.VM, string, bool) {
	for _, dc := range inv.Datacenters {
		for _, cluster := range dc.Clusters {
			for _, host := range cluster.Hosts {
				for _, vm := range host.VMs {
					if vm.ID == id {
						return vm, host.ID, true
					}
				}
			}
		}
	}
	return domain.VM{}, "", false
}
func findTargetNode(inv domain.InventoryRoot, id string) (domain.TargetNode, bool) {
	for _, node := range inv.Nodes {
		if node.ID == id {
			return node, true
		}
	}
	return domain.TargetNode{}, false
}
func validStorageMaps(vm domain.VM, node domain.TargetNode, maps []domain.StorageMap) bool {
	if len(maps) != len(vm.Disks) {
		return false
	}
	valid := map[string]bool{}
	for _, s := range node.Storage {
		valid[s.ID] = true
	}
	seen := map[string]bool{}
	for _, m := range maps {
		if !valid[m.TargetStorageID] {
			return false
		}
		seen[m.SourceDiskID] = true
	}
	for _, d := range vm.Disks {
		if !seen[d.ID] {
			return false
		}
	}
	return true
}
func validNetworkMaps(vm domain.VM, node domain.TargetNode, maps []domain.NetworkMap) bool {
	if len(maps) != len(vm.NICs) {
		return false
	}
	valid := map[string]bool{}
	for _, b := range node.Bridges {
		valid[b.Name] = true
	}
	seen := map[string]bool{}
	for _, m := range maps {
		if !valid[m.TargetBridge] {
			return false
		}
		seen[m.SourceNICID] = true
	}
	for _, n := range vm.NICs {
		if !seen[n.ID] {
			return false
		}
	}
	return true
}
func uniformTargetStorage(maps []domain.StorageMap) bool {
	if len(maps) == 0 {
		return false
	}
	values := []string{}
	for _, m := range maps {
		values = append(values, m.TargetStorageID)
	}
	sort.Strings(values)
	return values[0] == values[len(values)-1]
}
func uniformTargetBridge(maps []domain.NetworkMap) bool {
	if len(maps) == 0 {
		return true
	}
	values := []string{}
	for _, m := range maps {
		values = append(values, m.TargetBridge)
	}
	sort.Strings(values)
	return values[0] == values[len(values)-1]
}
