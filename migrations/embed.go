// Package migrations embeds the versioned SQL schema migrations used by the
// application and by integration tests.
package migrations

import "embed"

// FS contains every versioned migration file in this directory.
//
//go:embed *.sql
var FS embed.FS
