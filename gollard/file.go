package gollard

import (
	"io/fs"
	"os"
	"path/filepath"
)

// ImportDirectory walks every file under root, parses each one, and returns
// the combined migration and test tables.
func ImportDirectory(root string) (MigrationTable, TestTable, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, err
	}
	migrations := MigrationTable{}
	tests := TestTable{}
	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".sql" {
			return nil
		}
		if err := importFileInto(path, migrations, tests); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return migrations, tests, nil
}

// ImportFile parses a single file into its own migration/test tables.
func ImportFile(path string) (MigrationTable, TestTable, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, err
	}
	migrations := MigrationTable{}
	tests := TestTable{}
	if err := importFileInto(abs, migrations, tests); err != nil {
		return nil, nil, err
	}
	return migrations, tests, nil
}

func importFileInto(path string, migrations MigrationTable, tests TestTable) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	actions, err := ParseActions(path, string(data))
	if err != nil {
		return err
	}
	actions = adjustDependencies(actions)
	for _, a := range actions {
		switch {
		case a.Migration != nil:
			migrations[a.Migration.Name] = *a.Migration
		case a.Test != nil:
			tests[a.Test.Name] = *a.Test
		}
	}
	return nil
}

// adjustDependencies mirrors File.hs:adjustDependencies from the original
// Haskell mallard. Consecutive migrations within a file implicitly require the
// preceding migration unless that dependency is already declared explicitly.
// Tests are transparent — they do not interrupt or reset the chain.
func adjustDependencies(actions []Action) []Action {
	result := make([]Action, len(actions))
	var prev MigrationID
	hasPrev := false
	for i, a := range actions {
		if a.Test != nil {
			result[i] = a
			continue
		}
		m := *a.Migration
		if !hasPrev {
			hasPrev = true
			prev = m.Name
			result[i] = a
			continue
		}
		alreadyRequired := false
		for _, req := range m.Requires {
			if req == prev {
				alreadyRequired = true
				break
			}
		}
		if !alreadyRequired {
			newReqs := make([]MigrationID, 0, len(m.Requires)+1)
			newReqs = append(newReqs, prev)
			newReqs = append(newReqs, m.Requires...)
			m.Requires = newReqs
		}
		result[i] = Action{Migration: &m}
		prev = m.Name
	}
	return result
}
