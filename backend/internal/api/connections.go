package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/mock"
)

// Handlers holds dependencies shared by all API routes.
type Handlers struct {
	mock  *mock.Provider
	plans *PlanStore
	audit *AuditStore
	probe ConnectionProbe
}

type ConnectionProbe interface {
	Test(context.Context, domain.Connection) (string, error)
	Inventory(context.Context, domain.Connection) (domain.InventoryRoot, error)
}

func NewHandlers(p *mock.Provider, plans *PlanStore, audit *AuditStore) *Handlers {
	return &Handlers{mock: p, plans: plans, audit: audit}
}

func NewHandlersWithProbe(p *mock.Provider, plans *PlanStore, audit *AuditStore, probe ConnectionProbe) *Handlers {
	return &Handlers{mock: p, plans: plans, audit: audit, probe: probe}
}

// Register attaches all API routes to the given mux.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/connections", h.listConnections)
	mux.HandleFunc("POST /api/v1/connections", h.createConnection)
	mux.HandleFunc("GET /api/v1/connections/{id}", h.getConnection)
	mux.HandleFunc("DELETE /api/v1/connections/{id}", h.deleteConnection)
	mux.HandleFunc("GET /api/v1/connections/{id}/inventory", h.getInventory)
	mux.HandleFunc("POST /api/v1/connections/{id}/test", h.testConnection)

	mux.HandleFunc("GET /api/v1/plans", h.listPlans)
	mux.HandleFunc("POST /api/v1/plans", h.createPlan)
	mux.HandleFunc("GET /api/v1/plans/{id}", h.getPlan)
	mux.HandleFunc("DELETE /api/v1/plans/{id}", h.deletePlan)

	mux.HandleFunc("GET /api/v1/audit", h.listAudit)
}

// createConnectionInput is the body for adding a platform connection. Secrets
// are passed by reference only and never logged.
type createConnectionInput struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Role        string `json:"role"`
	Endpoint    string `json:"endpoint"`
	InsecureTLS bool   `json:"insecure_tls"`
	SecretRef   string `json:"secret_ref"`
}

// listConnections returns all configured platform connections.
func (h *Handlers) listConnections(w http.ResponseWriter, r *http.Request) {
	conns := h.mock.Connections()
	writeJSON(w, http.StatusOK, map[string]any{"connections": conns})
}

// createConnection adds a new source or target connection.
func (h *Handlers) createConnection(w http.ResponseWriter, r *http.Request) {
	var in createConnectionInput
	if err := decodeJSON(r, &in, 1<<20); err != nil {
		writeErr(w, http.StatusBadRequest, errorBody{Error: "invalid request body", Detail: err.Error(), Code: "bad_request"})
		return
	}
	if in.Kind == "" || in.Role == "" || in.Endpoint == "" {
		writeErr(w, http.StatusBadRequest, errorBody{
			Error:  "kind, role, and endpoint are required",
			Code:   "bad_request",
			Detail: "kind: vmware|proxmox|hyperv, role: source|target, endpoint: host or URL",
		})
		return
	}
	conn, err := h.mock.Add(domain.Connection{
		Name:        in.Name,
		Kind:        domain.PlatformKind(in.Kind),
		Role:        domain.Role(in.Role),
		Endpoint:    in.Endpoint,
		InsecureTLS: in.InsecureTLS,
		SecretRef:   in.SecretRef,
	})
	if err != nil {
		writeErr(w, http.StatusConflict, errorBody{Error: err.Error(), Code: "conflict"})
		return
	}
	h.audit.Append(domain.AuditEvent{
		ID:        "evt-" + newID(),
		Timestamp: nowUTC(),
		Actor:     "operator",
		Action:    "connection.create",
		Target:    conn.ID,
		Result:    "connected",
		Detail:    "Platform connection registered: " + string(conn.Kind) + " " + string(conn.Role) + " " + sanitizeLog(conn.Endpoint),
	})
	writeJSON(w, http.StatusCreated, conn)
}

// getConnection returns a single connection by id.
func (h *Handlers) getConnection(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	c, ok := h.mock.Connection(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "connection not found", Code: "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// deleteConnection removes a connection.
func (h *Handlers) deleteConnection(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	if !h.mock.Remove(id) {
		writeErr(w, http.StatusNotFound, errorBody{Error: "connection not found", Code: "not_found"})
		return
	}
	h.audit.Append(domain.AuditEvent{
		ID:        "evt-" + newID(),
		Timestamp: nowUTC(),
		Actor:     "operator",
		Action:    "connection.delete",
		Target:    id,
		Result:    "deleted",
		Detail:    "Platform connection removed from registry.",
	})
	w.WriteHeader(http.StatusNoContent)
}

// getInventory returns normalized inventory for a connection.
func (h *Handlers) getInventory(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	if h.probe != nil {
		conn, exists := h.mock.Connection(id)
		if !exists {
			writeErr(w, http.StatusNotFound, errorBody{Error: "connection not found", Code: "not_found"})
			return
		}
		inv, err := h.probe.Inventory(r.Context(), conn)
		if err != nil {
			writeErr(w, http.StatusBadGateway, errorBody{Error: "real inventory request failed", Detail: err.Error(), Code: "inventory_failed"})
			return
		}
		writeJSON(w, http.StatusOK, inv)
		return
	}
	inv, ok := h.mock.Inventory(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "connection not found", Code: "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

// testConnection performs a read-only connectivity probe. Mock mode clearly
// reports that no platform was contacted; lab mode uses the configured probe.
func (h *Handlers) testConnection(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	c, ok := h.mock.Connection(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "connection not found", Code: "not_found"})
		return
	}
	message := "Mock connectivity probe succeeded. No real platform was contacted."
	if h.probe != nil {
		var err error
		message, err = h.probe.Test(r.Context(), c)
		if err != nil {
			writeErr(w, http.StatusBadGateway, errorBody{Error: "real connectivity probe failed", Detail: err.Error(), Code: "connection_failed"})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"connection_id": c.ID,
		"status":        "connected",
		"message":       message,
	})
}

// sanitizeLog strips credential-shaped fragments from log-safe endpoint text.
func sanitizeLog(endpoint string) string {
	if at := strings.Index(endpoint, "@"); at >= 0 {
		if scheme := strings.Index(endpoint, "://"); scheme >= 0 && scheme < at {
			return endpoint[:scheme+3] + "***@" + endpoint[at+1:]
		}
	}
	return endpoint
}
