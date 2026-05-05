package gollard

import "fmt"

type AppliedMigrationMissingError struct {
	ID MigrationID
}

func (e *AppliedMigrationMissingError) Error() string {
	return fmt.Sprintf("applied migration %q is missing from the current migration plan", e.ID)
}

type DigestMismatchError struct {
	ID              MigrationID
	PlannedChecksum Digest
	AppliedChecksum Digest
}

func (e *DigestMismatchError) Error() string {
	return fmt.Sprintf("checksum mismatch for migration %q: planned=%x applied=%x", e.ID, e.PlannedChecksum, e.AppliedChecksum)
}

// ValidateAppliedMigrations verifies that every applied migration exists in
// the planned set and that their checksums match. Returns the first error.
func ValidateAppliedMigrations(planned, applied MigrationTable) error {
	for _, a := range applied {
		if err := ValidateAppliedMigration(planned, a); err != nil {
			return err
		}
	}
	return nil
}

func ValidateAppliedMigration(planned MigrationTable, applied Migration) error {
	p, ok := planned[applied.Name]
	if !ok {
		return &AppliedMigrationMissingError{ID: applied.Name}
	}
	if p.Checksum != applied.Checksum {
		return &DigestMismatchError{
			ID:              applied.Name,
			PlannedChecksum: p.Checksum,
			AppliedChecksum: applied.Checksum,
		}
	}
	return nil
}

type DigestComparison struct {
	ID              MigrationID
	PlannedChecksum Digest
	AppliedChecksum Digest
	Match           bool
}

// CheckAppliedMigrations is like ValidateAppliedMigrations but returns one
// comparison per applied migration instead of stopping at the first mismatch.
// It still returns an error if an applied migration is missing from the plan.
func CheckAppliedMigrations(planned, applied MigrationTable) ([]DigestComparison, error) {
	out := make([]DigestComparison, 0, len(applied))
	for _, a := range applied {
		c, err := CheckAppliedMigration(planned, a)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func CheckAppliedMigration(planned MigrationTable, applied Migration) (DigestComparison, error) {
	p, ok := planned[applied.Name]
	if !ok {
		return DigestComparison{}, &AppliedMigrationMissingError{ID: applied.Name}
	}
	return DigestComparison{
		ID:              applied.Name,
		PlannedChecksum: p.Checksum,
		AppliedChecksum: applied.Checksum,
		Match:           p.Checksum == applied.Checksum,
	}, nil
}
