package api

import (
	"net/http"
	"time"

	"github.com/drishti/hypershift/internal/domain"
)

// AuditStore is a thread-safe in-memory audit log used in Phase 0/1.
type AuditStore struct {
	events []domain.AuditEvent
}

func NewAuditStore() *AuditStore { return &AuditStore{} }

func (a *AuditStore) Append(e domain.AuditEvent) { a.events = append(a.events, e) }

func (a *AuditStore) All() []domain.AuditEvent {
	out := make([]domain.AuditEvent, len(a.events))
	copy(out, a.events)
	return out
}

func (h *Handlers) listAudit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"events": h.audit.All()})
}

// newID returns a compact time-based identifier suitable for dev IDs. It is
// not a security token.
func newID() string {
	return time.Now().UTC().Format("20060102T150405.000000000")
}
