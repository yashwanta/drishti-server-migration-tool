package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drishti/hypershift/internal/config"
	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/mock"
)

func newTestHandlers(t *testing.T) (*Handlers, *http.ServeMux) {
	t.Helper()
	h := NewHandlers(mock.New(), NewPlanStore(), NewAuditStore())
	mux := http.NewServeMux()
	h.Register(mux)
	return h, mux
}

func do(t *testing.T, mux *http.ServeMux, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestListConnections(t *testing.T) {
	_, mux := newTestHandlers(t)
	w := do(t, mux, "GET", "/api/v1/connections", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Connections []domain.Connection `json:"connections"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Connections) != 2 {
		t.Errorf("connections = %d, want 2", len(resp.Connections))
	}
}

func TestCreateConnection(t *testing.T) {
	_, mux := newTestHandlers(t)
	body := `{"kind":"vmware","role":"source","endpoint":"vcenter-prod.example.local","name":"vCenter Prod"}`
	w := do(t, mux, "POST", "/api/v1/connections", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	var conn domain.Connection
	if err := json.Unmarshal(w.Body.Bytes(), &conn); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if conn.ID == "" {
		t.Error("connection id empty")
	}
	if conn.Status != domain.ConnConnected {
		t.Errorf("status = %q, want connected", conn.Status)
	}
}

func TestConnectionResponseNeverContainsCredentials(t *testing.T) {
	_, mux := newTestHandlers(t)
	body := `{"kind":"vmware","role":"source","endpoint":"safe.example.local","username":"root","password":"super-secret","secret_ref":"vault/item"}`
	w := do(t, mux, "POST", "/api/v1/connections", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	response := w.Body.String()
	for _, forbidden := range []string{"super-secret", "vault/item", "secret_ref", "password"} {
		if strings.Contains(response, forbidden) {
			t.Fatalf("connection response leaked %q: %s", forbidden, response)
		}
	}
}

func TestProbeExplainsMockMode(t *testing.T) {
	_, mux := newTestHandlers(t)
	w := do(t, mux, "POST", "/api/v1/connections/probe", `{"kind":"vmware","role":"source","endpoint":"esxi.local","username":"root","password":"secret"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "does not contact") {
		t.Fatalf("mock probe message was misleading: %s", w.Body.String())
	}
}

func TestLabConnectionRequiresDirectCredentials(t *testing.T) {
	h := NewHandlersWithMode(mock.NewEmpty(), NewPlanStore(), NewAuditStore(), config.ModeLab)
	mux := http.NewServeMux()
	h.Register(mux)
	w := do(t, mux, "POST", "/api/v1/connections", `{"kind":"vmware","role":"source","endpoint":"esxi.local"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateConnectionRejectsBadKind(t *testing.T) {
	_, mux := newTestHandlers(t)
	body := `{"kind":"nutanix","role":"source","endpoint":"x.local"}`
	w := do(t, mux, "POST", "/api/v1/connections", body)
	if w.Code != http.StatusConflict && w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 409 or 400", w.Code)
	}
}

func TestCreateConnectionRejectsMissingFields(t *testing.T) {
	_, mux := newTestHandlers(t)
	w := do(t, mux, "POST", "/api/v1/connections", `{"name":"x"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestCreateConnectionDuplicate(t *testing.T) {
	_, mux := newTestHandlers(t)
	body := `{"kind":"vmware","role":"source","endpoint":"vc-prod-2.example.local"}`
	// First add succeeds.
	if w := do(t, mux, "POST", "/api/v1/connections", body); w.Code != http.StatusCreated {
		t.Fatalf("first add should be 201, got %d; %s", w.Code, w.Body.String())
	}
	// Second add of the same kind+endpoint must be rejected as a duplicate.
	if w := do(t, mux, "POST", "/api/v1/connections", body); w.Code != http.StatusConflict {
		t.Fatalf("duplicate should be 409, got %d; %s", w.Code, w.Body.String())
	}
}

func TestDeleteConnection(t *testing.T) {
	h, mux := newTestHandlers(t)
	// Add then delete
	add := do(t, mux, "POST", "/api/v1/connections", `{"kind":"proxmox","role":"target","endpoint":"pve-new.local"}`)
	var conn domain.Connection
	_ = json.Unmarshal(add.Body.Bytes(), &conn)

	del := do(t, mux, "DELETE", "/api/v1/connections/"+conn.ID, "")
	if del.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", del.Code)
	}
	if _, ok := h.mock.Connection(conn.ID); ok {
		t.Error("connection still present after delete")
	}
}

func TestTestConnection(t *testing.T) {
	_, mux := newTestHandlers(t)
	w := do(t, mux, "POST", "/api/v1/connections/conn-vmware-lab/test", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestGetInventory(t *testing.T) {
	_, mux := newTestHandlers(t)
	w := do(t, mux, "GET", "/api/v1/connections/conn-vmware-lab/inventory", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestGetInventoryNotFound(t *testing.T) {
	_, mux := newTestHandlers(t)
	w := do(t, mux, "GET", "/api/v1/connections/nope/inventory", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestCreatePlanRejectsMissingFields(t *testing.T) {
	_, mux := newTestHandlers(t)
	w := do(t, mux, "POST", "/api/v1/plans", `{"name":"x"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestCreatePlanAlwaysDraft(t *testing.T) {
	_, mux := newTestHandlers(t)
	body := `{"source_vm_id":"vm-web-01","source_connection_id":"conn-vmware-lab","target_node_id":"node-pve-01","target_connection_id":"conn-proxmox-lab"}`
	w := do(t, mux, "POST", "/api/v1/plans", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	var pl domain.Plan
	if err := json.Unmarshal(w.Body.Bytes(), &pl); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if pl.Status != domain.PlanDraft {
		t.Errorf("status = %q, want draft; a drop must never execute", pl.Status)
	}
	if pl.ID == "" {
		t.Error("plan id empty")
	}
	if pl.Strategy != domain.MigrationStrategyCold {
		t.Errorf("strategy = %q, want cold default", pl.Strategy)
	}
}

func TestCreatePlanAcceptsPVELiveStrategy(t *testing.T) {
	_, mux := newTestHandlers(t)
	body := `{"source_vm_id":"vm-web-01","source_connection_id":"conn-vmware-lab","target_node_id":"node-pve-01","target_connection_id":"conn-proxmox-lab","strategy":"pve-live"}`
	w := do(t, mux, "POST", "/api/v1/plans", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	var pl domain.Plan
	if err := json.Unmarshal(w.Body.Bytes(), &pl); err != nil {
		t.Fatal(err)
	}
	if pl.Strategy != domain.MigrationStrategyPVELive {
		t.Fatalf("strategy = %q", pl.Strategy)
	}
}

func TestCreatePlanRejectsUnimplementedWarmStrategy(t *testing.T) {
	_, mux := newTestHandlers(t)
	body := `{"source_vm_id":"vm-web-01","source_connection_id":"conn-vmware-lab","target_node_id":"node-pve-01","target_connection_id":"conn-proxmox-lab","strategy":"warm"}`
	w := do(t, mux, "POST", "/api/v1/plans", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestApproveRequiresPassingPreflight(t *testing.T) {
	h, mux := newTestHandlers(t)
	NewMigrationHandlers(h, nil, nil, "").RegisterMigration(mux)
	body := `{"source_vm_id":"vm-web-01","source_connection_id":"conn-vmware-lab","target_node_id":"node-pve-01","target_connection_id":"conn-proxmox-lab"}`
	w := do(t, mux, "POST", "/api/v1/plans", body)
	var pl domain.Plan
	_ = json.Unmarshal(w.Body.Bytes(), &pl)
	w = do(t, mux, "POST", "/api/v1/plans/"+pl.ID+"/approve", "")
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
}

func TestPlanLifecycleGetDelete(t *testing.T) {
	h, mux := newTestHandlers(t)
	body := `{"source_vm_id":"vm-web-01","source_connection_id":"conn-vmware-lab","target_node_id":"node-pve-01","target_connection_id":"conn-proxmox-lab"}`
	w := do(t, mux, "POST", "/api/v1/plans", body)
	var pl domain.Plan
	_ = json.Unmarshal(w.Body.Bytes(), &pl)

	wGet := do(t, mux, "GET", "/api/v1/plans/"+pl.ID, "")
	if wGet.Code != http.StatusOK {
		t.Fatalf("get status = %d", wGet.Code)
	}

	wDel := do(t, mux, "DELETE", "/api/v1/plans/"+pl.ID, "")
	if wDel.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", wDel.Code)
	}
	if _, ok := h.plans.Get(pl.ID); ok {
		t.Error("plan still present after delete")
	}
}

func TestPlanCreateWritesAudit(t *testing.T) {
	_, mux := newTestHandlers(t)
	body := `{"source_vm_id":"vm-web-01","source_connection_id":"conn-vmware-lab","target_node_id":"node-pve-01","target_connection_id":"conn-proxmox-lab"}`
	_ = do(t, mux, "POST", "/api/v1/plans", body)
	w := do(t, mux, "GET", "/api/v1/audit", "")
	var resp struct {
		Events []domain.AuditEvent `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal audit: %v", err)
	}
	if len(resp.Events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(resp.Events))
	}
	if resp.Events[0].Action != "plan.create" {
		t.Errorf("audit action = %q", resp.Events[0].Action)
	}
}
