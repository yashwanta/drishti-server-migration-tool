package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drishti/hypershift/internal/config"
	"github.com/drishti/hypershift/internal/job"
	"github.com/drishti/hypershift/internal/mock"
)

func TestExecutionRoutesUnlockOnlyWithBothLabInterlocks(t *testing.T) {
	tests := []struct {
		name            string
		mode            config.RunMode
		enableMigration bool
		enableMutation  bool
		wantLocked      bool
	}{
		{name: "mock with both flags", mode: config.ModeMock, enableMigration: true, enableMutation: true, wantLocked: true},
		{name: "lab without flags", mode: config.ModeLab, wantLocked: true},
		{name: "lab migration flag only", mode: config.ModeLab, enableMigration: true, wantLocked: true},
		{name: "lab mutation flag only", mode: config.ModeLab, enableMutation: true, wantLocked: true},
		{name: "lab with both flags", mode: config.ModeLab, enableMigration: true, enableMutation: true, wantLocked: false},
		{name: "live with both flags", mode: config.ModeLive, enableMigration: true, enableMutation: true, wantLocked: true},
		{name: "production with both flags", mode: config.ModeProduction, enableMigration: true, enableMutation: true, wantLocked: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handlers := NewHandlers(mock.New(), NewPlanStore(), NewAuditStore())
			engine := job.NewEngine(job.NewState(), NewMockFactory(), t.TempDir(), nil)
			migration := NewMigrationHandlers(handlers, engine, NewMockFactory(), t.TempDir())
			migration.ConfigureLabExecution(test.mode, test.enableMigration, test.enableMutation)
			mux := http.NewServeMux()
			migration.RegisterMigration(mux)

			for _, path := range []string{
				"/api/v1/plans/missing/execute",
				"/api/v1/jobs/missing/cutover",
				"/api/v1/jobs/missing/rollback",
			} {
				request := httptest.NewRequest(http.MethodPost, path, nil)
				response := httptest.NewRecorder()
				mux.ServeHTTP(response, request)
				if test.wantLocked {
					if response.Code != http.StatusNotImplemented || !strings.Contains(response.Body.String(), "execution_disabled") {
						t.Fatalf("%s escaped lock: status=%d body=%s", path, response.Code, response.Body.String())
					}
					continue
				}
				if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "execution_disabled") {
					t.Fatalf("lab route did not pass execution gate: %s status=%d body=%s", path, response.Code, response.Body.String())
				}
			}
		})
	}
}

func TestLegacyRemoteHookCannotChangeConfiguredModeDecision(t *testing.T) {
	handlers := NewHandlers(mock.New(), NewPlanStore(), NewAuditStore())
	migration := NewMigrationHandlers(handlers, nil, NewMockFactory(), t.TempDir())
	migration.ConfigureLabExecution(config.ModeLive, true, true)
	migration.EnableLabRemoteMigration(nil, true)
	if !migration.executionDisabled || !migration.executionLocked {
		t.Fatal("legacy hook changed the fail-closed live-mode decision")
	}
}
