package gollard

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseSchemaFile(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "sql", "example-contacts", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema.sql: %v", err)
	}
	actions, err := ParseActions("schema.sql", string(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	m := actions[0].Migration
	if m == nil {
		t.Fatalf("expected migration, got test")
	}
	if m.Name != "schema" {
		t.Errorf("name: got %q want %q", m.Name, "schema")
	}
	if m.Description != "Construct primary schema." {
		t.Errorf("description: got %q", m.Description)
	}
	if len(m.Requires) != 0 {
		t.Errorf("requires: got %v want empty", m.Requires)
	}
	if !strings.Contains(m.Script, "CREATE SCHEMA contact") {
		t.Errorf("script missing expected SQL: %q", m.Script)
	}
}

func TestParsePhoneFile(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "sql", "example-contacts", "tables", "phone.sql"))
	if err != nil {
		t.Fatalf("read phone.sql: %v", err)
	}
	actions, err := ParseActions("phone.sql", string(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}
	first := actions[0].Migration
	if first == nil || first.Name != "tables/phone" {
		t.Fatalf("first action: %+v", actions[0])
	}
	if len(first.Requires) != 1 || first.Requires[0] != "tables/person" {
		t.Errorf("requires: %v", first.Requires)
	}
	if !strings.Contains(first.Script, "CREATE TABLE phone") {
		t.Errorf("script missing CREATE TABLE: %q", first.Script)
	}
	second := actions[1].Migration
	if second == nil || second.Name != "tables/phone/name" {
		t.Fatalf("second action: %+v", actions[1])
	}
	if len(second.Requires) != 1 || second.Requires[0] != "tables/phone" {
		t.Errorf("second requires: %v", second.Requires)
	}
	if !strings.Contains(second.Script, "ADD COLUMN name text") {
		t.Errorf("second script missing ALTER: %q", second.Script)
	}
}

func TestParsePersonFile(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "sql", "example-contacts", "tables", "person.sql"))
	if err != nil {
		t.Fatalf("read person.sql: %v", err)
	}
	actions, err := ParseActions("person.sql", string(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	m := actions[0].Migration
	if m == nil {
		t.Fatalf("expected migration")
	}
	if m.Name != "tables/person" {
		t.Errorf("name: %q", m.Name)
	}
	if len(m.Requires) != 1 || m.Requires[0] != "schema" {
		t.Errorf("requires: %v", m.Requires)
	}
}

func TestParseTestFile(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "sql", "example-contacts", "tests", "sample-person.sql"))
	if err != nil {
		t.Fatalf("read sample-person.sql: %v", err)
	}
	actions, err := ParseActions("sample-person.sql", string(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	tt := actions[0].Test
	if tt == nil {
		t.Fatalf("expected test action, got migration")
	}
	if tt.Name != "samplePerson" {
		t.Errorf("name: %q", tt.Name)
	}
}

// ---------------------------------------------------------------------------
// Error cases: broken / malformed migration files
// ---------------------------------------------------------------------------

func TestParseActions_BrokenMigrations(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr string // substring expected in error message
	}{
		{
			name:    "no hash-bang prefix",
			input:   "SELECT 1; -- not a gollard file",
			wantErr: "expected '#!'",
		},
		{
			name:    "empty file",
			input:   "",
			wantErr: "expected '#!'",
		},
		{
			name:    "unknown keyword",
			input:   "#!foobar",
			wantErr: "expected 'migration' or 'test'",
		},
		{
			name:    "typo in migration keyword",
			input:   "#!migraton name: \"m\", description: \"d\";",
			wantErr: "expected 'migration' or 'test'",
		},
		{
			name:    "missing name field",
			input:   "#!migration description: \"d\";",
			wantErr: "name",
		},
		{
			name:    "missing description field",
			input:   "#!migration name: \"m\";",
			wantErr: "description",
		},
		{
			name:    "name field is a list",
			input:   "#!migration name: [\"m\"], description: \"d\";",
			wantErr: "name",
		},
		{
			name:    "description field is a list",
			input:   "#!migration name: \"m\", description: [\"d\"];",
			wantErr: "description",
		},
		{
			name:    "unquoted field value",
			input:   "#!migration name: foo, description: \"d\";",
			wantErr: "expected field value",
		},
		{
			name:    "unterminated string in name",
			input:   "#!migration name: \"unterminated",
			wantErr: "unterminated string",
		},
		{
			name:    "unterminated string in requires list",
			input:   "#!migration name: \"m\", description: \"d\", requires: [\"unterminated;",
			wantErr: "unterminated string",
		},
		{
			name:    "missing semicolon after header",
			input:   "#!migration name: \"m\", description: \"d\"",
			wantErr: "';'",
		},
		{
			name:    "missing closing bracket in requires list",
			input:   "#!migration name: \"m\", description: \"d\", requires: [\"a\";",
			wantErr: "']'",
		},
		{
			name:    "missing colon after field name",
			input:   "#!migration name \"m\", description: \"d\";",
			wantErr: "':'",
		},
		{
			name:    "test missing name",
			input:   "#!test description: \"d\";",
			wantErr: "name",
		},
		{
			name:    "test missing description",
			input:   "#!test name: \"t\";",
			wantErr: "description",
		},
		{
			name:    "second migration broken after valid first",
			input:   "#!migration name: \"first\", description: \"d\";\n#!migration description: \"d\";",
			wantErr: "name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseActions("test.sql", tc.input)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestParseError_IncludesFilename verifies that ParseError includes the filename.
func TestParseError_IncludesFilename(t *testing.T) {
	_, err := ParseActions("myfile.sql", "not a gollard file")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "myfile.sql") {
		t.Errorf("error %q should include filename", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Valid edge cases
// ---------------------------------------------------------------------------

// TestParseActions_HashBangOnly verifies that "#!" with nothing after it is
// treated as a valid but empty gollard file (0 actions, no error).
func TestParseActions_HashBangOnly(t *testing.T) {
	actions, err := ParseActions("t.sql", "#!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("expected 0 actions, got %d", len(actions))
	}
}

func TestParseActions_RequiresSingleString(t *testing.T) {
	src := `#!migration name: "child", description: "d", requires: "parent";`
	actions, err := ParseActions("t.sql", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := actions[0].Migration
	if len(m.Requires) != 1 || m.Requires[0] != "parent" {
		t.Errorf("requires: got %v", m.Requires)
	}
}

func TestParseActions_RequiresEmptyList(t *testing.T) {
	src := `#!migration name: "m", description: "d", requires: [];`
	actions, err := ParseActions("t.sql", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions[0].Migration.Requires) != 0 {
		t.Errorf("expected empty requires, got %v", actions[0].Migration.Requires)
	}
}

func TestParseActions_RequiresMultiple(t *testing.T) {
	src := `#!migration name: "m", description: "d", requires: ["a", "b", "c"];`
	actions, err := ParseActions("t.sql", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := actions[0].Migration.Requires
	want := []MigrationID{"a", "b", "c"}
	if !slices.Equal(got, want) {
		t.Errorf("requires: got %v want %v", got, want)
	}
}

func TestParseActions_MigrationFollowedByTest(t *testing.T) {
	src := "#!migration name: \"m\", description: \"d\";\nCREATE TABLE m(id int);\n" +
		"#!test name: \"t\", description: \"test desc\";\nSELECT 1;\n"
	actions, err := ParseActions("t.sql", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}
	if actions[0].Migration == nil {
		t.Error("first action should be migration")
	}
	if actions[1].Test == nil {
		t.Error("second action should be test")
	}
}

func TestParseActions_ManyMigrationsInFile(t *testing.T) {
	src := `#!migration name: "m1", description: "First.";
CREATE TABLE m1(id int);
#!migration name: "m2", description: "Second.", requires: ["m1"];
CREATE TABLE m2(id int);
#!migration name: "m3", description: "Third.", requires: ["m2"];
CREATE TABLE m3(id int);
#!migration name: "m4", description: "Fourth.", requires: ["m3"];
CREATE TABLE m4(id int);
#!migration name: "m5", description: "Fifth.", requires: ["m4"];
CREATE TABLE m5(id int);
`
	actions, err := ParseActions("many.sql", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 5 {
		t.Fatalf("expected 5 actions, got %d", len(actions))
	}
	for i, want := range []MigrationID{"m1", "m2", "m3", "m4", "m5"} {
		if actions[i].Migration == nil {
			t.Errorf("action %d: expected migration, got test", i)
			continue
		}
		if actions[i].Migration.Name != want {
			t.Errorf("action %d: name got %q want %q", i, actions[i].Migration.Name, want)
		}
	}
}

func TestParseActions_MixedMigrationsAndTests(t *testing.T) {
	src := `#!migration name: "setup", description: "Schema.";
CREATE SCHEMA app;
#!test name: "setupCheck", description: "Schema exists.";
SELECT schema_name FROM information_schema.schemata WHERE schema_name = 'app';
#!migration name: "users", description: "Users table.", requires: ["setup"];
CREATE TABLE app.users(id int);
#!test name: "usersCheck", description: "Users table exists.";
SELECT COUNT(*) FROM app.users;
`
	actions, err := ParseActions("mixed.sql", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 4 {
		t.Fatalf("expected 4 actions, got %d", len(actions))
	}
	if actions[0].Migration == nil || actions[0].Migration.Name != "setup" {
		t.Errorf("action 0: expected migration 'setup'")
	}
	if actions[1].Test == nil || actions[1].Test.Name != "setupCheck" {
		t.Errorf("action 1: expected test 'setupCheck'")
	}
	if actions[2].Migration == nil || actions[2].Migration.Name != "users" {
		t.Errorf("action 2: expected migration 'users'")
	}
	if actions[3].Test == nil || actions[3].Test.Name != "usersCheck" {
		t.Errorf("action 3: expected test 'usersCheck'")
	}
}

// TestParseActions_SQLCommentFormat verifies that the real-world format with
// leading "--" SQL comment markers is parsed correctly. The skipWS function
// treats '-' as whitespace, stripping comment leaders.
func TestParseActions_SQLCommentFormat(t *testing.T) {
	src := `-- #!migration
-- name: "schema",
-- description: "Primary schema.";
CREATE SCHEMA app;`
	actions, err := ParseActions("schema.sql", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 1 || actions[0].Migration == nil {
		t.Fatalf("expected 1 migration")
	}
	m := actions[0].Migration
	if m.Name != "schema" {
		t.Errorf("name: got %q want %q", m.Name, "schema")
	}
	if !strings.Contains(m.Script, "CREATE SCHEMA app") {
		t.Errorf("script %q should contain CREATE SCHEMA", m.Script)
	}
}

// TestParseActions_BlockComment verifies that @@...@@ comments between
// tokens are stripped by skipWS.
func TestParseActions_BlockComment(t *testing.T) {
	src := `#!migration name: @@inline block comment@@ "foo", description: "bar";`
	actions, err := ParseActions("t.sql", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 1 || actions[0].Migration.Name != "foo" {
		t.Errorf("expected migration named 'foo', got %+v", actions)
	}
}

// TestParseActions_ScriptCapture verifies that multi-statement SQL bodies
// are captured verbatim and reflected in the checksum.
func TestParseActions_ScriptCapture(t *testing.T) {
	src := "#!migration name: \"m\", description: \"d\";\nCREATE TABLE t(id INT);\nALTER TABLE t ADD COLUMN name TEXT;"
	actions, err := ParseActions("t.sql", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := actions[0].Migration
	if !strings.Contains(m.Script, "CREATE TABLE t") {
		t.Errorf("script %q missing first statement", m.Script)
	}
	if !strings.Contains(m.Script, "ALTER TABLE t") {
		t.Errorf("script %q missing second statement", m.Script)
	}
	if m.Checksum != HashScript(m.Script) {
		t.Error("Checksum must equal HashScript(Script)")
	}
}

// TestParseActions_BodyTrimming verifies that trailing whitespace and dash
// characters are stripped from migration bodies (mirroring Haskell behaviour).
func TestParseActions_BodyTrimming(t *testing.T) {
	src := "#!migration name: \"m\", description: \"d\";\nCREATE TABLE t(id int);\n\n---\n"
	actions, err := ParseActions("t.sql", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	script := actions[0].Migration.Script
	if strings.HasSuffix(script, "\n") || strings.HasSuffix(script, "-") || strings.HasSuffix(script, " ") {
		t.Errorf("migration body not trimmed; got %q", script)
	}
}

// TestParseActions_TestBodyNotTrimmed verifies that test bodies are NOT
// right-trimmed (unlike migration bodies).
func TestParseActions_TestBodyNotTrimmed(t *testing.T) {
	src := "#!test name: \"t\", description: \"d\";\nSELECT 1;\n\n"
	actions, err := ParseActions("t.sql", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := actions[0].Test.Script
	if !strings.HasSuffix(body, "\n") {
		t.Errorf("test body should preserve trailing newline; got %q", body)
	}
}

// TestParseActions_ChecksumDeterminism verifies that parsing the same source
// twice yields identical checksums.
func TestParseActions_ChecksumDeterminism(t *testing.T) {
	src := "#!migration name: \"m\", description: \"d\";\nCREATE TABLE t(id int);\n"
	a1, _ := ParseActions("t.sql", src)
	a2, _ := ParseActions("t.sql", src)
	if a1[0].Migration.Checksum != a2[0].Migration.Checksum {
		t.Error("checksum should be deterministic across calls")
	}
}

// TestParseActions_PgDollarQuotedBody verifies that PostgreSQL dollar-quoted
// PL/pgSQL blocks (DO $$ ... $$) in migration bodies are captured verbatim.
func TestParseActions_PgDollarQuotedBody(t *testing.T) {
	src := `#!migration name: "m", description: "d";
DO $$
DECLARE
    x INT;
BEGIN
    SELECT 1 INTO x;
END $$;
`
	actions, err := ParseActions("t.sql", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	script := actions[0].Migration.Script
	if !strings.Contains(script, "DO $$") {
		t.Errorf("script %q should contain DO $$", script)
	}
	if !strings.Contains(script, "END $$") {
		t.Errorf("script %q should contain END $$", script)
	}
}

// TestParseActions_BodyWithSQLComments verifies the behaviour of SQL comments
// inside a migration body. Because skipWS treats '-' as whitespace and is
// called after the header's ';', any leading "--" immediately following the
// semicolon are consumed before body collection begins. Comments that appear
// WITHIN the body (not at its very start) are preserved verbatim.
func TestParseActions_BodyWithSQLComments(t *testing.T) {
	src := `#!migration name: "m", description: "d";
-- This leading comment is stripped by skipWS (dashes are whitespace to the parser)
CREATE TABLE t (id INT);
-- This mid-body comment is preserved as-is
CREATE INDEX IF NOT EXISTS idx_t ON t(id);
`
	actions, err := ParseActions("t.sql", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	script := actions[0].Migration.Script
	// The very first "-- ..." after the semicolon is consumed by skipWS.
	if strings.HasPrefix(script, "--") {
		t.Errorf("leading -- should be stripped by skipWS, got script starting with: %q", script[:min(40, len(script))])
	}
	// SQL inside the body is captured.
	if !strings.Contains(script, "CREATE TABLE t") {
		t.Errorf("script %q should contain CREATE TABLE", script)
	}
	// Comments in the MIDDLE of the body survive.
	if !strings.Contains(script, "-- This mid-body comment") {
		t.Errorf("mid-body comment should be preserved in: %q", script)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestParseActions_ChecksumChangesWithBody verifies that editing the body
// changes the checksum — the core integrity guarantee of the tool.
func TestParseActions_ChecksumChangesWithBody(t *testing.T) {
	parse := func(body string) Digest {
		src := "#!migration name: \"m\", description: \"d\";\n" + body
		actions, err := ParseActions("t.sql", src)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		return actions[0].Migration.Checksum
	}
	if parse("SELECT 1;") == parse("SELECT 2;") {
		t.Error("different bodies should produce different checksums")
	}
}
