package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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