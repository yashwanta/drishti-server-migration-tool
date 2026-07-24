// Package store owns the database migration baseline. In mock mode an in-memory
// store is used; when DRISHTI_DB_URL is set, these migrations apply to Postgres.
package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// Embedded exposes the bundled migrations as an fs.FS.
func Embedded() fs.FS {
	sub, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		panic(fmt.Sprintf("store: embed sub: %v", err))
	}
	return sub
}

// Migration is a single ordered SQL migration.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// Load reads all migrations from f, sorted by version. The directory entries
// must be named like 0001_init.sql.
func Load(f fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(f, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	var out []Migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		parts := strings.SplitN(e.Name(), "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("unexpected migration filename %q", e.Name())
		}
		var ver int
		if _, err := fmt.Sscanf(parts[0], "%d", &ver); err != nil {
			return nil, fmt.Errorf("parse migration version %q: %w", e.Name(), err)
		}
		body, err := fs.ReadFile(f, e.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", e.Name(), err)
		}
		out = append(out, Migration{Version: ver, Name: e.Name(), SQL: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// Migrator is the interface a concrete DB driver satisfies.
type Migrator interface {
	EnsureSchemaMigrations(ctx context.Context) error
	AppliedVersions(ctx context.Context) (map[int]bool, error)
	Apply(ctx context.Context, m Migration) error
}

// Up applies all pending migrations in order. It is idempotent.
func Up(ctx context.Context, f fs.FS, mig Migrator) error {
	all, err := Load(f)
	if err != nil {
		return err
	}
	if err := mig.EnsureSchemaMigrations(ctx); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	applied, err := mig.AppliedVersions(ctx)
	if err != nil {
		return err
	}
	for _, m := range all {
		if applied[m.Version] {
			continue
		}
		if err := mig.Apply(ctx, m); err != nil {
			return fmt.Errorf("apply %s: %w", m.Name, err)
		}
	}
	return nil
}