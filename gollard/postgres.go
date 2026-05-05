package gollard

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{Pool: pool}
}

// EnsureMigrationSchema bootstraps the internal `mallard` schema, applying any
// versioned schema scripts that have not yet been recorded.
func (db *DB) EnsureMigrationSchema(ctx context.Context) error {
	current, init, err := db.getMigrationSchemaVersion(ctx)
	if err != nil {
		return err
	}
	scripts := mallardSchemaScripts()
	startIdx := 0
	if init {
		startIdx = int(current) + 1
	}
	for i := startIdx; i < len(scripts); i++ {
		s := scripts[i]
		if err := db.applySchemaScript(ctx, s); err != nil {
			return fmt.Errorf("apply mallard schema migration %d: %w", s.Version, err)
		}
		fmt.Printf("Migrator Version: %d\n", s.Version)
	}
	return nil
}

func (db *DB) applySchemaScript(ctx context.Context, s schemaScript) error {
	return db.runTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, string(s.Script)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "INSERT INTO mallard.migrator_version (version) VALUES ($1)", s.Version)
		return err
	})
}

// getMigrationSchemaVersion returns (currentVersion, initialized).
// initialized=false means the mallard schema has never been bootstrapped.
func (db *DB) getMigrationSchemaVersion(ctx context.Context) (int64, bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'mallard' AND table_name = 'migrator_version'
		)`).Scan(&exists)
	if err != nil {
		return 0, false, err
	}
	if !exists {
		return 0, false, nil
	}
	var maxVersion int64
	if err := db.Pool.QueryRow(ctx, `SELECT coalesce(max(version), 0) FROM mallard.migrator_version`).Scan(&maxVersion); err != nil {
		return 0, false, err
	}
	return maxVersion, true, nil
}

func (db *DB) GetAppliedMigrations(ctx context.Context) (MigrationTable, error) {
	rows, err := db.Pool.Query(ctx, `SELECT name, description, requires, checksum, script_text FROM mallard.applied_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := MigrationTable{}
	for rows.Next() {
		var (
			name     string
			desc     string
			requires []string
			checksum []byte
			script   string
		)
		if err := rows.Scan(&name, &desc, &requires, &checksum, &script); err != nil {
			return nil, err
		}
		if len(checksum) != sha256.Size {
			return nil, fmt.Errorf("invalid checksum length for migration %q: got %d, want %d", name, len(checksum), sha256.Size)
		}
		var d Digest
		copy(d[:], checksum)
		reqs := make([]MigrationID, len(requires))
		for i, r := range requires {
			reqs[i] = MigrationID(r)
		}
		out[MigrationID(name)] = Migration{
			Name:        MigrationID(name),
			Description: desc,
			Requires:    reqs,
			Checksum:    d,
			Script:      script,
		}
	}
	return out, rows.Err()
}

func (db *DB) ApplyMigrations(ctx context.Context, ms []Migration) error {
	for _, m := range ms {
		if err := db.ApplyMigration(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) ApplyMigration(ctx context.Context, m Migration) error {
	err := db.runTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, m.Script); err != nil {
			return err
		}
		reqs := make([]string, len(m.Requires))
		for i, r := range m.Requires {
			reqs[i] = string(r)
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO mallard.applied_migrations (name, description, requires, checksum, script_text) VALUES ($1, $2, $3, $4, $5)`,
			string(m.Name), m.Description, reqs, m.Checksum[:], m.Script)
		return err
	})
	if err != nil {
		return fmt.Errorf("apply migration %q: %w", m.Name, err)
	}
	fmt.Printf("Applied migration: %s\n", m.Name)
	return nil
}

// RunTests runs each test inside a Serializable transaction that is always
// rolled back at the end. A test is considered failed iff its SQL errors.
func (db *DB) RunTests(ctx context.Context, ts []Test) error {
	for _, t := range ts {
		if err := db.RunTest(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) RunTest(ctx context.Context, t Test) error {
	tx, err := db.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable, AccessMode: pgx.ReadWrite})
	if err != nil {
		return err
	}
	// Always roll back, mirroring HT.condemn.
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, t.Script); err != nil {
		return fmt.Errorf("test %q failed: %w", t.Name, err)
	}
	return nil
}

func (db *DB) SetChecksum(ctx context.Context, id MigrationID, checksum Digest) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE mallard.applied_migrations SET checksum = $1 WHERE name = $2`,
		checksum[:], string(id))
	return err
}

func (db *DB) runTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := db.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable, AccessMode: pgx.ReadWrite})
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}
