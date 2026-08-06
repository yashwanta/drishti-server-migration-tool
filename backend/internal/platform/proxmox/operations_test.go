package proxmox

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/drishti/hypershift/internal/config"
	"github.com/drishti/hypershift/internal/safety"
)

func TestTargetStartStopAndIsolationUseOnlySafeOperations(t *testing.T) {
	var mu sync.Mutex
	state := "stopped"
	net0 := "virtio,bridge=vmbr-lab,firewall=1,link_down=1"
	postActions := []string{}
	owner := ownershipMarker("plan-ops")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		assertTargetAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/resources":
			writeTargetEnvelope(t, w, fmt.Sprintf(`[{"type":"qemu","node":"pve","vmid":420,"status":%q}]`, state))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/qemu/420/config":
			writeTargetEnvelope(t, w, fmt.Sprintf(`{"description":%q,"net0":%q}`, owner, net0))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/qemu/420/status/current":
			writeTargetEnvelope(t, w, fmt.Sprintf(`{"status":%q}`, state))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/network":
			writeTargetEnvelope(t, w, `[{"iface":"vmbr-lab","type":"bridge"}]`)
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/pve/qemu/420/config":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			net0 = r.Form.Get("net0")
			postActions = append(postActions, "isolate")
			writeTargetEnvelope(t, w, `null`)
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/pve/qemu/420/status/start":
			state = "running"
			postActions = append(postActions, "start")
			writeTargetEnvelope(t, w, `"UPID:start"`)
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/pve/qemu/420/status/shutdown":
			state = "stopped"
			postActions = append(postActions, "shutdown")
			writeTargetEnvelope(t, w, `"UPID:shutdown"`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tasks/"):
			writeTargetEnvelope(t, w, `{"status":"stopped","exitstatus":"OK"}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()
	target := newFixtureTarget(t, srv.URL, safety.MutationPolicy{Mode: config.ModeLab, Enabled: true})
	if err := target.SetNetwork(context.Background(), 420, "vmbr-lab", 200, true); err != nil {
		t.Fatalf("SetNetwork: %v", err)
	}
	if !strings.Contains(net0, "link_down=1") || !strings.Contains(net0, "tag=200") {
		t.Fatalf("unsafe isolated network config: %q", net0)
	}
	if err := target.StartVM(context.Background(), 420); err != nil {
		t.Fatalf("StartVM: %v", err)
	}
	if err := target.StopVM(context.Background(), 420); err != nil {
		t.Fatalf("StopVM: %v", err)
	}
	if strings.Join(postActions, ",") != "isolate,start,shutdown" {
		t.Fatalf("actions = %v", postActions)
	}
}

func TestTargetRefusesNonIsolatedNetworkWithoutHTTP(t *testing.T) {
	requests := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()
	target := newFixtureTarget(t, srv.URL, safety.MutationPolicy{Mode: config.ModeLab, Enabled: true})
	if err := target.SetNetwork(context.Background(), 420, "vmbr0", 0, false); err == nil {
		t.Fatal("expected non-isolated network refusal")
	}
	if err := target.SetNetwork(context.Background(), 420, "vmbr0", 0, true); err == nil {
		t.Fatal("expected non-allowlisted isolation bridge refusal")
	}
	if requests != 0 {
		t.Fatalf("refused network change made %d HTTP requests", requests)
	}
}

func TestTargetStartRefusesConnectedNICWithoutMutation(t *testing.T) {
	postCalls := 0
	owner := ownershipMarker("plan-ops")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTargetAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/resources":
			writeTargetEnvelope(t, w, `[{"type":"qemu","node":"pve","vmid":420,"status":"stopped"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/qemu/420/config":
			writeTargetEnvelope(t, w, fmt.Sprintf(`{"description":%q,"net0":"virtio,bridge=vmbr0"}`, owner))
		case r.Method == http.MethodPost:
			postCalls++
			writeTargetEnvelope(t, w, `null`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()
	target := newFixtureTarget(t, srv.URL, safety.MutationPolicy{Mode: config.ModeLab, Enabled: true})
	if err := target.StartVM(context.Background(), 420); err == nil || !strings.Contains(err.Error(), "not link-down") {
		t.Fatalf("expected connected NIC refusal, got %v", err)
	}
	if postCalls != 0 {
		t.Fatalf("unsafe start caused %d mutation calls", postCalls)
	}
}
