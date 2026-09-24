// Package database opens the SQLite index/cache and applies migrations.
//
// The database is disposable by design: deleting the file and restarting
// recreates an empty schema, and the indexer repopulates it from the
// filesystem without changing any video ID.
package database

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go driver, keeps the binary CGO-free
)

// Open opens (creating if needed) the SQLite database at path and applies
// any pending migrations.
func Open(path string) (*sql.DB, error) {
	// WAL mode lets the indexer write while browse requests read
	// concurrently; busy_timeout avoids "database is locked" errors
	// under brief write contention instead of failing requests outright.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", path)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	// SQLite handles one writer at a time; a single shared connection
	// avoids SQLITE_BUSY from this process's own goroutines racing each
	// other, while WAL still allows readers to proceed concurrently.
	db.SetMaxOpenConns(1)

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying migrations: %w", err)
	}

	return db, nil
}
