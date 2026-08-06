package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/mock"
)

func TestApprovePlanRecordsSeparateSourcePowerOffApproval(t *testing.T) {
	plans := NewPlanStore()
	plan := domain.Plan{ID: "plan-approval", Status: domain.PlanPreflight, PreflightPassed: true}
	plans.Put(plan)
	h := NewHandlers(mock.New(), plans, NewAuditStore())
	mh := NewMigrationHandlers(h, nil, nil, "")
	mux := http.NewServeMux()
	mh.RegisterMigration(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plans/plan-approval/approve", bytes.NewBufferString(`{"approve_source_power_off":true}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", res.Code, res.Body.String())
	}
	updated, ok := plans.Get(plan.ID)
	if !ok || !updated.SourcePowerOffApproved || updated.Status != domain.PlanApproved {
		t.Fatalf("approval not recorded: %#v", updated)
	}
}

func TestConcurrentApprovalRecordsExactlyOneApproval(t *testing.T) {
	plans := NewPlanStore()
	audit := NewAuditStore()
	plan := domain.Plan{ID: "plan-concurrent-approval", Status: domain.PlanPreflight, PreflightPassed: true}
	plans.Put(plan)
	h := NewHandlers(mock.New(), plans, audit)
	mux := http.NewServeMux()
	NewMigrationHandlers(h, nil, nil, "").RegisterMigration(mux)

	var successes atomic.Int32
	var workers sync.WaitGroup
	for i := 0; i < 32; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/plans/plan-concurrent-approval/approve", bytes.NewBufferString(`{"approve_source_power_off":true}`))
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			mux.ServeHTTP(res, req)
			if res.Code == http.StatusOK {
				successes.Add(1)
			} else if res.Code != http.StatusConflict {
				t.Errorf("unexpected status %d: %s", res.Code, res.Body.String())
			}
		}()
	}
	workers.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successful approvals = %d, want 1", successes.Load())
	}
	updated, ok := plans.Get(plan.ID)
	if !ok || updated.Status != domain.PlanApproved || !updated.SourcePowerOffApproved {
		t.Fatalf("approval state not recorded: %#v", updated)
	}
	if events := audit.All(); len(events) != 1 || events[0].Action != "plan.approve" {
		t.Fatalf("approval audit events = %#v", events)
	}
}
