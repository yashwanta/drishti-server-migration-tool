package proxmox

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/drishti/hypershift/internal/config"
	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/platform"
	"github.com/drishti/hypershift/internal/safety"
)

func TestTargetCreateVMIsProtectedStoppedIsolatedAndIdempotent(t *testing.T) {
	var mu sync.Mutex
	created := false
	createCalls := 0
	marker := ownershipMarker("plan-create-1")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		assertTargetAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/resources":
			data := `[{"type":"node","node":"pve","status":"online"}]`
			if created {
				data = `[{"type":"node","node":"pve","status":"online"},{"type":"qemu","node":"pve","vmid":420,"status":"stopped"}]`
			}
			writeTargetEnvelope(t, w, data)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/network":
			writeTargetEnvelope(t, w, `[{"iface":"vmbr-lab","type":"bridge"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/storage":
			writeTargetEnvelope(t, w, `[{"storage":"local-lvm","content":"images","active":1,"enabled":1,"avail":1073741824}]`)
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/pve/qemu":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("protection") != "1" || !strings.Contains(r.Form.Get("net0"), "link_down=1") || r.Form.Get("onboot") != "0" {
				t.Fatalf("unsafe create form: %v", r.Form)
			}
			if r.Form.Get("description") != marker || r.Form.Get("bios") != "ovmf" || !strings.HasPrefix(r.Form.Get("efidisk0"), "local-lvm:") {
				t.Fatalf("unexpected create form: %v", r.Form)
			}
			createCalls++
			created = true
			writeTargetEnvelope(t, w, `"UPID:create"`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tasks/"):
			writeTargetEnvelope(t, w, `{"status":"stopped","exitstatus":"OK"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/qemu/420/config":
			if !created {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			writeTargetEnvelope(t, w, fmt.Sprintf(`{"name":"labvm","cores":2,"memory":2048,"bios":"ovmf","protection":1,"description":%q,"efidisk0":"local-lvm:vm-420-disk-0","net0":"virtio,bridge=vmbr-lab,firewall=1,link_down=1"}`, marker))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/qemu/420/status/current":
			writeTargetEnvelope(t, w, `{"status":"stopped"}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()
	target := newFixtureTarget(t, srv.URL, safety.MutationPolicy{Mode: config.ModeLab, Enabled: true})
	spec := platform.CreateVMSpec{Name: "labvm", NodeID: "pve", VMID: 420, CPU: 2, MemoryMB: 2048, Firmware: domain.FirmwareUEFI, IdempotencyKey: "plan-create-1", IsolatedBridge: "vmbr-lab", StorageID: "local-lvm"}
	if _, err := target.CreateVM(context.Background(), spec); err != nil {
		t.Fatalf("CreateVM: %v", err)
	}
	if _, err := target.CreateVM(context.Background(), spec); err != nil {
		t.Fatalf("idempotent CreateVM: %v", err)
	}
	if createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", createCalls)
	}
}

func TestTargetCreateVMRejectsProductionAndDenylistedEndpointsWithoutHTTP(t *testing.T) {
	requests := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()
	spec := platform.CreateVMSpec{Name: "labvm", NodeID: "pve", VMID: 420, CPU: 2, MemoryMB: 2048, Firmware: domain.FirmwareBIOS, IdempotencyKey: "plan", IsolatedBridge: "vmbr-lab"}
	production := newFixtureTarget(t, srv.URL, safety.MutationPolicy{Mode: config.ModeProduction, Enabled: true})
	if _, err := production.CreateVM(context.Background(), spec); err == nil {
		t.Fatal("expected production mutation refusal")
	}
	u := strings.TrimPrefix(srv.URL, "https://")
	denylisted := newFixtureTarget(t, srv.URL, safety.MutationPolicy{Mode: config.ModeLab, Enabled: true, Denylist: []string{u}})
	if _, err := denylisted.CreateVM(context.Background(), spec); err == nil {
		t.Fatal("expected denylisted endpoint refusal")
	}
	nonAllowlisted := newFixtureTarget(t, srv.URL, safety.MutationPolicy{Mode: config.ModeLab, Enabled: true})
	spec.IsolatedBridge = "vmbr-production"
	if _, err := nonAllowlisted.CreateVM(context.Background(), spec); err == nil {
		t.Fatal("expected non-allowlisted bridge refusal")
	}
	if requests != 0 {
		t.Fatalf("refused mutations made %d HTTP requests", requests)
	}
}

func TestNewTargetRequiresImportRootAndIsolationAllowlist(t *testing.T) {
	t.Setenv("DRISHTI_SECRET_TARGET_TOKEN_ID", "root@pam!drishti")
	t.Setenv("DRISHTI_SECRET_TARGET_TOKEN_SECRET", "fixture-secret")
	conn := domain.Connection{ID: "target", Kind: domain.PlatformProxmox, Role: domain.RoleTarget, Endpoint: "https://127.0.0.1:8006", SecretRef: "target"}
	policy := safety.MutationPolicy{Mode: config.ModeLab, Enabled: true}
	if _, err := NewTarget(conn, policy, "", []string{"vmbr-lab"}); err == nil {
		t.Fatal("expected missing import root to be rejected")
	}
	if _, err := NewTarget(conn, policy, "/mnt/drishti-import", nil); err == nil {
		t.Fatal("expected missing isolation allowlist to be rejected")
	}
}

func TestTargetCreateVMRejectsForeignExistingVMID(t *testing.T) {
	postCalls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTargetAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/resources":
			writeTargetEnvelope(t, w, `[{"type":"node","node":"pve","status":"online"},{"type":"qemu","node":"other","vmid":420,"status":"stopped"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/network":
			writeTargetEnvelope(t, w, `[{"iface":"vmbr-lab","type":"bridge"}]`)
		case r.Method == http.MethodPost:
			postCalls++
			writeTargetEnvelope(t, w, `null`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()
	target := newFixtureTarget(t, srv.URL, safety.MutationPolicy{Mode: config.ModeLab, Enabled: true})
	spec := platform.CreateVMSpec{Name: "labvm", NodeID: "pve", VMID: 420, CPU: 2, MemoryMB: 2048, Firmware: domain.FirmwareBIOS, IdempotencyKey: "plan", IsolatedBridge: "vmbr-lab"}
	if _, err := target.CreateVM(context.Background(), spec); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected foreign VMID refusal, got %v", err)
	}
	if postCalls != 0 {
		t.Fatalf("foreign VMID caused %d mutation requests", postCalls)
	}
}

func TestTargetCreateVMReportsFailedProxmoxTask(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTargetAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/resources":
			writeTargetEnvelope(t, w, `[{"type":"node","node":"pve","status":"online"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/network":
			writeTargetEnvelope(t, w, `[{"iface":"vmbr-lab","type":"bridge"}]`)
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/pve/qemu":
			writeTargetEnvelope(t, w, `"UPID:create-failed"`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tasks/"):
			writeTargetEnvelope(t, w, `{"status":"stopped","exitstatus":"ERROR"}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()
	target := newFixtureTarget(t, srv.URL, safety.MutationPolicy{Mode: config.ModeLab, Enabled: true})
	spec := platform.CreateVMSpec{Name: "labvm", NodeID: "pve", VMID: 420, CPU: 2, MemoryMB: 2048, Firmware: domain.FirmwareBIOS, IdempotencyKey: "plan", IsolatedBridge: "vmbr-lab"}
	if _, err := target.CreateVM(context.Background(), spec); err == nil || !strings.Contains(err.Error(), "task failed") {
		t.Fatalf("expected task failure, got %v", err)
	}
}

func newFixtureTarget(t *testing.T, endpoint string, policy safety.MutationPolicy) *Target {
	t.Helper()
	t.Setenv("DRISHTI_SECRET_TARGET_TOKEN_ID", "root@pam!drishti")
	t.Setenv("DRISHTI_SECRET_TARGET_TOKEN_SECRET", "fixture-secret")
	target, err := NewTarget(domain.Connection{ID: "target", Kind: domain.PlatformProxmox, Role: domain.RoleTarget, Endpoint: endpoint, SecretRef: "target", InsecureTLS: true}, policy, "/mnt/drishti-import", []string{"vmbr-lab"})
	if err != nil {
		t.Fatal(err)
	}
	target.pollInterval = time.Millisecond
	return target
}

func assertTargetAuth(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "PVEAPIToken=root@pam!drishti=fixture-secret" {
		t.Fatalf("authorization = %q", got)
	}
}

func writeTargetEnvelope(t *testing.T, w http.ResponseWriter, data string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"data":%s}`, data)
}
