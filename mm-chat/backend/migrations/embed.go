// Package migrations exposes SQL migration files embedded into Go binaries.
package migrations

import "embed"

// FS contains all SQL migration files in this directory.
//
//go:embed *.sql
var FS embed.FS
