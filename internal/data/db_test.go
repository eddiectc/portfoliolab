package data

import (
	"log/slog"
	"testing"
)

func TestOpen_InMemory(t *testing.T) {
	logger := slog.Default()

	db, err := Open("file:test_inmem::memory:?cache=shared", logger)
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer Close(db)

	// Verify we can execute a query
	var result int
	err = db.QueryRow("SELECT 1").Scan(&result)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if result != 1 {
		t.Errorf("expected 1, got %d", result)
	}
}

func TestOpen_WALMode(t *testing.T) {
	logger := slog.Default()

	db, err := Open("file:test_wal_mode::memory:?cache=shared", logger)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer Close(db)

	var journalMode string
	err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("check journal mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("expected WAL mode, got %s", journalMode)
	}
}

func TestOpen_ForeignKeys(t *testing.T) {
	logger := slog.Default()

	db, err := Open("file:test_fk::memory:?cache=shared", logger)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer Close(db)

	var fkEnabled int
	err = db.QueryRow("PRAGMA foreign_keys").Scan(&fkEnabled)
	if err != nil {
		t.Fatalf("check foreign keys: %v", err)
	}
	if fkEnabled != 1 {
		t.Errorf("expected foreign keys enabled, got %d", fkEnabled)
	}
}

func TestClose_NilDB(t *testing.T) {
	// Should not panic
	err := Close(nil)
	if err != nil {
		t.Errorf("expected no error for nil DB, got %v", err)
	}
}
