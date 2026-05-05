package gollard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const singleMigrationSQL = `#!migration name: "file/alpha", description: "First migration.";
CREATE TABLE alpha (id INTEGER PRIMARY KEY);
`

const migrationAndTestSQL = `#!migration name: "file/beta", description: "Second migration.";
CREATE TABLE beta (id INTEGER PRIMARY KEY);
#!test name: "betaCheck", description: "Verify beta exists.";
SELECT COUNT(*) FROM beta;
`

func TestImportFile_SingleMigration(t *testing.T) {
	f := writeTempSQL(t, singleMigrationSQL)
	migrations, tests, err := ImportFile(f)
	if err != nil {
		t.Fatalf("ImportFile: %v", err)
	}
	if len(migrations) != 1 {
		t.Errorf("expected 1 migration, got %d", len(migrations))
	}
	if _, ok := migrations["file/alpha"]; !ok {
		t.Errorf("migration 'file/alpha' not found; got %v", migrations)
	}
	if len(tests) != 0 {
		t.Errorf("expected 0 tests, got %d", len(tests))
	}
}

func TestImportFile_MigrationAndTest(t *testing.T) {
	f := writeTempSQL(t, migrationAndTestSQL)
	migrations, tests, err := ImportFile(f)
	if err != nil {
		t.Fatalf("ImportFile: %v", err)
	}
	if len(migrations) != 1 {
		t.Errorf("expected 1 migration, got %d", len(migrations))
	}
	if len(tests) != 1 {
		t.Errorf("expected 1 test, got %d", len(tests))
	}
	if _, ok := tests["betaCheck"]; !ok {
		t.Errorf("test 'betaCheck' not found; got %v", tests)
	}
}

func TestImportFile_ParseError(t *testing.T) {
	f := writeTempSQL(t, "SELECT 1; -- not a gollard file")
	_, _, err := ImportFile(f)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestImportFile_NotFound(t *testing.T) {
	_, _, err := ImportFile("/does/not/exist.sql")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestImportDirectory_ExampleContacts(t *testing.T) {
	migrations, tests, err := ImportDirectory("../sql/example-contacts")
	if err != nil {
		t.Fatalf("ImportDirectory: %v", err)
	}
	// Example contacts has: schema, tables/person, tables/phone, tables/phone/name
	for _, want := range []MigrationID{"schema", "tables/person", "tables/phone", "tables/phone/name"} {
		if _, ok := migrations[want]; !ok {
			t.Errorf("expected migration %q, not found in %v", want, migrations)
		}
	}
	if len(tests) == 0 {
		t.Error("expected at least one test from example-contacts")
	}
}

func TestImportDirectory_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.sql"),
		`#!migration name: "dir/a", description: "Migration A.";
CREATE TABLE a (id int);
`)
	writeFile(t, filepath.Join(dir, "b.sql"),
		`#!migration name: "dir/b", description: "Migration B.";
CREATE TABLE b (id int);
`)
	migrations, _, err := ImportDirectory(dir)
	if err != nil {
		t.Fatalf("ImportDirectory: %v", err)
	}
	if len(migrations) != 2 {
		t.Errorf("expected 2 migrations, got %d: %v", len(migrations), migrations)
	}
}

func TestImportDirectory_PropagatesParseError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bad.sql"), "not a gollard file")
	_, _, err := ImportDirectory(dir)
	if err == nil {
		t.Fatal("expected error from malformed file, got nil")
	}
	if !strings.Contains(err.Error(), "expected '#!'") {
		t.Errorf("error %q should mention missing '#!'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// adjustDependencies — the core implicit-chaining behaviour from File.hs
// ---------------------------------------------------------------------------

func TestAdjustDependencies_NoChainForSingleMigration(t *testing.T) {
	actions := []Action{{Migration: &Migration{Name: "a"}}}
	got := adjustDependencies(actions)
	if len(got[0].Migration.Requires) != 0 {
		t.Errorf("single migration should have no implicit requires, got %v", got[0].Migration.Requires)
	}
}

func TestAdjustDependencies_ImplicitChain(t *testing.T) {
	// Two migrations with no explicit requires: b should implicitly require a.
	actions := []Action{
		{Migration: &Migration{Name: "a"}},
		{Migration: &Migration{Name: "b"}},
	}
	got := adjustDependencies(actions)
	reqs := got[1].Migration.Requires
	if len(reqs) != 1 || reqs[0] != "a" {
		t.Errorf("b should implicitly require a, got %v", reqs)
	}
}

func TestAdjustDependencies_ExplicitDepNotDuplicated(t *testing.T) {
	// b already lists a explicitly — adjustDependencies must not add it again.
	actions := []Action{
		{Migration: &Migration{Name: "a"}},
		{Migration: &Migration{Name: "b", Requires: []MigrationID{"a"}}},
	}
	got := adjustDependencies(actions)
	reqs := got[1].Migration.Requires
	if len(reqs) != 1 || reqs[0] != "a" {
		t.Errorf("requires should contain 'a' exactly once, got %v", reqs)
	}
}

func TestAdjustDependencies_TestsTransparent(t *testing.T) {
	// A test between two migrations must not break the chain.
	actions := []Action{
		{Migration: &Migration{Name: "a"}},
		{Test: &Test{Name: "t"}},
		{Migration: &Migration{Name: "b"}},
	}
	got := adjustDependencies(actions)
	if got[1].Test == nil {
		t.Error("test action should be preserved")
	}
	reqs := got[2].Migration.Requires
	if len(reqs) != 1 || reqs[0] != "a" {
		t.Errorf("b should still implicitly require a through a test, got %v", reqs)
	}
}

func TestAdjustDependencies_ThreeInChain(t *testing.T) {
	// a → b → c with no explicit deps.
	actions := []Action{
		{Migration: &Migration{Name: "a"}},
		{Migration: &Migration{Name: "b"}},
		{Migration: &Migration{Name: "c"}},
	}
	got := adjustDependencies(actions)
	if r := got[1].Migration.Requires; len(r) != 1 || r[0] != "a" {
		t.Errorf("b.Requires: got %v want [a]", r)
	}
	if r := got[2].Migration.Requires; len(r) != 1 || r[0] != "b" {
		t.Errorf("c.Requires: got %v want [b]", r)
	}
}

func TestAdjustDependencies_SkipToEarlierAncestor(t *testing.T) {
	// c explicitly requires a (skipping b). b is implicitly added to c's chain.
	// This mirrors the real-world pattern in chat.sql.
	actions := []Action{
		{Migration: &Migration{Name: "a"}},
		{Migration: &Migration{Name: "b", Requires: []MigrationID{"a"}}},
		{Migration: &Migration{Name: "c", Requires: []MigrationID{"a"}}},
	}
	got := adjustDependencies(actions)
	reqs := got[2].Migration.Requires
	// b should be prepended since it's not already listed.
	if len(reqs) < 2 {
		t.Fatalf("c should require both b and a, got %v", reqs)
	}
	if reqs[0] != "b" {
		t.Errorf("b should be prepended first, got %v", reqs)
	}
	found := false
	for _, r := range reqs {
		if r == "a" {
			found = true
		}
	}
	if !found {
		t.Errorf("a should still be in c's requires: %v", reqs)
	}
}

func TestImportFile_ImplicitChaining(t *testing.T) {
	// Two migrations in one file with no explicit requires — the second should
	// implicitly require the first after importFileInto applies adjustDependencies.
	content := `#!migration name: "first", description: "First.";
CREATE TABLE first(id int);
#!migration name: "second", description: "Second.";
CREATE TABLE second(id int);
`
	f := writeTempSQL(t, content)
	migrations, _, err := ImportFile(f)
	if err != nil {
		t.Fatalf("ImportFile: %v", err)
	}
	second, ok := migrations["second"]
	if !ok {
		t.Fatal("migration 'second' not found")
	}
	if len(second.Requires) != 1 || second.Requires[0] != "first" {
		t.Errorf("second should implicitly require first, got %v", second.Requires)
	}
}

func TestImportFile_ImplicitChainingWithTest(t *testing.T) {
	// A test between migrations must not break the implicit chain.
	content := `#!migration name: "a", description: "A.";
CREATE TABLE a(id int);
#!test name: "checkA", description: "Check A.";
SELECT COUNT(*) FROM a;
#!migration name: "b", description: "B.";
CREATE TABLE b(id int);
`
	f := writeTempSQL(t, content)
	migrations, _, err := ImportFile(f)
	if err != nil {
		t.Fatalf("ImportFile: %v", err)
	}
	b, ok := migrations["b"]
	if !ok {
		t.Fatal("migration 'b' not found")
	}
	if len(b.Requires) != 1 || b.Requires[0] != "a" {
		t.Errorf("b should implicitly require a through test, got %v", b.Requires)
	}
}

// ---------------------------------------------------------------------------

// TestImportDirectory_IgnoresNonSqlFiles verifies that non-.sql files (e.g.
// .DS_Store, README.md) are silently skipped and do not cause parse errors.
func TestImportDirectory_IgnoresNonSqlFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "migration.sql"),
		`#!migration name: "m", description: "d.";
CREATE TABLE m (id int);
`)
	writeFile(t, filepath.Join(dir, ".DS_Store"), "binary junk")
	writeFile(t, filepath.Join(dir, "README.md"), "# Not a migration file")
	writeFile(t, filepath.Join(dir, "notes.txt"), "some notes")

	migrations, _, err := ImportDirectory(dir)
	if err != nil {
		t.Fatalf("ImportDirectory should ignore non-.sql files, got: %v", err)
	}
	if len(migrations) != 1 {
		t.Errorf("expected 1 migration, got %d: %v", len(migrations), migrations)
	}
}

func writeTempSQL(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.sql")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writeFile %s: %v", path, err)
	}
}
