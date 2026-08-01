package proxmox

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drishti/hypershift/internal/domain"
)

func TestStartRemoteMigrationUsesOfflineRetainSourceContract(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data string
		switch r.URL.Path {
		case "/api2/json/cluster/resources":
			data = `[{"type":"node","node":"pve","status":"online"}]`
		case "/api2/json/nodes/pve/qemu":
			data = `[]`
		case "/api2/json/nodes/pve/status":
			data = `{"cpu":0,"cpuinfo":{"cpus":8,"mhz":2400},"memory":{"total":34359738368,"used":0}}`
		case "/api2/json/nodes/pve/storage":
			data = `[{"storage":"local-lvm","type":"lvmthin","content":"images,rootdir","total":107374182400,"avail":85899345920,"active":1,"enabled":1}]`
		case "/api2/json/nodes/pve/network":
			data = `[{"iface":"vmbr0","type":"bridge","bridge_vlan_aware":1}]`
		case "/api2/json/cluster/nextid":
			data = `200`
		default:
			http.Error(w, "unexpected target path", http.StatusNotFound)
			return
		}
		writeEnvelope(t, w, data)
	}))
	defer target.Close()

	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data string
		switch r.URL.Path {
		case "/api2/json/cluster/resources":
			data = `[{"type":"node","node":"lpclab","status":"online"}]`
		case "/api2/json/nodes/lpclab/qemu":
			data = `[{"vmid":126,"name":"disposable","status":"stopped","maxcpu":2,"maxmem":2147483648}]`
		case "/api2/json/nodes/lpclab/qemu/126/config":
			data = `{"name":"disposable","cores":2,"memory":2048,"scsi0":"source-zfs:vm-126-disk-0,size=8G","net0":"virtio=BC:24:11:00:00:01,bridge=vmbr0"}`
		case "/api2/json/nodes/lpclab/qemu/126/remote_migrate":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("delete") != "0" || r.Form.Get("online") != "0" {
				t.Fatalf("unsafe migration flags: %v", r.Form)
			}
			if r.Form.Get("target-storage") != "local-lvm" || r.Form.Get("target-bridge") != "vmbr0" || r.Form.Has("target-node") {
				t.Fatalf("mapping form = %v", r.Form)
			}
			if !strings.Contains(r.Form.Get("target-endpoint"), "fingerprint=") {
				t.Fatal("target fingerprint missing")
			}
			data = `"UPID:lpclab:00000001:remote_migrate:126:root@pam:"`
		default:
			http.Error(w, "unexpected source path", http.StatusNotFound)
			return
		}
		writeEnvelope(t, w, data)
	}))
	defer source.Close()

	t.Setenv("DRISHTI_SECRET_SRC_TOKEN_ID", "drishti@pve!source")
	t.Setenv("DRISHTI_SECRET_SRC_TOKEN_SECRET", "source-secret")
	t.Setenv("DRISHTI_SECRET_DST_TOKEN_ID", "drishti@pve!target")
	t.Setenv("DRISHTI_SECRET_DST_TOKEN_SECRET", "target-secret")
	request := RemoteMigrationRequest{
		Source: domain.Connection{Kind: domain.PlatformProxmox, Role: domain.RoleSource, Endpoint: source.URL, SecretRef: "src", InsecureTLS: true},
		Target: domain.Connection{Kind: domain.PlatformProxmox, Role: domain.RoleTarget, Endpoint: target.URL, SecretRef: "dst", InsecureTLS: true},
		Plan:   domain.Plan{SourceVMID: "qemu-126", TargetNodeID: "pve", StorageMaps: []domain.StorageMap{{SourceDiskID: "scsi0", TargetStorageID: "local-lvm", TargetFormat: domain.DiskRaw}}, NetworkMaps: []domain.NetworkMap{{SourceNICID: "net0", TargetBridge: "vmbr0"}}},
	}
	result, err := NewProbe().StartRemoteMigration(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetVMID != 200 || result.SourceVMID != 126 || result.UPID == "" {
		t.Fatalf("result = %+v", result)
	}
}

func writeEnvelope(t *testing.T, w http.ResponseWriter, data string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"data":%s}`, data)
}

func TestReadTaskProgressUsesLatestPercentage(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, `[{"n":1,"t":"drive-scsi0: transferred 10.0 GiB of 100.0 GiB (10.00%)"},{"n":2,"t":"drive-scsi0: transferred 42.0 GiB of 100.0 GiB (42.00%)"}]`)
	}))
	defer srv.Close()
	client, err := newAPIClient(testConnection(t, srv.URL, domain.RoleSource))
	if err != nil {
		t.Fatal(err)
	}
	progress, ok := readTaskProgress(context.Background(), client, RemoteMigrationResult{SourceNode: "pve", UPID: "UPID:test"})
	if !ok || progress.Percent != 42 || !strings.Contains(progress.Message, "42.00%") {
		t.Fatalf("progress = %+v, ok=%v", progress, ok)
	}
}
