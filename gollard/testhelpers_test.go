package gollard

// mig builds a Migration with the given name and optional requires list.
func mig(name string, requires ...string) Migration {
	reqs := make([]MigrationID, len(requires))
	for i, r := range requires {
		reqs[i] = MigrationID(r)
	}
	return Migration{Name: MigrationID(name), Requires: reqs}
}

// migTable builds a MigrationTable from the provided migrations.
func migTable(ms ...Migration) MigrationTable {
	t := MigrationTable{}
	for _, m := range ms {
		t[m.Name] = m
	}
	return t
}

// migWithScript builds a Migration with a populated Checksum derived from script.
func migWithScript(name, script string) Migration {
	return Migration{
		Name:     MigrationID(name),
		Checksum: HashScript(script),
		Script:   script,
	}
}
