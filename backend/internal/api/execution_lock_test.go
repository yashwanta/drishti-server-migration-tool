package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/mock"
	"github.com/drishti/hypershift/internal/platform"
	"github.com/drishti/hypershift/internal/platform/proxmox"
)

type panicAdapterFactory struct{}

func (panicAdapterFactory) Source(domain.Connection) (platform.SourceAdapter, error) {
	panic("disabled HTTP route reached source adapter selection")
}

func (panicAdapterFactory) Target(domain.Connection) (platform.TargetAdapter, error) {
	panic("disabled HTTP route reached target adapter selection")
}

func TestExecutionLockBlocksExecuteCutoverAndRollbackBeforeAdapterUse(t *testing.T) {
	h := NewHandlers(mock.New(), NewPlanStore(), NewAuditStore())
	mh := NewMigrationHandlers(h, nil, panicAdapterFactory{}, t.TempDir())
	// The constructor is fail-closed, and the legacy enabling hook must not
	// make execution reachable even for an authenticated caller.
	mh.EnableLabRemoteMigration(proxmox.NewProbe(), true)
	mux := http.NewServeMux()
	mh.RegisterMigration(mux)

	for _, path := range []string{
		"/api/v1/plans/plan-1/execute",
		"/api/v1/jobs/job-1/cutover",
		"/api/v1/jobs/job-1/rollback",
	} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, path, nil)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != http.StatusNotImplemented {
				t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
			}
			if got := response.Body.String(); !strings.Contains(got, "execution_disabled") {
				t.Fatalf("response does not report fail-closed execution: %s", got)
			}
		})
	}
}

var _ platform.AdapterFactory = panicAdapterFactory{}
