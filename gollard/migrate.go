package gollard

import (
	"context"
	"fmt"
	"slices"
)

// Migrate applies any unapplied migrations under the given root directory.
// If runTests is true, every test under the root is run after the migrations.
func Migrate(ctx context.Context, db *DB, root string, runTests bool) error {
	if err := db.EnsureMigrationSchema(ctx); err != nil {
		return err
	}
	planned, tests, err := ImportDirectory(root)
	if err != nil {
		return err
	}
	applied, err := db.GetAppliedMigrations(ctx)
	if err != nil {
		return err
	}
	if err := ValidateAppliedMigrations(planned, applied); err != nil {
		return err
	}
	graph, err := MakeMigrationGraph(planned)
	if err != nil {
		return err
	}
	appliedIDs := make([]MigrationID, 0, len(applied))
	for id := range applied {
		appliedIDs = append(appliedIDs, id)
	}
	unapplied := graph.GetUnappliedMigrations(appliedIDs)
	toApply, err := InflateMigrationIDs(planned, unapplied)
	if err != nil {
		return err
	}
	if err := db.ApplyMigrations(ctx, toApply); err != nil {
		return err
	}
	if runTests {
		ts := make([]Test, 0, len(tests))
		for _, t := range tests {
			ts = append(ts, t)
		}
		// Deterministic test order.
		slices.SortFunc(ts, func(a, b Test) int {
			if a.Name < b.Name {
				return -1
			}
			if a.Name > b.Name {
				return 1
			}
			return 0
		})
		if err := db.RunTests(ctx, ts); err != nil {
			return err
		}
	}
	return nil
}

// ConfirmChecksums prints, for every applied migration, whether its checksum
// still matches the migration in the directory.
func ConfirmChecksums(ctx context.Context, db *DB, root string) error {
	planned, _, err := ImportDirectory(root)
	if err != nil {
		return err
	}
	applied, err := db.GetAppliedMigrations(ctx)
	if err != nil {
		return err
	}
	cs, err := CheckAppliedMigrations(planned, applied)
	if err != nil {
		return err
	}
	slices.SortFunc(cs, func(a, b DigestComparison) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	for _, c := range cs {
		status := "Invalid"
		if c.Match {
			status = "Valid"
		}
		fmt.Printf("%s : %s\n\n", c.ID, status)
		fmt.Printf("    %x\n", c.PlannedChecksum)
		fmt.Printf("    %x\n\n", c.AppliedChecksum)
	}
	return nil
}

// RepairChecksum overwrites the checksum recorded in the database for the
// named migration with the checksum computed from the planned source files.
func RepairChecksum(ctx context.Context, db *DB, root string, id MigrationID) error {
	planned, _, err := ImportDirectory(root)
	if err != nil {
		return err
	}
	applied, err := db.GetAppliedMigrations(ctx)
	if err != nil {
		return err
	}
	p, okPlanned := planned[id]
	_, okApplied := applied[id]
	if !okPlanned || !okApplied {
		return fmt.Errorf("could not find migration %q in either directory or applied database state", id)
	}
	return db.SetChecksum(ctx, id, p.Checksum)
}
