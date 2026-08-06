package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/drishti/hypershift/internal/domain"
)

type AuditBackend interface {
	AppendAudit(context.Context, domain.AuditEvent) error
	ListAudit(context.Context) ([]domain.AuditEvent, error)
}

// AuditStore is append-only and concurrency safe. Mock mode stores events in
// memory; real modes delegate to PostgreSQL.
type AuditStore struct {
	mu      sync.RWMutex
	events  []domain.AuditEvent
	backend AuditBackend
}

func NewAuditStore() *AuditStore { return &AuditStore{} }

func NewPersistentAuditStore(backend AuditBackend) *AuditStore {
	return &AuditStore{backend: backend}
}

func (a *AuditStore) Append(e domain.AuditEvent) { _ = a.AppendContext(context.Background(), e) }

func (a *AuditStore) AppendContext(ctx context.Context, e domain.AuditEvent) error {
	if a.backend != nil {
		return a.backend.AppendAudit(ctx, e)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
	return nil
}

func (a *AuditStore) All() []domain.AuditEvent {
	events, _ := a.AllContext(context.Background())
	return events
}

func (a *AuditStore) AllContext(ctx context.Context) ([]domain.AuditEvent, error) {
	if a.backend != nil {
		return a.backend.ListAudit(ctx)
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]domain.AuditEvent, len(a.events))
	copy(out, a.events)
	return out, nil
}

func (h *Handlers) listAudit(w http.ResponseWriter, r *http.Request) {
	events, err := h.audit.AllContext(r.Context())
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "audit store unavailable", Code: "store_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

// newID returns a compact time-based identifier suitable for dev IDs. It is
// not a security token.
func newID() string {
	return time.Now().UTC().Format("20060102T150405.000000000")
}

// nowUTC returns the current UTC time.
func nowUTC() time.Time { return time.Now().UTC() }
