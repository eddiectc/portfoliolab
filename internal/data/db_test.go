package data

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestOpen_InMemory(t *testing.T) {
	logger := slog.Default()

	db, err := Open(":memory:", logger)
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer func() { _ = Close(db) }()

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

	// WAL mode is not supported on :memory: databases; use a temp file.
	tmpDir := t.TempDir()
	tmpPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(tmpPath, logger)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = Close(db) }()
	defer func() { _ = os.Remove(tmpPath) }()

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

	db, err := Open(":memory:", logger)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = Close(db) }()

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

func TestCleanWALFiles(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create the main DB file
	f, err := os.Create(dbPath)
	if err != nil {
		t.Fatalf("create db file: %v", err)
	}
	_ = f.Close()

	// Create empty WAL and SHM files (simulating a crash)
	walPath := dbPath + "-wal"
	shmPath := dbPath + "-shm"
	_, _ = os.Create(walPath)
	_, _ = os.Create(shmPath)

	// Verify they exist
	if _, err := os.Stat(walPath); os.IsNotExist(err) {
		t.Fatal("WAL file should exist before cleanup")
	}
	if _, err := os.Stat(shmPath); os.IsNotExist(err) {
		t.Fatal("SHM file should exist before cleanup")
	}

	// Clean up
	cleanWALFiles(dbPath)

	// Verify they're gone
	if _, err := os.Stat(walPath); !os.IsNotExist(err) {
		t.Error("WAL file should be removed after cleanup")
	}
	if _, err := os.Stat(shmPath); !os.IsNotExist(err) {
		t.Error("SHM file should be removed after cleanup")
	}
}

func TestCleanWALFiles_NonEmptyKept(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create main DB and non-empty WAL
	_, _ = os.Create(dbPath)
	f, _ := os.Create(dbPath + "-wal")
	_, _ = f.Write([]byte("data"))
	_ = f.Close()

	// Non-empty WAL should NOT be removed
	cleanWALFiles(dbPath)

	if _, err := os.Stat(dbPath + "-wal"); os.IsNotExist(err) {
		t.Error("non-empty WAL file should be kept")
	}
}

func TestOpen_CreatesDirectory(t *testing.T) {
	logger := slog.Default()

	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "sub", "dir", "test.db")

	db, err := Open(nestedPath, logger)
	if err != nil {
		t.Fatalf("open db with nested path: %v", err)
	}
	defer func() { _ = Close(db) }()

	// Verify the directory was created
	if _, err := os.Stat(filepath.Dir(nestedPath)); os.IsNotExist(err) {
		t.Error("directory should have been created")
	}
}
