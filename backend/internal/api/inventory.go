package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/drishti/hypershift/internal/auth"
	"github.com/drishti/hypershift/internal/domain"
)

// PlanBackend is implemented by the durable PostgreSQL repository.
type PlanBackend interface {
	ListPlans(context.Context) ([]domain.Plan, error)
	GetPlan(context.Context, string) (domain.Plan, bool, error)
	PutPlan(context.Context, domain.Plan) error
	DeletePlan(context.Context, string) error
}

type planApprovalBackend interface {
	ApprovePlan(context.Context, string, bool, domain.AuditEvent) (domain.Plan, bool, error)
}

// PlanStore provides a concurrency-safe memory implementation in mock mode and
// delegates to PostgreSQL in lab, live, and production modes.
type PlanStore struct {
	mu         sync.RWMutex
	approvalMu sync.Mutex
	plans      map[string]domain.Plan
	backend    PlanBackend
}

func NewPlanStore() *PlanStore {
	return &PlanStore{plans: make(map[string]domain.Plan)}
}

func NewPersistentPlanStore(backend PlanBackend) *PlanStore {
	return &PlanStore{backend: backend}
}

func clonePlan(plan domain.Plan) domain.Plan {
	plan.StorageMaps = append([]domain.StorageMap(nil), plan.StorageMaps...)
	plan.NetworkMaps = append([]domain.NetworkMap(nil), plan.NetworkMaps...)
	if plan.TargetVMID != nil {
		vmid := *plan.TargetVMID
		plan.TargetVMID = &vmid
	}
	return plan
}

// All returns a snapshot of all plans ordered by creation time.
func (p *PlanStore) All() []domain.Plan {
	out, _ := p.AllContext(context.Background())
	return out
}

func (p *PlanStore) AllContext(ctx context.Context) ([]domain.Plan, error) {
	if p.backend != nil {
		return p.backend.ListPlans(ctx)
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]domain.Plan, 0, len(p.plans))
	for _, pl := range p.plans {
		out = append(out, clonePlan(pl))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (p *PlanStore) Get(id string) (domain.Plan, bool) {
	pl, ok, _ := p.GetContext(context.Background(), id)
	return pl, ok
}

func (p *PlanStore) GetContext(ctx context.Context, id string) (domain.Plan, bool, error) {
	if p.backend != nil {
		return p.backend.GetPlan(ctx, id)
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	pl, ok := p.plans[id]
	return clonePlan(pl), ok, nil
}

func (p *PlanStore) Put(pl domain.Plan) { _ = p.PutContext(context.Background(), pl) }

func (p *PlanStore) PutContext(ctx context.Context, pl domain.Plan) error {
	if p.backend != nil {
		return p.backend.PutPlan(ctx, pl)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.plans[pl.ID] = clonePlan(pl)
	return nil
}

func (p *PlanStore) Delete(id string) { _ = p.DeleteContext(context.Background(), id) }

func (p *PlanStore) DeleteContext(ctx context.Context, id string) error {
	if p.backend != nil {
		return p.backend.DeletePlan(ctx, id)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.plans, id)
	return nil
}

// ApproveContext atomically records the plan approval and its audit event when
// the backend supports transactions. The mock implementation serializes the
// same operation under a process-local lock.
func (p *PlanStore) ApproveContext(ctx context.Context, id string, powerOff bool, event domain.AuditEvent, audit *AuditStore) (domain.Plan, bool, error) {
	if backend, ok := p.backend.(planApprovalBackend); ok {
		return backend.ApprovePlan(ctx, id, powerOff, event)
	}
	p.approvalMu.Lock()
	defer p.approvalMu.Unlock()
	plan, found, err := p.GetContext(ctx, id)
	if err != nil || !found {
		return plan, found, err
	}
	if plan.Status != domain.PlanPreflight || !plan.PreflightPassed {
		return plan, true, fmt.Errorf("plan requires a passing preflight before approval")
	}
	plan.Status = domain.PlanApproved
	plan.SourcePowerOffApproved = powerOff
	plan.UpdatedAt = event.Timestamp
	if err := p.PutContext(ctx, plan); err != nil {
		return domain.Plan{}, true, err
	}
	if err := audit.AppendContext(ctx, event); err != nil {
		return domain.Plan{}, true, err
	}
	return plan, true, nil
}

// listPlans returns all draft/approved plans.
func (h *Handlers) listPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.plans.AllContext(r.Context())
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "plan store unavailable", Code: "store_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plans": plans})
}

// createPlanInput is the body accepted when a drop opens a plan. It intentionally
// omits execution fields; status is always forced to "draft".
type createPlanInput struct {
	Name         string              `json:"name"`
	SourceVMID   string              `json:"source_vm_id"`
	SourceConnID string              `json:"source_connection_id"`
	TargetNodeID string              `json:"target_node_id"`
	TargetConnID string              `json:"target_connection_id"`
	TargetVMName string              `json:"target_vm_name"`
	CPU          int                 `json:"cpu"`
	MemoryMB     int64               `json:"memory_mb"`
	Firmware     string              `json:"firmware"`
	DiskFormat   string              `json:"disk_format"`
	Strategy     string              `json:"strategy"`
	StorageMaps  []domain.StorageMap `json:"storage_maps"`
	NetworkMaps  []domain.NetworkMap `json:"network_maps"`
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
		StorageMaps:  in.StorageMaps,
		NetworkMaps:  in.NetworkMaps,
		DiskFormat:   domain.DiskFormat(in.DiskFormat),
		Strategy:     domain.MigrationStrategy(in.Strategy),
		Status:       domain.PlanDraft,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		CreatedBy:    auth.Actor(r.Context()),
	}
	if pl.Strategy == "" {
		pl.Strategy = domain.MigrationStrategyCold
	}
	if pl.Strategy != domain.MigrationStrategyCold && pl.Strategy != domain.MigrationStrategyPVELive {
		writeErr(w, http.StatusBadRequest, errorBody{Error: "strategy must be cold or pve-live; warm migration is not implemented yet", Code: "bad_request"})
		return
	}
	if pl.DiskFormat != domain.DiskRaw && pl.DiskFormat != domain.DiskQCOW2 {
		pl.DiskFormat = domain.DiskRaw
	}
	for _, sm := range pl.StorageMaps {
		if sm.SourceDiskID == "" || sm.TargetStorageID == "" {
			writeErr(w, http.StatusBadRequest, errorBody{Error: "every storage mapping requires source_disk_id and target_storage_id", Code: "bad_request"})
			return
		}
	}
	for _, nm := range pl.NetworkMaps {
		if nm.SourceNICID == "" || nm.TargetBridge == "" {
			writeErr(w, http.StatusBadRequest, errorBody{Error: "every network mapping requires source_nic_id and target_bridge", Code: "bad_request"})
			return
		}
	}
	if err := h.plans.PutContext(r.Context(), pl); err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "plan store unavailable", Code: "store_unavailable"})
		return
	}
	if err := h.audit.AppendContext(r.Context(), domain.AuditEvent{
		ID:        "evt-" + newID(),
		Timestamp: time.Now().UTC(),
		Actor:     auth.Actor(r.Context()),
		Action:    "plan.create",
		Target:    pl.ID,
		Result:    "draft",
		Detail:    "Migration plan created from drag-and-drop; no migration executed.",
	}); err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "audit store unavailable", Code: "store_unavailable"})
		return
	}
	writeJSON(w, http.StatusCreated, pl)
}

// getPlan returns a single plan.
func (h *Handlers) getPlan(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	pl, ok, err := h.plans.GetContext(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "plan store unavailable", Code: "store_unavailable"})
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "plan not found", Code: "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, pl)
}

// deletePlan archives (does not hard-delete) a draft plan.
func (h *Handlers) deletePlan(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	pl, ok, err := h.plans.GetContext(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "plan store unavailable", Code: "store_unavailable"})
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "plan not found", Code: "not_found"})
		return
	}
	if pl.Status != domain.PlanDraft {
		writeErr(w, http.StatusConflict, errorBody{Error: "only draft plans can be removed", Code: "conflict"})
		return
	}
	if err := h.plans.DeleteContext(r.Context(), id); err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "plan store unavailable", Code: "store_unavailable"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
