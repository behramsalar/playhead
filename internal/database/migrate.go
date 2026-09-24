package database

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"playhead/migrations"
)

// migrate applies any embedded migration file (named "NNNN_description.sql")
// whose version isn't yet recorded in schema_migrations, in ascending order,
// each in its own transaction.
func migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}

	applied := make(map[int]bool)
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("reading schema_migrations: %w", err)
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("reading embedded migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		version, err := migrationVersion(entry.Name())
		if err != nil {
			return fmt.Errorf("migration %s: %w", entry.Name(), err)
		}
		if applied[version] {
			continue
		}

		contents, err := migrations.FS.ReadFile(entry.Name())
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", entry.Name(), err)
		}

		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(contents)); err != nil {
			tx.Rollback()
			return fmt.Errorf("applying migration %s: %w", entry.Name(), err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %s: %w", entry.Name(), err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// migrationVersion parses the leading numeric prefix of a migration
// filename, e.g. "0001_init.sql" -> 1.
func migrationVersion(name string) (int, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("filename %q missing '_' separator", name)
	}
	version, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("filename %q has non-numeric version prefix: %w", name, err)
	}
	return version, nil
}
