package store

import (
	"context"
	"testing"
)

// fakeMigrator records applied versions in memory.
type fakeMigrator struct {
	ensured bool
	applied map[int]bool
	failOn  int
}

func newFake() *fakeMigrator { return &fakeMigrator{applied: map[int]bool{}} }

func (f *fakeMigrator) EnsureSchemaMigrations(ctx context.Context) error {
	f.ensured = true
	return nil
}
func (f *fakeMigrator) AppliedVersions(ctx context.Context) (map[int]bool, error) {
	return f.applied, nil
}
func (f *fakeMigrator) Apply(ctx context.Context, m Migration) error {
	if f.failOn == m.Version {
		return errSentinel
	}
	f.applied[m.Version] = true
	return nil
}

type sentinel struct{}

func (sentinel) Error() string { return "boom" }

var errSentinel = sentinel{}

func TestLoadEmbeddedMigrations(t *testing.T) {
	all, err := Load(Embedded())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("migration count = %d, want 4", len(all))
	}
	if all[0].Version != 1 {
		t.Errorf("first version = %d, want 1", all[0].Version)
	}
	if all[0].SQL == "" {
		t.Error("migration SQL empty")
	}
	if all[1].Version != 2 || all[1].Name != "0002_runtime_persistence.sql" {
		t.Fatalf("second migration = %#v", all[1])
	}
	if all[2].Version != 3 || all[2].Name != "0003_job_actors.sql" {
		t.Fatalf("third migration = %#v", all[2])
	}
	if all[3].Version != 4 || all[3].Name != "0004_auth_users.sql" {
		t.Fatalf("fourth migration = %#v", all[3])
	}
}

func TestUpIdempotent(t *testing.T) {
	ctx := context.Background()
	mig := newFake()
	if err := Up(ctx, Embedded(), mig); err != nil {
		t.Fatalf("first Up: %v", err)
	}
	if !mig.ensured {
		t.Error("EnsureSchemaMigrations not called")
	}
	if len(mig.applied) == 0 {
		t.Error("no migrations applied")
	}
	appliedCount := len(mig.applied)
	if err := Up(ctx, Embedded(), mig); err != nil {
		t.Fatalf("second Up: %v", err)
	}
	if len(mig.applied) != appliedCount {
		t.Errorf("re-running Up applied new migrations: %d != %d", len(mig.applied), appliedCount)
	}
}

func TestUpStopsOnError(t *testing.T) {
	mig := newFake()
	mig.failOn = 1
	err := Up(context.Background(), Embedded(), mig)
	if err == nil {
		t.Fatal("expected error")
	}
}
