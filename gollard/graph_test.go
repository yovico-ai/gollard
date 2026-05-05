package gollard

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestMakeMigrationGraph_Empty(t *testing.T) {
	g, err := MakeMigrationGraph(MigrationTable{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(g.nodes))
	}
}

func TestMakeMigrationGraph_SingleNode(t *testing.T) {
	g, err := MakeMigrationGraph(migTable(mig("alpha")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.nodes) != 1 || g.nodes[0] != "alpha" {
		t.Errorf("unexpected nodes: %v", g.nodes)
	}
}

func TestMakeMigrationGraph_LinearChain(t *testing.T) {
	g, err := MakeMigrationGraph(migTable(
		mig("a"),
		mig("b", "a"),
		mig("c", "b"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := g.GetUnappliedMigrations(nil)
	want := []MigrationID{"a", "b", "c"}
	if !slices.Equal(got, want) {
		t.Errorf("topological order: got %v want %v", got, want)
	}
}

func TestMakeMigrationGraph_MissingDependency(t *testing.T) {
	_, err := MakeMigrationGraph(migTable(mig("a", "nonexistent")))
	var mde *MissingDependencyError
	if !errors.As(err, &mde) {
		t.Fatalf("expected *MissingDependencyError, got %T: %v", err, err)
	}
	if mde.Migration != "a" {
		t.Errorf("Migration: got %q want %q", mde.Migration, "a")
	}
	if mde.Requires != "nonexistent" {
		t.Errorf("Requires: got %q want %q", mde.Requires, "nonexistent")
	}
	if !strings.Contains(mde.Error(), "a") || !strings.Contains(mde.Error(), "nonexistent") {
		t.Errorf("error message %q should mention both IDs", mde.Error())
	}
}

func TestMakeMigrationGraph_SimpleCycle(t *testing.T) {
	_, err := MakeMigrationGraph(migTable(
		mig("a", "b"),
		mig("b", "a"),
	))
	var cde *CircularDependencyError
	if !errors.As(err, &cde) {
		t.Fatalf("expected *CircularDependencyError, got %T: %v", err, err)
	}
	if len(cde.Cycle) == 0 {
		t.Error("cycle should not be empty")
	}
	if !strings.Contains(cde.Error(), "circular") {
		t.Errorf("error message %q should mention 'circular'", cde.Error())
	}
}

func TestMakeMigrationGraph_ThreeNodeCycle(t *testing.T) {
	_, err := MakeMigrationGraph(migTable(
		mig("a", "b"),
		mig("b", "c"),
		mig("c", "a"),
	))
	var cde *CircularDependencyError
	if !errors.As(err, &cde) {
		t.Fatalf("expected *CircularDependencyError, got %T: %v", err, err)
	}
	if len(cde.Cycle) < 3 {
		t.Errorf("expected cycle of at least 3 nodes, got %v", cde.Cycle)
	}
}

// TestMakeMigrationGraph_NoFalsePositiveCycle verifies that a valid
// DAG with a shared dependency (diamond) is accepted without error.
func TestMakeMigrationGraph_NoFalsePositiveCycle(t *testing.T) {
	_, err := MakeMigrationGraph(migTable(
		mig("schema"),
		mig("person", "schema"),
		mig("company", "schema"),
		mig("employee", "person", "company"),
	))
	if err != nil {
		t.Fatalf("unexpected error on valid diamond DAG: %v", err)
	}
}

func TestGetUnappliedMigrations_NoneApplied(t *testing.T) {
	g, _ := MakeMigrationGraph(migTable(
		mig("schema"),
		mig("tables/person", "schema"),
	))
	got := g.GetUnappliedMigrations(nil)
	want := []MigrationID{"schema", "tables/person"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestGetUnappliedMigrations_AllApplied(t *testing.T) {
	g, _ := MakeMigrationGraph(migTable(mig("a"), mig("b", "a")))
	got := g.GetUnappliedMigrations([]MigrationID{"a", "b"})
	if len(got) != 0 {
		t.Errorf("expected 0 unapplied, got %v", got)
	}
}

func TestGetUnappliedMigrations_Partial(t *testing.T) {
	g, _ := MakeMigrationGraph(migTable(
		mig("schema"),
		mig("person", "schema"),
		mig("phone", "person"),
	))
	got := g.GetUnappliedMigrations([]MigrationID{"schema"})
	want := []MigrationID{"person", "phone"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestGetUnappliedMigrations_AlphabeticalTieBreaking(t *testing.T) {
	// alpha and beta both depend on root; alphabetical order should be preserved.
	g, _ := MakeMigrationGraph(migTable(
		mig("root"),
		mig("beta", "root"),
		mig("alpha", "root"),
	))
	got := g.GetUnappliedMigrations(nil)
	want := []MigrationID{"root", "alpha", "beta"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestGetUnappliedMigrations_Diamond(t *testing.T) {
	// schema → person  ↘
	//         → company → employee
	g, _ := MakeMigrationGraph(migTable(
		mig("schema"),
		mig("person", "schema"),
		mig("company", "schema"),
		mig("employee", "person", "company"),
	))
	got := g.GetUnappliedMigrations(nil)
	if len(got) != 4 {
		t.Fatalf("expected 4 migrations, got %d: %v", len(got), got)
	}
	if got[0] != "schema" {
		t.Errorf("schema must come first, got %q", got[0])
	}
	if got[len(got)-1] != "employee" {
		t.Errorf("employee must come last, got %q", got[len(got)-1])
	}
}

// TestMakeMigrationGraph_DuplicateRequires verifies that a migration listing
// the same dependency twice (as seen in real-world files) is accepted and does
// not produce a cycle error or corrupt the topological sort.
func TestMakeMigrationGraph_DuplicateRequires(t *testing.T) {
	// "m" requires ["a", "b", "a"] — "a" appears twice.
	m := Migration{
		Name:     "m",
		Requires: []MigrationID{"a", "b", "a"},
	}
	g, err := MakeMigrationGraph(migTable(mig("a"), mig("b"), m))
	if err != nil {
		t.Fatalf("duplicate requires should not cause error: %v", err)
	}
	got := g.GetUnappliedMigrations(nil)
	// a and b must both precede m; exact a/b order is alphabetical.
	if len(got) != 3 {
		t.Fatalf("expected 3 migrations, got %d: %v", len(got), got)
	}
	if got[len(got)-1] != "m" {
		t.Errorf("m must come last, got %v", got)
	}
}

// TestMakeMigrationGraph_FanOut verifies a single migration depended on by many
// others — the real-world pattern of adding ENUM values to a base type.
func TestMakeMigrationGraph_FanOut(t *testing.T) {
	// base_type is required by 5 independent addValue migrations.
	table := migTable(
		mig("base_type"),
		mig("add_foo", "base_type"),
		mig("add_bar", "base_type"),
		mig("add_baz", "base_type"),
		mig("add_qux", "base_type"),
		mig("add_zap", "base_type"),
	)
	g, err := MakeMigrationGraph(table)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := g.GetUnappliedMigrations(nil)
	if len(got) != 6 {
		t.Fatalf("expected 6 migrations, got %d: %v", len(got), got)
	}
	if got[0] != "base_type" {
		t.Errorf("base_type must be first, got %q", got[0])
	}
	// All add_* variants must appear after base_type.
	for _, id := range got[1:] {
		if id == "base_type" {
			t.Errorf("base_type appears more than once: %v", got)
		}
	}
}

// TestMakeMigrationGraph_CrossNamespace verifies that migrations using
// slash-namespaced IDs (e.g. "customer/foo", "global/bar") can declare
// cross-namespace dependencies correctly.
func TestMakeMigrationGraph_CrossNamespace(t *testing.T) {
	table := migTable(
		mig("public/uuid"),
		mig("global/customers", "public/uuid"),
		mig("customer/connections", "public/uuid"),
		mig("customer/connections-fk", "customer/connections", "global/customers"),
	)
	g, err := MakeMigrationGraph(table)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := g.GetUnappliedMigrations(nil)
	if len(got) != 4 {
		t.Fatalf("expected 4 migrations, got %d: %v", len(got), got)
	}
	if got[0] != "public/uuid" {
		t.Errorf("public/uuid must be first, got %q", got[0])
	}
	if got[len(got)-1] != "customer/connections-fk" {
		t.Errorf("customer/connections-fk must be last, got %q", got[len(got)-1])
	}
}

// TestGetUnappliedMigrations_AppliedNotInGraph verifies that IDs in the
// applied list that are absent from the graph are silently ignored.
func TestGetUnappliedMigrations_AppliedNotInGraph(t *testing.T) {
	g, _ := MakeMigrationGraph(migTable(mig("a"), mig("b", "a")))
	got := g.GetUnappliedMigrations([]MigrationID{"a", "ghost"})
	want := []MigrationID{"b"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}
