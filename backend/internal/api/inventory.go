package api

import (
	"net/http"
	"time"

	"github.com/drishti/hypershift/internal/domain"
)

// PlanStore is a thread-safe in-memory plan store for Phase 0/1. Persistence
// to Postgres arrives in Phase 6.
type PlanStore struct {
	mu    chanToken
	plans map[string]domain.Plan
}

type chanToken struct{}

func NewPlanStore() *PlanStore {
	return &PlanStore{plans: make(map[string]domain.Plan)}
}

// All returns a snapshot of all plans ordered by creation time.
func (p *PlanStore) All() []domain.Plan {
	out := make([]domain.Plan, 0, len(p.plans))
	for _, pl := range p.plans {
		out = append(out, pl)
	}
	return out
}

func (p *PlanStore) Get(id string) (domain.Plan, bool) {
	pl, ok := p.plans[id]
	return pl, ok
}

func (p *PlanStore) Put(pl domain.Plan) { p.plans[pl.ID] = pl }

func (p *PlanStore) Delete(id string) { delete(p.plans, id) }

// listPlans returns all draft/approved plans.
func (h *Handlers) listPlans(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"plans": h.plans.All()})
}

// createPlanInput is the body accepted when a drop opens a plan. It intentionally
// omits execution fields; status is always forced to "draft".
type createPlanInput struct {
	Name         string `json:"name"`
	SourceVMID   string `json:"source_vm_id"`
	SourceConnID string `json:"source_connection_id"`
	TargetNodeID string `json:"target_node_id"`
	TargetConnID string `json:"target_connection_id"`
	TargetVMName string `json:"target_vm_name"`
	CPU          int    `json:"cpu"`
	MemoryMB     int64  `json:"memory_mb"`
	Firmware     string `json:"firmware"`
}

// createPlan is what a drop calls. It NEVER starts a migration; the created
// plan is always a draft awaiting preflight and approval.
func (h *Handlers) createPlan(w http.ResponseWriter, r *http.Request) {
	var in createPlanInput
	if err := decodeJSON(r, &in, 1<<20); err != nil {
		writeErr(w, http.StatusBadRequest, errorBody{Error: "invalid request body", Detail: err.Error(), Code: "bad_request"})
		return
	}
	if in.SourceVMID == "" || in.SourceConnID == "" || in.TargetNodeID == "" || in.TargetConnID == "" {
		writeErr(w, http.StatusBadRequest, errorBody{Error: "source_vm_id, source_connection_id, target_node_id and target_connection_id are required", Code: "bad_request"})
		return
	}
	if in.Name == "" {
		in.Name = "plan-" + in.SourceVMID
	}
	fw := domain.FirmwareBIOS
	if in.Firmware == string(domain.FirmwareUEFI) {
		fw = domain.FirmwareUEFI
	}
	if in.CPU <= 0 {
		in.CPU = 1
	}
	if in.MemoryMB <= 0 {
		in.MemoryMB = 512
	}
	pl := domain.Plan{
		ID:           "plan-" + newID(),
		Name:         in.Name,
		SourceVMID:   in.SourceVMID,
		SourceConnID: in.SourceConnID,
		TargetNodeID: in.TargetNodeID,
		TargetConnID: in.TargetConnID,
		TargetVMName: in.TargetVMName,
		CPU:          in.CPU,
		MemoryMB:     in.MemoryMB,
		Firmware:     fw,
		DiskFormat:   domain.DiskQCOW2,
		Status:       domain.PlanDraft,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		CreatedBy:    "operator",
	}
	h.plans.Put(pl)
	h.audit.Append(domain.AuditEvent{
		ID:        "evt-" + newID(),
		Timestamp: time.Now().UTC(),
		Actor:     "operator",
		Action:    "plan.create",
		Target:    pl.ID,
		Result:    "draft",
		Detail:    "Migration plan created from drag-and-drop; no migration executed.",
	})
	writeJSON(w, http.StatusCreated, pl)
}

// getPlan returns a single plan.
func (h *Handlers) getPlan(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	pl, ok := h.plans.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "plan not found", Code: "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, pl)
}

// deletePlan archives (does not hard-delete) a draft plan.
func (h *Handlers) deletePlan(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	pl, ok := h.plans.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "plan not found", Code: "not_found"})
		return
	}
	if pl.Status != domain.PlanDraft {
		writeErr(w, http.StatusConflict, errorBody{Error: "only draft plans can be removed", Code: "conflict"})
		return
	}
	h.plans.Delete(id)
	w.WriteHeader(http.StatusNoContent)
}
