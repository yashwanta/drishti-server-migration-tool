package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/drishti/hypershift/internal/api"
	"github.com/drishti/hypershift/internal/auth"
	"github.com/drishti/hypershift/internal/config"
	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/job"
	"github.com/drishti/hypershift/internal/mock"
	"github.com/drishti/hypershift/internal/rbac"
	"github.com/drishti/hypershift/internal/server"
	"github.com/drishti/hypershift/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func TestAuthenticatedRoutePersistsActorsAcrossPlanApprovalJobStepsAndAudit(t *testing.T) {
	databaseURL := os.Getenv("DRISHTI_TEST_DB_URL")
	if databaseURL == "" {
		t.Skip("DRISHTI_TEST_DB_URL is not set")
	}
	ctx := context.Background()
	database, err := store.OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	const actorID = "smoke-admin-local"
	hash, err := bcrypt.GenerateFromPassword([]byte("disposable-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	authService, err := auth.New([]auth.UserCredential{{
		User:     rbac.User{ID: actorID, Name: "Local Smoke Admin", Roles: []rbac.Role{rbac.RolePlatformAdmin}},
		Username: "smoke-admin", PasswordHash: string(hash),
	}}, time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	token, csrf, _, err := authService.Login("smoke-admin", "disposable-password")
	if err != nil {
		t.Fatal(err)
	}

	provider := mock.New()
	plans := api.NewPersistentPlanStore(database)
	audit := api.NewPersistentAuditStore(database)
	handlers := api.NewHandlers(provider, plans, audit)
	engine := job.NewEngine(job.NewPersistentState(database), api.NewMockFactory(), t.TempDir(), database.AppendAudit)
	srv := server.New(config.Config{Mode: config.ModeMock}, nil, authService)
	handlers.Register(srv.Router())
	api.NewMigrationHandlers(handlers, engine, api.NewMockFactory(), t.TempDir()).RegisterMigration(srv.Router())

	request := func(targetServer *server.Server, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", csrf)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		response := httptest.NewRecorder()
		targetServer.Handler().ServeHTTP(response, req)
		return response
	}

	created := request(srv, http.MethodPost, "/api/v1/plans", `{
		"name":"authenticated-postgres-smoke",
		"source_vm_id":"vm-web-01",
		"source_connection_id":"conn-vmware-lab",
		"target_node_id":"node-pve-01",
		"target_connection_id":"conn-proxmox-lab",
		"target_vm_name":"smoke-target",
		"strategy":"cold"
	}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create plan: status=%d body=%s", created.Code, created.Body.String())
	}
	var plan domain.Plan
	if err := json.Unmarshal(created.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.CreatedBy != actorID {
		t.Fatalf("created_by=%q want=%q", plan.CreatedBy, actorID)
	}

	plan.Status = domain.PlanPreflight
	plan.PreflightPassed = true
	plan.UpdatedAt = time.Now().UTC()
	if err := database.PutPlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	approved := request(srv, http.MethodPost, "/api/v1/plans/"+plan.ID+"/approve", `{"approve_source_power_off":true}`)
	if approved.Code != http.StatusOK {
		t.Fatalf("approve plan: status=%d body=%s", approved.Code, approved.Body.String())
	}

	// Exercise the same authenticated ActionExecute boundary without registering
	// or invoking the production execute handler, which remains locked at 501.
	jobServer := server.New(config.Config{Mode: config.ModeMock}, nil, authService)
	jobServer.Router().HandleFunc("POST /api/v1/jobs/{id}/validate", func(w http.ResponseWriter, r *http.Request) {
		if _, err := engine.Create(r.Context(), plan); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := database.AppendAudit(r.Context(), domain.AuditEvent{ID: "evt-smoke-job-" + plan.ID, Timestamp: time.Now().UTC(), Actor: auth.Actor(r.Context()), Action: "job.smoke_create", Target: "job-" + plan.ID, Result: "pending", Detail: "No migration execution invoked."}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	jobCreated := request(jobServer, http.MethodPost, "/api/v1/jobs/job-smoke/validate", `{}`)
	if jobCreated.Code != http.StatusNoContent {
		t.Fatalf("create pending job through authenticated boundary: status=%d body=%s", jobCreated.Code, jobCreated.Body.String())
	}

	persistedPlan, found, err := database.GetPlan(ctx, plan.ID)
	if err != nil || !found || persistedPlan.CreatedBy != actorID || persistedPlan.Status != domain.PlanApproved || !persistedPlan.SourcePowerOffApproved {
		t.Fatalf("persisted plan/approval: found=%v err=%v plan=%#v", found, err, persistedPlan)
	}
	persistedJob, found, err := database.GetJob(ctx, "job-"+plan.ID)
	if err != nil || !found || persistedJob.Actor != actorID || len(persistedJob.Steps) == 0 {
		t.Fatalf("persisted job: found=%v err=%v job=%#v", found, err, persistedJob)
	}
	for _, step := range persistedJob.Steps {
		if step.Actor != actorID {
			t.Fatalf("step %q actor=%q want=%q", step.Name, step.Actor, actorID)
		}
	}
	events, err := database.ListAudit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{"plan.create": false, "plan.approve": false, "job.smoke_create": false}
	for _, event := range events {
		if event.Target == plan.ID || event.Target == "job-"+plan.ID {
			if _, tracked := wanted[event.Action]; tracked && event.Actor == actorID {
				wanted[event.Action] = true
			}
		}
	}
	for action, found := range wanted {
		if !found {
			t.Fatalf("missing durable authenticated audit event %q for actor %q", action, actorID)
		}
	}
}
