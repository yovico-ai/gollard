package gollard

import (
	"crypto/sha256"
	"fmt"
)

type MigrationID string

type Digest [sha256.Size]byte

func HashScript(script string) Digest {
	return sha256.Sum256([]byte(script))
}

type Migration struct {
	Name        MigrationID
	Description string
	Requires    []MigrationID
	Checksum    Digest
	Script      string
}

type TestID string

type Test struct {
	Name        TestID
	Description string
	Script      string
}

type MigrationTable map[MigrationID]Migration
type TestTable map[TestID]Test

type MigrationMissingFromTableError struct {
	ID MigrationID
}

func (e *MigrationMissingFromTableError) Error() string {
	return fmt.Sprintf("migration missing from table: %s", e.ID)
}

func InflateMigrationIDs(table MigrationTable, ids []MigrationID) ([]Migration, error) {
	out := make([]Migration, 0, len(ids))
	for _, id := range ids {
		m, ok := table[id]
		if !ok {
			return nil, &MigrationMissingFromTableError{ID: id}
		}
		out = append(out, m)
	}
	return out, nil
}
