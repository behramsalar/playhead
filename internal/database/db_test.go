package database_test

import (
	"path/filepath"
	"testing"

	"playhead/internal/database"
)

func TestOpenAppliesMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")

	db, err := database.Open(path)
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer db.Close()

	var name string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'videos'`).Scan(&name); err != nil {
		t.Fatalf("videos table missing after Open: %v", err)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")

	db1, err := database.Open(path)
	if err != nil {
		t.Fatalf("first Open error: %v", err)
	}
	db1.Close()

	// Reopening an already-migrated database must not fail or reapply
	// migrations (which would error on CREATE TABLE of an existing table).
	db2, err := database.Open(path)
	if err != nil {
		t.Fatalf("second Open error: %v", err)
	}
	defer db2.Close()

	var afterFirst int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&afterFirst); err != nil {
		t.Fatalf("querying schema_migrations: %v", err)
	}
	if afterFirst == 0 {
		t.Fatal("expected at least one migration to be recorded")
	}

	db2.Close()
	db3, err := database.Open(path)
	if err != nil {
		t.Fatalf("third Open error: %v", err)
	}
	defer db3.Close()

	var afterThird int
	if err := db3.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&afterThird); err != nil {
		t.Fatalf("querying schema_migrations: %v", err)
	}
	if afterThird != afterFirst {
		t.Fatalf("schema_migrations count changed across idempotent opens: %d -> %d", afterFirst, afterThird)
	}
}
