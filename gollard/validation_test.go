package gollard

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateAppliedMigrations_Empty(t *testing.T) {
	planned := migTable(mig("a"), mig("b", "a"))
	if err := ValidateAppliedMigrations(planned, MigrationTable{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateAppliedMigrations_AllMatch(t *testing.T) {
	m := migWithScript("schema", "CREATE SCHEMA foo;")
	planned := migTable(m)
	applied := migTable(m)
	if err := ValidateAppliedMigrations(planned, applied); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateAppliedMigrations_ChecksumMismatch(t *testing.T) {
	planned := migTable(migWithScript("schema", "CREATE SCHEMA foo;"))
	applied := migTable(migWithScript("schema", "CREATE SCHEMA bar;"))
	err := ValidateAppliedMigrations(planned, applied)
	var dme *DigestMismatchError
	if !errors.As(err, &dme) {
		t.Fatalf("expected *DigestMismatchError, got %T: %v", err, err)
	}
	if dme.ID != "schema" {
		t.Errorf("ID: got %q want %q", dme.ID, "schema")
	}
	if dme.PlannedChecksum == dme.AppliedChecksum {
		t.Error("planned and applied checksums should differ")
	}
}

func TestValidateAppliedMigrations_MissingFromPlan(t *testing.T) {
	planned := migTable(mig("schema"))
	applied := migTable(migWithScript("orphan", "SELECT 1;"))
	err := ValidateAppliedMigrations(planned, applied)
	var ame *AppliedMigrationMissingError
	if !errors.As(err, &ame) {
		t.Fatalf("expected *AppliedMigrationMissingError, got %T: %v", err, err)
	}
	if ame.ID != "orphan" {
		t.Errorf("ID: got %q want %q", ame.ID, "orphan")
	}
}

func TestValidateAppliedMigration_Match(t *testing.T) {
	m := migWithScript("schema", "CREATE SCHEMA foo;")
	if err := ValidateAppliedMigration(migTable(m), m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateAppliedMigration_Mismatch(t *testing.T) {
	planned := migWithScript("schema", "CREATE SCHEMA foo;")
	applied := migWithScript("schema", "CREATE SCHEMA bar;")
	err := ValidateAppliedMigration(migTable(planned), applied)
	var dme *DigestMismatchError
	if !errors.As(err, &dme) {
		t.Fatalf("expected *DigestMismatchError, got %T: %v", err, err)
	}
}

func TestValidateAppliedMigration_Missing(t *testing.T) {
	applied := migWithScript("ghost", "SELECT 1;")
	err := ValidateAppliedMigration(migTable(mig("schema")), applied)
	var ame *AppliedMigrationMissingError
	if !errors.As(err, &ame) {
		t.Fatalf("expected *AppliedMigrationMissingError, got %T: %v", err, err)
	}
}

func TestCheckAppliedMigrations_AllMatch(t *testing.T) {
	m := migWithScript("schema", "CREATE SCHEMA foo;")
	comparisons, err := CheckAppliedMigrations(migTable(m), migTable(m))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comparisons) != 1 {
		t.Fatalf("expected 1 comparison, got %d", len(comparisons))
	}
	if !comparisons[0].Match {
		t.Errorf("expected Match=true")
	}
}

func TestCheckAppliedMigrations_Mismatch(t *testing.T) {
	planned := migWithScript("schema", "CREATE SCHEMA foo;")
	applied := migWithScript("schema", "CREATE SCHEMA bar;")
	comparisons, err := CheckAppliedMigrations(migTable(planned), migTable(applied))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comparisons) != 1 {
		t.Fatalf("expected 1 comparison, got %d", len(comparisons))
	}
	if comparisons[0].Match {
		t.Error("expected Match=false")
	}
	if comparisons[0].ID != "schema" {
		t.Errorf("ID: got %q want %q", comparisons[0].ID, "schema")
	}
	if comparisons[0].PlannedChecksum == comparisons[0].AppliedChecksum {
		t.Error("PlannedChecksum and AppliedChecksum should differ")
	}
}

func TestCheckAppliedMigrations_MissingFromPlan(t *testing.T) {
	applied := migWithScript("ghost", "SELECT 1;")
	_, err := CheckAppliedMigrations(migTable(mig("schema")), migTable(applied))
	var ame *AppliedMigrationMissingError
	if !errors.As(err, &ame) {
		t.Fatalf("expected *AppliedMigrationMissingError, got %T: %v", err, err)
	}
}

func TestCheckAppliedMigration_Match(t *testing.T) {
	m := migWithScript("schema", "CREATE SCHEMA foo;")
	c, err := CheckAppliedMigration(migTable(m), m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.Match {
		t.Error("expected Match=true")
	}
	if c.PlannedChecksum != c.AppliedChecksum {
		t.Error("checksums should match")
	}
}

func TestErrorMessages(t *testing.T) {
	t.Run("DigestMismatchError", func(t *testing.T) {
		planned := migWithScript("m", "SELECT 1;")
		applied := migWithScript("m", "SELECT 2;")
		err := ValidateAppliedMigration(migTable(planned), applied)
		msg := err.Error()
		for _, want := range []string{"m", "mismatch"} {
			if !strings.Contains(msg, want) {
				t.Errorf("error %q does not contain %q", msg, want)
			}
		}
	})

	t.Run("AppliedMigrationMissingError", func(t *testing.T) {
		applied := migWithScript("ghost", "SELECT 1;")
		err := ValidateAppliedMigration(MigrationTable{}, applied)
		msg := err.Error()
		if !strings.Contains(msg, "ghost") {
			t.Errorf("error %q should mention the migration ID", msg)
		}
	})
}
