// Package migrations embeds the SQL migration files so the Go binary can
// apply them without external files, keeping the SQLite index disposable
// and reproducible from a fresh checkout.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
