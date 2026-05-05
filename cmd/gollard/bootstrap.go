package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/fvc-ai/gollard/gollard"
	"github.com/jackc/pgx/v5/pgxpool"
)

// runBootstrapFromAtlas seeds mallard.applied_migrations from the schema
// directory's contents, then drops Atlas's atlas_schema_revisions schema.
//
// Use this once when migrating an existing project off Atlas. Atlas's
// applied state lives in atlas_schema_revisions.atlas_schema_revisions
// and uses a different (version, hash) key than mallard's
// applied_migrations.name. Since the SQL bodies are identical (we
// converted the files in-place by adding gollard headers), we can copy
// the applied set across without re-running anything.
//
// Idempotent: if atlas_schema_revisions doesn't exist, this is a no-op.
// If a migration's name is already in mallard.applied_migrations, it
// gets skipped. Safe to chain before `gollard migrate` on every boot
// during the cutover window.
func runBootstrapFromAtlas(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("bootstrap-from-atlas", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: gollard bootstrap-from-atlas ROOT [options]")
		fs.PrintDefaults()
	}
	var c connOpts
	registerConnFlags(fs, &c)
	if err := parseRootAndFlags(fs, args, "ROOT"); err != nil {
		return err
	}
	root := fs.Arg(0)

	pool, err := openPool(ctx, &c)
	if err != nil {
		return err
	}
	defer pool.Close()

	return bootstrapFromAtlas(ctx, pool, root)
}

// bootstrapFromAtlas is the actual logic, factored so `migrate
// --from-atlas` can chain into it without re-parsing flags.
func bootstrapFromAtlas(ctx context.Context, pool *pgxpool.Pool, root string) error {
	// Detect atlas. If absent, we're already migrated; bail clean.
	var atlasExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.schemata
			WHERE schema_name = 'atlas_schema_revisions'
		)`).Scan(&atlasExists); err != nil {
		return fmt.Errorf("probe atlas schema: %w", err)
	}
	if !atlasExists {
		fmt.Println("bootstrap-from-atlas: atlas_schema_revisions schema not present, nothing to do")
		return nil
	}

	db := gollard.NewDB(pool)
	if err := db.EnsureMigrationSchema(ctx); err != nil {
		return fmt.Errorf("ensure mallard schema: %w", err)
	}

	planned, _, err := gollard.ImportDirectory(root)
	if err != nil {
		return fmt.Errorf("import %s: %w", root, err)
	}

	graph, err := gollard.MakeMigrationGraph(planned)
	if err != nil {
		return fmt.Errorf("graph: %w", err)
	}
	// Topo order so applied_migrations rows read top-down.
	order := graph.GetUnappliedMigrations(nil)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	inserted := 0
	for _, id := range order {
		m, ok := planned[id]
		if !ok {
			return fmt.Errorf("migration %q missing from plan", id)
		}
		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM mallard.applied_migrations WHERE name = $1)`,
			string(m.Name),
		).Scan(&exists); err != nil {
			return fmt.Errorf("probe %s: %w", m.Name, err)
		}
		if exists {
			continue
		}
		reqs := make([]string, len(m.Requires))
		for i, r := range m.Requires {
			reqs[i] = string(r)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO mallard.applied_migrations (name, description, requires, checksum, script_text)
			 VALUES ($1, $2, $3, $4, $5)`,
			string(m.Name), m.Description, reqs, m.Checksum[:], m.Script,
		); err != nil {
			return fmt.Errorf("seed %s: %w", m.Name, err)
		}
		inserted++
	}

	if _, err := tx.Exec(ctx, `DROP SCHEMA atlas_schema_revisions CASCADE`); err != nil {
		return fmt.Errorf("drop atlas schema: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Printf("bootstrap-from-atlas: seeded %d migrations, dropped atlas_schema_revisions\n", inserted)
	return nil
}
