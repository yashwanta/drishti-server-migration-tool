package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/drishti/hypershift/internal/auth"
	"github.com/drishti/hypershift/internal/config"
	"github.com/drishti/hypershift/internal/credential"
	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/mock"
	vmwareplatform "github.com/drishti/hypershift/internal/platform/vmware"
)

// Handlers holds dependencies shared by all API routes.
type Handlers struct {
	mock  *mock.Provider
	plans *PlanStore
	audit *AuditStore
	probe ConnectionProbe
	mode  config.RunMode
	creds *credential.Memory
}

type ConnectionProbe interface {
	Test(context.Context, domain.Connection) (string, error)
	Inventory(context.Context, domain.Connection) (domain.InventoryRoot, error)
}

func NewHandlers(p *mock.Provider, plans *PlanStore, audit *AuditStore) *Handlers {
	return NewHandlersWithMode(p, plans, audit, config.ModeMock)
}

// NewHandlersWithMode enables real read-only connectors outside mock mode.
func NewHandlersWithMode(p *mock.Provider, plans *PlanStore, audit *AuditStore, mode config.RunMode) *Handlers {
	return &Handlers{mock: p, plans: plans, audit: audit, mode: mode, creds: credential.NewMemory()}
}

func NewHandlersWithProbe(p *mock.Provider, plans *PlanStore, audit *AuditStore, probe ConnectionProbe) *Handlers {
	return NewHandlersWithModeAndProbe(p, plans, audit, config.ModeLab, probe)
}

func NewHandlersWithModeAndProbe(p *mock.Provider, plans *PlanStore, audit *AuditStore, mode config.RunMode, probe ConnectionProbe) *Handlers {
	return NewHandlersWithRuntime(p, plans, audit, mode, probe, credential.NewMemory())
}

// NewHandlersWithRuntime injects the process-local credential store shared by
// read-only connection handlers and the runtime adapter factory.
func NewHandlersWithRuntime(p *mock.Provider, plans *PlanStore, audit *AuditStore, mode config.RunMode, probe ConnectionProbe, creds *credential.Memory) *Handlers {
	if creds == nil {
		creds = credential.NewMemory()
	}
	return &Handlers{mock: p, plans: plans, audit: audit, probe: probe, mode: mode, creds: creds}
}

// Register attaches all API routes to the given mux.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/connections", h.listConnections)
	mux.HandleFunc("POST /api/v1/connections", h.createConnection)
	mux.HandleFunc("POST /api/v1/connections/probe", h.probeConnection)
	mux.HandleFunc("GET /api/v1/connections/{id}", h.getConnection)
	mux.HandleFunc("DELETE /api/v1/connections/{id}", h.deleteConnection)
	mux.HandleFunc("GET /api/v1/connections/{id}/inventory", h.getInventory)
	mux.HandleFunc("POST /api/v1/connections/{id}/test", h.testConnection)
	mux.HandleFunc("GET /api/v1/runtime", h.getRuntime)

	mux.HandleFunc("GET /api/v1/plans", h.listPlans)
	mux.HandleFunc("POST /api/v1/plans", h.createPlan)
	mux.HandleFunc("GET /api/v1/plans/{id}", h.getPlan)
	mux.HandleFunc("DELETE /api/v1/plans/{id}", h.deletePlan)

	mux.HandleFunc("GET /api/v1/audit", h.listAudit)
}

// createConnectionInput accepts either a secret reference or ephemeral direct
// credentials. Password is never copied into a domain model or API response.
type createConnectionInput struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Role        string `json:"role"`
	Endpoint    string `json:"endpoint"`
	InsecureTLS bool   `json:"insecure_tls"`
	SecretRef   string `json:"secret_ref"`
	Username    string `json:"username"`
	Password    string `json:"password"`
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
	if h.mode != config.ModeMock {
		switch domain.PlatformKind(in.Kind) {
		case domain.PlatformVMware:
			if err := validateDirectCredentials(in); err != "" {
				writeErr(w, http.StatusBadRequest, errorBody{Error: "credentials required", Code: "bad_request", Detail: err})
				return
			}
			if err := h.probeVMware(r.Context(), in); err != nil {
				writeErr(w, http.StatusBadGateway, errorBody{Error: "VMware connection failed", Code: "connection_failed", Detail: safePlatformError(err)})
				return
			}
		case domain.PlatformProxmox:
			if h.probe == nil {
				writeErr(w, http.StatusBadRequest, errorBody{Error: "live connector unavailable", Code: "not_implemented", Detail: "The Proxmox connector is not configured."})
				return
			}
			if in.SecretRef == "" {
				writeErr(w, http.StatusBadRequest, errorBody{Error: "secret reference required", Code: "bad_request", Detail: "Proxmox currently requires an API-token secret reference configured in the backend environment."})
				return
			}
			candidate := domain.Connection{Kind: domain.PlatformProxmox, Endpoint: in.Endpoint, InsecureTLS: in.InsecureTLS, SecretRef: in.SecretRef}
			if _, err := h.probe.Test(r.Context(), candidate); err != nil {
				writeErr(w, http.StatusBadGateway, errorBody{Error: "Proxmox connection failed", Code: "connection_failed", Detail: safePlatformError(err)})
				return
			}
		default:
			writeErr(w, http.StatusBadRequest, errorBody{Error: "live connector unavailable", Code: "not_implemented", Detail: "Only VMware and Proxmox live connectors are available."})
			return
		}
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
	if h.mode != config.ModeMock && conn.Kind == domain.PlatformVMware {
		h.creds.Put(conn.ID, credential.Value{Username: in.Username, Password: in.Password})
	}
	if err := h.audit.AppendContext(r.Context(), domain.AuditEvent{
		ID:        "evt-" + newID(),
		Timestamp: nowUTC(),
		Actor:     auth.Actor(r.Context()),
		Action:    "connection.create",
		Target:    conn.ID,
		Result:    "connected",
		Detail:    "Platform connection registered: " + string(conn.Kind) + " " + string(conn.Role) + " " + sanitizeLog(conn.Endpoint),
	}); err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "audit store unavailable", Code: "store_unavailable"})
		return
	}
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
	h.creds.Delete(id)
	if err := h.audit.AppendContext(r.Context(), domain.AuditEvent{
		ID:        "evt-" + newID(),
		Timestamp: nowUTC(),
		Actor:     auth.Actor(r.Context()),
		Action:    "connection.delete",
		Target:    id,
		Result:    "deleted",
		Detail:    "Platform connection removed from registry.",
	}); err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "audit store unavailable", Code: "store_unavailable"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getInventory returns normalized inventory for a connection.
func (h *Handlers) getInventory(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	if h.mode != config.ModeMock {
		conn, ok := h.mock.Connection(id)
		if !ok {
			writeErr(w, http.StatusNotFound, errorBody{Error: "connection not found", Code: "not_found"})
			return
		}
		var inv domain.InventoryRoot
		var err error
		switch conn.Kind {
		case domain.PlatformVMware:
			creds, exists := h.creds.Get(id)
			if !exists {
				writeErr(w, http.StatusUnauthorized, errorBody{Error: "credentials unavailable", Code: "credentials_unavailable", Detail: "Re-add the connection; direct credentials are held only until the backend restarts."})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
			defer cancel()
			inv, err = vmwareplatform.New(conn.Endpoint, creds.Username, creds.Password, conn.InsecureTLS).Inventory(ctx, conn.ID)
		case domain.PlatformProxmox:
			if h.probe == nil {
				writeErr(w, http.StatusBadRequest, errorBody{Error: "Proxmox connector unavailable", Code: "not_implemented"})
				return
			}
			inv, err = h.probe.Inventory(r.Context(), conn)
		default:
			writeErr(w, http.StatusBadRequest, errorBody{Error: "live inventory unavailable", Code: "not_implemented"})
			return
		}
		if err != nil {
			writeErr(w, http.StatusBadGateway, errorBody{Error: "real inventory request failed", Code: "inventory_failed", Detail: safePlatformError(err)})
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
// reports that no platform was contacted; live modes use the configured connector.
func (h *Handlers) testConnection(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	c, ok := h.mock.Connection(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "connection not found", Code: "not_found"})
		return
	}
	if h.mode != config.ModeMock {
		var message string
		var err error
		switch c.Kind {
		case domain.PlatformVMware:
			creds, exists := h.creds.Get(id)
			if !exists {
				writeErr(w, http.StatusUnauthorized, errorBody{Error: "credentials unavailable", Code: "credentials_unavailable"})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			err = vmwareplatform.New(c.Endpoint, creds.Username, creds.Password, c.InsecureTLS).Test(ctx)
			message = "Authenticated to VMware successfully using a read-only API session."
		case domain.PlatformProxmox:
			if h.probe == nil {
				writeErr(w, http.StatusBadRequest, errorBody{Error: "Proxmox connector unavailable", Code: "not_implemented"})
				return
			}
			message, err = h.probe.Test(r.Context(), c)
		default:
			err = fmt.Errorf("live connectivity is unavailable for platform %q", c.Kind)
		}
		if err != nil {
			writeErr(w, http.StatusBadGateway, errorBody{Error: "real connectivity probe failed", Code: "connection_failed", Detail: safePlatformError(err)})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"connection_id": c.ID, "status": "connected", "message": message})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"connection_id": c.ID,
		"status":        "connected",
		"message":       "Mock connectivity probe succeeded. No real platform was contacted.",
	})
}

// probeConnection tests unsaved credentials without registering or persisting them.
func (h *Handlers) probeConnection(w http.ResponseWriter, r *http.Request) {
	var in createConnectionInput
	if err := decodeJSON(r, &in, 1<<20); err != nil {
		writeErr(w, http.StatusBadRequest, errorBody{Error: "invalid request body", Detail: err.Error(), Code: "bad_request"})
		return
	}
	if h.mode == config.ModeMock {
		writeJSON(w, http.StatusOK, map[string]any{"status": "mock", "message": "Mock mode does not contact the supplied endpoint. Switch the backend to lab mode for real VMware discovery."})
		return
	}
	var message string
	switch domain.PlatformKind(in.Kind) {
	case domain.PlatformVMware:
		if err := validateDirectCredentials(in); err != "" {
			writeErr(w, http.StatusBadRequest, errorBody{Error: "credentials required", Code: "bad_request", Detail: err})
			return
		}
		if err := h.probeVMware(r.Context(), in); err != nil {
			writeErr(w, http.StatusBadGateway, errorBody{Error: "VMware connection failed", Code: "connection_failed", Detail: safePlatformError(err)})
			return
		}
		message = "Authenticated to VMware successfully using a read-only API session."
	case domain.PlatformProxmox:
		if h.probe == nil || in.SecretRef == "" {
			writeErr(w, http.StatusBadRequest, errorBody{Error: "Proxmox API-token secret reference required", Code: "bad_request"})
			return
		}
		candidate := domain.Connection{Kind: domain.PlatformProxmox, Endpoint: in.Endpoint, InsecureTLS: in.InsecureTLS, SecretRef: in.SecretRef}
		var err error
		message, err = h.probe.Test(r.Context(), candidate)
		if err != nil {
			writeErr(w, http.StatusBadGateway, errorBody{Error: "Proxmox connection failed", Code: "connection_failed", Detail: safePlatformError(err)})
			return
		}
	default:
		writeErr(w, http.StatusBadRequest, errorBody{Error: "live connector unavailable", Code: "not_implemented"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "connected", "message": message})
}

func (h *Handlers) getRuntime(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"mode": h.mode})
}

func (h *Handlers) probeVMware(parent context.Context, in createConnectionInput) error {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	return vmwareplatform.New(in.Endpoint, in.Username, in.Password, in.InsecureTLS).Test(ctx)
}

func validateDirectCredentials(in createConnectionInput) string {
	if in.Username == "" || in.Password == "" {
		if in.SecretRef != "" {
			return "A secret-reference resolver is not configured yet; choose Username & password for this lab session."
		}
		return "Username and password are required for a live VMware connection."
	}
	return ""
}

// safePlatformError removes any accidental credential-bearing URL before a
// connector error is returned to the browser.
func safePlatformError(err error) string {
	return sanitizeLog(err.Error())
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
