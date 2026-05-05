package gollard

import (
	"os"
	"path/filepath"
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
