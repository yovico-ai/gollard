package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `validate` is the only command that runs without a database, so it is the
// only one that can gate a push or a CI job. These pin the three failures it
// exists to catch — each of which is otherwise invisible until a deploy, and
// each of which stops the ENTIRE directory rather than just the offending
// file, because ImportDirectory parses everything before anything is applied.

const validHeader = `-- #!migration
-- name: "first",
-- description: "The first one.",
-- requires: [];
CREATE TABLE widgets (id int);
`

// writeDir materialises a migration directory and returns its path.
func writeDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func TestValidate_AcceptsAWellFormedDirectory(t *testing.T) {
	dir := writeDir(t, map[string]string{"001_first.sql": validHeader})

	if err := runValidate([]string{dir}); err != nil {
		t.Fatalf("runValidate() = %v, want nil", err)
	}
}

// The header is a comma-separated field list terminated by a semicolon.
// Omitting a separator is what took down every yovico environment on
// 2026-09-03: two files merged with `name:` and `description:` ending in
// nothing, and the failure surfaced only when the migrate Job ran.
func TestValidate_RejectsAHeaderMissingItsSeparator(t *testing.T) {
	broken := strings.Replace(validHeader, `-- name: "first",`, `-- name: "first"`, 1)
	dir := writeDir(t, map[string]string{"001_first.sql": broken})

	err := runValidate([]string{dir})
	if err == nil {
		t.Fatal("runValidate() = nil, want a parse error")
	}
	if !strings.Contains(err.Error(), "parse error") {
		t.Errorf("error = %q, want it to mention a parse error", err)
	}
}

// A malformed file must fail the run even when every other file is fine —
// that is the property that makes this worth gating on, and the reason a
// single bad migration is not a contained problem.
func TestValidate_OneBadFileFailsAnOtherwiseValidDirectory(t *testing.T) {
	dir := writeDir(t, map[string]string{
		"001_first.sql": validHeader,
		"002_second.sql": `-- #!migration
-- name: "second"
-- description: "Missing the comma above.",
-- requires: ["first"];
CREATE TABLE gadgets (id int);
`,
	})

	if err := runValidate([]string{dir}); err == nil {
		t.Fatal("runValidate() = nil, want the bad file to fail the whole directory")
	}
}

// A `requires` typo parses fine and only fails when the graph is built, so it
// would otherwise also survive to deploy time.
func TestValidate_RejectsARequiresThatNamesNothing(t *testing.T) {
	dir := writeDir(t, map[string]string{
		"001_first.sql": validHeader,
		"002_second.sql": `-- #!migration
-- name: "second",
-- description: "Depends on a name that does not exist.",
-- requires: ["frist"];
CREATE TABLE gadgets (id int);
`,
	})

	err := runValidate([]string{dir})
	if err == nil {
		t.Fatal("runValidate() = nil, want a missing-dependency error")
	}
	if !strings.Contains(err.Error(), "frist") {
		t.Errorf("error = %q, want it to name the unresolved dependency", err)
	}
}

func TestValidate_RejectsACycle(t *testing.T) {
	dir := writeDir(t, map[string]string{
		"001_a.sql": `-- #!migration
-- name: "a",
-- description: "Requires b.",
-- requires: ["b"];
CREATE TABLE a (id int);
`,
		"002_b.sql": `-- #!migration
-- name: "b",
-- description: "Requires a.",
-- requires: ["a"];
CREATE TABLE b (id int);
`,
	})

	if err := runValidate([]string{dir}); err == nil {
		t.Fatal("runValidate() = nil, want a circular-dependency error")
	}
}

func TestValidate_ReportsAMissingDirectory(t *testing.T) {
	if err := runValidate([]string{filepath.Join(t.TempDir(), "does-not-exist")}); err == nil {
		t.Fatal("runValidate() = nil, want an error for a directory that is not there")
	}
}
