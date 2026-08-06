package proxmox

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drishti/hypershift/internal/domain"
)

func TestSourceInventoryReadsQEMUConfiguration(t *testing.T) {
	srv := newInventoryServer(t)
	defer srv.Close()
	conn := testConnection(t, srv.URL, domain.RoleSource)
	inv, err := NewProbe().Inventory(context.Background(), conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Datacenters) != 1 || len(inv.Datacenters[0].Clusters) != 1 {
		t.Fatalf("unexpected hierarchy: %+v", inv)
	}
	hosts := inv.Datacenters[0].Clusters[0].Hosts
	if len(hosts) != 1 || len(hosts[0].VMs) != 1 {
		t.Fatalf("hosts/VMs = %+v", hosts)
	}
	vm := hosts[0].VMs[0]
	if vm.ID != "qemu-101" || vm.Name != "lab-vm" || vm.CPUs != 4 || vm.MemoryMB != 4096 {
		t.Fatalf("VM = %+v", vm)
	}
	if vm.Firmware != domain.FirmwareUEFI || vm.GuestFamily != domain.GuestLinux {
		t.Fatalf("firmware/family = %s/%s", vm.Firmware, vm.GuestFamily)
	}
	if len(vm.Disks) != 1 || vm.Disks[0].CapacityBytes != 32*1024*1024*1024 {
		t.Fatalf("disks = %+v", vm.Disks)
	}
	if len(vm.NICs) != 1 || vm.NICs[0].NetworkID != "vmbr0" {
		t.Fatalf("NICs = %+v", vm.NICs)
	}
}

func TestTargetInventoryReadsCapacityStorageAndBridges(t *testing.T) {
	srv := newInventoryServer(t)
	defer srv.Close()
	conn := testConnection(t, srv.URL, domain.RoleTarget)
	inv, err := NewProbe().Inventory(context.Background(), conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Nodes) != 1 {
		t.Fatalf("nodes = %+v", inv.Nodes)
	}
	node := inv.Nodes[0]
	if node.ID != "pve" || node.NextVMID != 102 || len(node.VMs) != 1 {
		t.Fatalf("node = %+v", node)
	}
	if node.CPUTotalMHz != 19200 || node.MemoryTotalMB != 32768 {
		t.Fatalf("capacity = %+v", node)
	}
	if len(node.Storage) != 1 || node.Storage[0].ID != "local-lvm" {
		t.Fatalf("storage = %+v", node.Storage)
	}
	if len(node.Bridges) != 1 || node.Bridges[0].Name != "vmbr0" || !node.Bridges[0].VLANAware {
		t.Fatalf("bridges = %+v", node.Bridges)
	}
}

func testConnection(t *testing.T, endpoint string, role domain.Role) domain.Connection {
	t.Helper()
	t.Setenv("DRISHTI_SECRET_LAB_TOKEN_ID", "user@pve!drishti")
	t.Setenv("DRISHTI_SECRET_LAB_TOKEN_SECRET", "topsecret")
	return domain.Connection{ID: "conn-live", Kind: domain.PlatformProxmox, Role: role, Endpoint: endpoint, SecretRef: "lab", InsecureTLS: true}
}

func newInventoryServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "PVEAPIToken=user@pve!drishti=topsecret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var data string
		switch r.URL.Path {
		case "/api2/json/cluster/resources":
			data = `[{"type":"node","node":"pve","status":"online","maxmem":34359738368,"mem":8589934592},{"type":"qemu","node":"pve","vmid":101,"name":"lab-vm","status":"stopped","maxcpu":4,"maxmem":4294967296},{"type":"lxc","node":"pve","vmid":201,"name":"excluded-container"}]`
		case "/api2/json/nodes/pve/qemu/101/config":
			data = `{"name":"lab-vm","cores":2,"sockets":2,"memory":4096,"bios":"ovmf","ostype":"l26","agent":"1","description":"Disposable lab VM","scsi0":"local-lvm:vm-101-disk-0,size=32G","ide2":"local:iso/test.iso,media=cdrom","net0":"virtio=BC:24:11:00:00:01,bridge=vmbr0"}`
		case "/api2/json/nodes/pve/qemu":
			data = `[{"vmid":101,"name":"lab-vm","status":"stopped","maxcpu":4,"maxmem":4294967296},{"vmid":999,"name":"excluded-template","template":1}]`
		case "/api2/json/cluster/nextid":
			data = `"102"`
		case "/api2/json/nodes/pve/status":
			data = `{"cpu":0.25,"cpuinfo":{"cpus":8,"mhz":2400},"memory":{"total":34359738368,"used":8589934592}}`
		case "/api2/json/nodes/pve/storage":
			data = `[{"storage":"local-lvm","type":"lvmthin","content":"images,rootdir","total":107374182400,"avail":64424509440,"shared":0,"active":1,"enabled":1},{"storage":"local","type":"dir","content":"iso,backup","total":1,"avail":1,"active":1,"enabled":1}]`
		case "/api2/json/nodes/pve/network":
			data = `[{"iface":"vmbr0","type":"bridge","bridge_ports":"eno1","bridge_vlan_aware":1}]`
		default:
			http.Error(w, fmt.Sprintf("unexpected path %s", r.URL.Path), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":%s}`, data)
	}))
}
