package data

import (
	"database/sql"
	"fmt"
	"log/slog"

	_ "modernc.org/sqlite"
	_ "modernc.org/sqlite/lib"
)

// Open opens a SQLite database connection at the given path.
// It enables WAL mode and sets reasonable connection pool settings
// for a single-user workload.
func Open(path string, logger *slog.Logger) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database %s: %w", path, err)
	}

	// Single-user workload: one connection is enough
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// Enable WAL mode for better concurrent read performance.
	// (:memory: databases don't support WAL — they use "memory" journal mode.)
	if path != ":memory:" {
		if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
			db.Close()
			return nil, fmt.Errorf("enable WAL mode: %w", err)
		}
	}

	// Enable foreign key support
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// Verify journal mode
	var journalMode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		db.Close()
		return nil, fmt.Errorf("check journal mode: %w", err)
	}
	if path != ":memory:" && journalMode != "wal" {
		db.Close()
		return nil, fmt.Errorf("expected WAL mode, got %s", journalMode)
	}

	logger.Info("database opened", "path", path, "journal_mode", journalMode)

	return db, nil
}

// Close closes the database connection.
func Close(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return db.Close()
}
