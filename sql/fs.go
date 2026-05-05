// Package gollardsql exposes the embedded SQL files that ship with gollard.
//
// MallardFS contains the bootstrap migrations for the internal `mallard`
// schema (mallard.migrator_version + mallard.applied_migrations).
package gollardsql

import "embed"

//go:embed mallard/*.sql
var MallardFS embed.FS
