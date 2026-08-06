package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/drishti/hypershift/internal/domain"
)

const mib = 1024 * 1024

type resource struct {
	Type     string  `json:"type"`
	Node     string  `json:"node"`
	Name     string  `json:"name"`
	Status   string  `json:"status"`
	VMID     int     `json:"vmid"`
	Template int     `json:"template"`
	CPU      float64 `json:"cpu"`
	MaxCPU   float64 `json:"maxcpu"`
	Mem      float64 `json:"mem"`
	MaxMem   float64 `json:"maxmem"`
}

func readNodeQEMU(ctx context.Context, client *apiClient, node string) ([]resource, error) {
	var guests []resource
	path := "/api2/json/nodes/" + url.PathEscape(node) + "/qemu"
	if err := client.get(ctx, path, &guests); err != nil {
		return nil, fmt.Errorf("list QEMU VMs on node %s (token needs VM.Audit): %w", node, err)
	}
	for i := range guests {
		guests[i].Type = "qemu"
		guests[i].Node = node
	}
	return guests, nil
}

// Inventory returns live, read-only inventory normalized for the connection role.
func (p *Probe) Inventory(ctx context.Context, conn domain.Connection) (domain.InventoryRoot, error) {
	if conn.Kind != domain.PlatformProxmox {
		return domain.InventoryRoot{}, fmt.Errorf("real inventory is not implemented for platform %q", conn.Kind)
	}
	client, err := newAPIClient(conn)
	if err != nil {
		return domain.InventoryRoot{}, err
	}
	var resources []resource
	if err := client.get(ctx, "/api2/json/cluster/resources", &resources); err != nil {
		return domain.InventoryRoot{}, err
	}
	if conn.Role == domain.RoleSource {
		return sourceInventory(ctx, client, conn.ID, resources)
	}
	return targetInventory(ctx, client, conn.ID, resources)
}

func sourceInventory(ctx context.Context, client *apiClient, connID string, resources []resource) (domain.InventoryRoot, error) {
	hosts := map[string]*domain.Host{}
	for _, r := range resources {
		if r.Type == "node" {
			hosts[r.Node] = &domain.Host{ID: r.Node, Name: r.Node, PowerState: r.Status, MemoryTotalMB: int64(r.MaxMem / mib), MemoryUsedMB: int64(r.Mem / mib), VMs: []domain.VM{}}
		}
	}
	for nodeName, host := range hosts {
		guests, err := readNodeQEMU(ctx, client, nodeName)
		if err != nil {
			return domain.InventoryRoot{}, err
		}
		for _, r := range guests {
			if r.Template != 0 {
				continue
			}
			vm, err := readSourceVM(ctx, client, r)
			if err != nil {
				return domain.InventoryRoot{}, fmt.Errorf("read VM %d on node %s: %w", r.VMID, r.Node, err)
			}
			host.VMs = append(host.VMs, vm)
		}
	}
	hostList := make([]domain.Host, 0, len(hosts))
	for _, host := range hosts {
		sort.Slice(host.VMs, func(i, j int) bool { return host.VMs[i].Name < host.VMs[j].Name })
		hostList = append(hostList, *host)
	}
	sort.Slice(hostList, func(i, j int) bool { return hostList[i].Name < hostList[j].Name })
	return domain.InventoryRoot{ConnectionID: connID, GeneratedAt: time.Now().UTC(), Datacenters: []domain.Datacenter{{ID: "pve-datacenter", Name: "Proxmox VE", Clusters: []domain.Cluster{{ID: "pve-cluster", Name: "Proxmox Cluster", Hosts: hostList}}}}}, nil
}

func readSourceVM(ctx context.Context, client *apiClient, r resource) (domain.VM, error) {
	var cfg map[string]json.RawMessage
	path := fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/config", url.PathEscape(r.Node), r.VMID)
	if err := client.get(ctx, path, &cfg); err != nil {
		return domain.VM{}, err
	}
	name := rawString(cfg["name"])
	if name == "" {
		name = r.Name
	}
	vm := domain.VM{
		ID: fmt.Sprintf("qemu-%d", r.VMID), Name: name, HostID: r.Node,
		PowerState: powerState(r.Status), CPUs: rawInt(cfg["cores"]) * max(1, rawInt(cfg["sockets"])),
		MemoryMB: int64(rawInt(cfg["memory"])), Firmware: domain.FirmwareBIOS,
		GuestOS: rawString(cfg["ostype"]), GuestFamily: guestFamily(rawString(cfg["ostype"])),
		ToolsStatus: agentStatus(rawString(cfg["agent"])), Notes: rawString(cfg["description"]),
		Disks: []domain.Disk{}, NICs: []domain.NIC{}, Snapshots: []domain.Snapshot{}, DatastoreIDs: []string{}, NetworkIDs: []string{},
	}
	if vm.CPUs == 0 {
		vm.CPUs = int(r.MaxCPU)
	}
	if vm.MemoryMB == 0 {
		vm.MemoryMB = int64(r.MaxMem / mib)
	}
	if rawString(cfg["bios"]) == "ovmf" {
		vm.Firmware = domain.FirmwareUEFI
	}
	for key, raw := range cfg {
		value := rawString(raw)
		if isDiskKey(key) && !strings.Contains(value, "media=cdrom") && value != "none" {
			disk := parseDisk(key, value)
			vm.Disks = append(vm.Disks, disk)
			if disk.DatastoreID != "" {
				vm.DatastoreIDs = appendUnique(vm.DatastoreIDs, disk.DatastoreID)
			}
		}
		if strings.HasPrefix(key, "net") && isNumericSuffix(key, 3) {
			nic := parseNIC(key, value)
			vm.NICs = append(vm.NICs, nic)
			if nic.NetworkID != "" {
				vm.NetworkIDs = appendUnique(vm.NetworkIDs, nic.NetworkID)
			}
		}
	}
	sort.Slice(vm.Disks, func(i, j int) bool { return vm.Disks[i].ID < vm.Disks[j].ID })
	sort.Slice(vm.NICs, func(i, j int) bool { return vm.NICs[i].ID < vm.NICs[j].ID })
	return vm, nil
}

func targetInventory(ctx context.Context, client *apiClient, connID string, resources []resource) (domain.InventoryRoot, error) {
	nextID, err := readNextVMID(ctx, client)
	if err != nil {
		return domain.InventoryRoot{}, err
	}
	var nodes []domain.TargetNode
	for _, r := range resources {
		if r.Type != "node" {
			continue
		}
		node := domain.TargetNode{ID: r.Node, Name: r.Node, Online: r.Status == "online", MemoryTotalMB: int64(r.MaxMem / mib), MemoryUsedMB: int64(r.Mem / mib), NextVMID: nextID, Storage: []domain.TargetStorage{}, Bridges: []domain.Bridge{}, VMs: []domain.TargetVM{}}
		readNodeCapacity(ctx, client, &node)
		guests, err := readNodeQEMU(ctx, client, r.Node)
		if err != nil {
			return domain.InventoryRoot{}, err
		}
		for _, vmr := range guests {
			if vmr.Template == 0 {
				node.VMs = append(node.VMs, domain.TargetVM{ID: vmr.VMID, Name: vmr.Name, NodeID: r.Node, Status: powerState(vmr.Status), CPUs: int(vmr.MaxCPU), MemoryMB: int64(vmr.MaxMem / mib)})
			}
		}
		if err := readNodeStorage(ctx, client, &node); err != nil {
			return domain.InventoryRoot{}, err
		}
		if err := readNodeBridges(ctx, client, &node); err != nil {
			return domain.InventoryRoot{}, err
		}
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	return domain.InventoryRoot{ConnectionID: connID, GeneratedAt: time.Now().UTC(), Nodes: nodes}, nil
}

func readNextVMID(ctx context.Context, client *apiClient) (int, error) {
	var raw json.RawMessage
	if err := client.get(ctx, "/api2/json/cluster/nextid", &raw); err != nil {
		return 0, fmt.Errorf("read next target VMID: %w", err)
	}
	vmid := rawInt(raw)
	if vmid < 100 {
		return 0, fmt.Errorf("read next target VMID: Proxmox returned %q", rawString(raw))
	}
	return vmid, nil
}

func readNodeStorage(ctx context.Context, client *apiClient, node *domain.TargetNode) error {
	var data []struct {
		Storage string `json:"storage"`
		Type    string `json:"type"`
		Content string `json:"content"`
		Total   int64  `json:"total"`
		Avail   int64  `json:"avail"`
		Shared  int    `json:"shared"`
		Active  int    `json:"active"`
		Enabled int    `json:"enabled"`
	}
	path := "/api2/json/nodes/" + url.PathEscape(node.ID) + "/storage"
	if err := client.get(ctx, path, &data); err != nil {
		return fmt.Errorf("read storage for node %s: %w", node.ID, err)
	}
	for _, s := range data {
		if s.Active == 0 || s.Enabled == 0 || !containsCSV(s.Content, "images") {
			continue
		}
		node.Storage = append(node.Storage, domain.TargetStorage{ID: s.Storage, Name: s.Storage, Type: s.Type, ContentTypes: splitCSV(s.Content), CapacityBytes: s.Total, FreeBytes: s.Avail, Shared: s.Shared != 0})
	}
	return nil
}

func readNodeBridges(ctx context.Context, client *apiClient, node *domain.TargetNode) error {
	var data []struct {
		Iface       string `json:"iface"`
		Type        string `json:"type"`
		BridgePorts string `json:"bridge_ports"`
		BridgeVLAN  int    `json:"bridge_vlan_aware"`
	}
	path := "/api2/json/nodes/" + url.PathEscape(node.ID) + "/network?type=bridge"
	if err := client.get(ctx, path, &data); err != nil {
		return fmt.Errorf("read bridges for node %s: %w", node.ID, err)
	}
	for _, b := range data {
		if b.Type != "bridge" {
			continue
		}
		vlan := b.BridgeVLAN != 0
		node.Bridges = append(node.Bridges, domain.Bridge{ID: b.Iface, Name: b.Iface, VLANAware: vlan, Ports: strings.Fields(b.BridgePorts)})
		node.VLANAware = node.VLANAware || vlan
	}
	return nil
}

func readNodeCapacity(ctx context.Context, client *apiClient, node *domain.TargetNode) {
	var status struct {
		CPU     float64 `json:"cpu"`
		CPUInfo struct {
			CPUs int     `json:"cpus"`
			MHz  float64 `json:"mhz"`
		} `json:"cpuinfo"`
		Memory struct {
			Total int64 `json:"total"`
			Used  int64 `json:"used"`
		} `json:"memory"`
	}
	path := "/api2/json/nodes/" + url.PathEscape(node.ID) + "/status"
	if client.get(ctx, path, &status) != nil {
		return
	}
	node.CPUTotalMHz = int64(float64(status.CPUInfo.CPUs) * status.CPUInfo.MHz)
	node.CPUUsedMHz = int64(float64(node.CPUTotalMHz) * status.CPU)
	if status.Memory.Total > 0 {
		node.MemoryTotalMB = status.Memory.Total / mib
		node.MemoryUsedMB = status.Memory.Used / mib
	}
}

func rawString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return ""
}
func rawInt(raw json.RawMessage) int { n, _ := strconv.Atoi(rawString(raw)); return n }
func powerState(s string) domain.PowerState {
	if s == "running" || s == "online" {
		return domain.PowerOn
	}
	if s == "paused" {
		return domain.PowerSuspended
	}
	return domain.PowerOff
}
func guestFamily(s string) domain.GuestFamily {
	if strings.HasPrefix(s, "win") {
		return domain.GuestWindows
	}
	if s == "l26" || strings.Contains(s, "linux") {
		return domain.GuestLinux
	}
	return domain.GuestOther
}
func agentStatus(s string) string {
	if s == "" || s == "0" {
		return "disabled"
	}
	return "enabled"
}
func isDiskKey(k string) bool {
	for _, p := range []string{"scsi", "virtio", "sata", "ide"} {
		if strings.HasPrefix(k, p) && isNumericSuffix(k, len(p)) {
			return true
		}
	}
	return false
}
func isNumericSuffix(s string, start int) bool {
	if len(s) <= start {
		return false
	}
	_, err := strconv.Atoi(s[start:])
	return err == nil
}
func parseDisk(id, value string) domain.Disk {
	parts := strings.Split(value, ",")
	storage := strings.SplitN(parts[0], ":", 2)[0]
	opts := options(parts[1:])
	return domain.Disk{ID: id, Label: id, CapacityBytes: parseSize(opts["size"]), Format: diskFormat(opts["format"], parts[0]), Controller: strings.TrimRight(id, "0123456789"), Thin: true, DatastoreID: storage}
}
func parseNIC(id, value string) domain.NIC {
	parts := strings.Split(value, ",")
	first := strings.SplitN(parts[0], "=", 2)
	model, mac := first[0], ""
	if len(first) == 2 {
		mac = first[1]
	}
	opts := options(parts[1:])
	return domain.NIC{ID: id, Label: id, MACAddress: mac, NetworkID: opts["bridge"], Connected: opts["link_down"] != "1", Model: model}
}
func options(parts []string) map[string]string {
	out := map[string]string{}
	for _, p := range parts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			out[kv[0]] = kv[1]
		}
	}
	return out
}
func parseSize(s string) int64 {
	if s == "" {
		return 0
	}
	units := map[byte]int64{'K': 1024, 'M': mib, 'G': 1024 * mib, 'T': 1024 * 1024 * mib}
	last := s[len(s)-1]
	mult := int64(1)
	if v, ok := units[last]; ok {
		mult = v
		s = s[:len(s)-1]
	}
	n, _ := strconv.ParseFloat(s, 64)
	return int64(n * float64(mult))
}
func diskFormat(explicit, volume string) domain.DiskFormat {
	value := explicit + " " + volume
	if strings.Contains(value, "qcow2") {
		return domain.DiskQCOW2
	}
	return domain.DiskRaw
}
func appendUnique(xs []string, value string) []string {
	for _, x := range xs {
		if x == value {
			return xs
		}
	}
	return append(xs, value)
}
func splitCSV(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
func containsCSV(s, want string) bool {
	for _, v := range splitCSV(s) {
		if v == want {
			return true
		}
	}
	return false
}
