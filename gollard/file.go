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
