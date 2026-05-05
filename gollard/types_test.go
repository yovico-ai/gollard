package gollard

import (
	"slices"
	"testing"
)

func TestHashScript_Deterministic(t *testing.T) {
	script := "CREATE TABLE foo (id INTEGER PRIMARY KEY);"
	if HashScript(script) != HashScript(script) {
		t.Error("HashScript must return the same digest for the same input")
	}
}

func TestHashScript_UniqueOutputs(t *testing.T) {
	if HashScript("SELECT 1;") == HashScript("SELECT 2;") {
		t.Error("HashScript should produce different digests for different scripts")
	}
}

func TestHashScript_EmptyString(t *testing.T) {
	// SHA-256 of "" is a defined constant — verify we get a non-zero digest.
	d := HashScript("")
	var zero Digest
	if d == zero {
		t.Error("HashScript(\"\") should not return zero digest")
	}
}

func TestHashScript_WhitespaceMatters(t *testing.T) {
	// Trailing whitespace produces a different checksum; this is intentional
	// because trimRightHeaderWhitespace is applied before hashing.
	if HashScript("SELECT 1;") == HashScript("SELECT 1; ") {
		t.Error("scripts differing only in trailing whitespace should hash differently")
	}
}

func TestInflateMigrationIDs_HappyPath(t *testing.T) {
	a := migWithScript("a", "SELECT 1;")
	b := migWithScript("b", "SELECT 2;")
	table := migTable(a, b)

	got, err := InflateMigrationIDs(table, []MigrationID{"a", "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 migrations, got %d", len(got))
	}
	if got[0].Name != "a" || got[1].Name != "b" {
		t.Errorf("unexpected order: %v %v", got[0].Name, got[1].Name)
	}
}

func TestInflateMigrationIDs_PreservesOrder(t *testing.T) {
	a := migWithScript("a", "SELECT 1;")
	b := migWithScript("b", "SELECT 2;")
	table := migTable(a, b)

	// Request in reverse order — output order must match input order.
	got, err := InflateMigrationIDs(table, []MigrationID{"b", "a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Name != "b" || got[1].Name != "a" {
		t.Errorf("output order should match input: got %v %v", got[0].Name, got[1].Name)
	}
}

func TestInflateMigrationIDs_Empty(t *testing.T) {
	got, err := InflateMigrationIDs(migTable(mig("a")), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

func TestInflateMigrationIDs_MissingID(t *testing.T) {
	_, err := InflateMigrationIDs(migTable(mig("a")), []MigrationID{"a", "ghost"})
	if err == nil {
		t.Fatal("expected error for missing ID")
	}
	mmte, ok := err.(*MigrationMissingFromTableError)
	if !ok {
		t.Fatalf("expected *MigrationMissingFromTableError, got %T", err)
	}
	if mmte.ID != "ghost" {
		t.Errorf("ID: got %q want %q", mmte.ID, "ghost")
	}
}

// TestDigestEquality verifies that Digest comparison works as expected since
// it is used as a map key and compared directly in validation.
func TestDigestEquality(t *testing.T) {
	d1 := HashScript("SELECT 1;")
	d2 := HashScript("SELECT 1;")
	d3 := HashScript("SELECT 2;")

	if d1 != d2 {
		t.Error("same script should produce equal digests")
	}
	if d1 == d3 {
		t.Error("different scripts should produce unequal digests")
	}

	// Verify Digest works as a map key.
	m := map[Digest]string{d1: "one"}
	if m[d2] != "one" {
		t.Error("equal digests should look up to the same map entry")
	}
	_ = slices.Contains([]Digest{d1}, d2) // compile-time check that Digest is comparable
}
