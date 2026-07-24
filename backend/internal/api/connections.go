package api

import (
	"net/http"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/mock"
)

// Handlers holds dependencies shared by all API routes.
type Handlers struct {
	mock    *mock.Provider
	plans   *PlanStore
	audit   *AuditStore
}

func NewHandlers(p *mock.Provider, plans *PlanStore, audit *AuditStore) *Handlers {
	return &Handlers{mock: p, plans: plans, audit: audit}
}

// Register attaches all API routes to the given mux.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/connections", h.listConnections)
	mux.HandleFunc("GET /api/v1/connections/{id}", h.getConnection)
	mux.HandleFunc("GET /api/v1/connections/{id}/inventory", h.getInventory)

	mux.HandleFunc("GET /api/v1/plans", h.listPlans)
	mux.HandleFunc("POST /api/v1/plans", h.createPlan)
	mux.HandleFunc("GET /api/v1/plans/{id}", h.getPlan)
	mux.HandleFunc("DELETE /api/v1/plans/{id}", h.deletePlan)

	mux.HandleFunc("GET /api/v1/audit", h.listAudit)
}

// listConnections returns all configured platform connections.
func (h *Handlers) listConnections(w http.ResponseWriter, r *http.Request) {
	conns := h.mock.Connections()
	out := make([]domain.Connection, len(conns))
	copy(out, conns)
	writeJSON(w, http.StatusOK, map[string]any{"connections": out})
}

// getConnection returns a single connection by id.
func (h *Handlers) getConnection(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	for _, c := range h.mock.Connections() {
		if c.ID == id {
			writeJSON(w, http.StatusOK, c)
			return
		}
	}
	writeErr(w, http.StatusNotFound, errorBody{Error: "connection not found", Code: "not_found"})
}

// getInventory returns normalized inventory for a connection.
func (h *Handlers) getInventory(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	inv, ok := h.mock.Inventory(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "connection not found", Code: "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, inv)
}
