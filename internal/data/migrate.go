package data

import (
	"database/sql"
	"fmt"
	"log/slog"

	_ "modernc.org/sqlite"

	"github.com/pressly/goose/v3"
)

// MigrateUp runs all pending database migrations from the given directory.
func MigrateUp(db *sql.DB, migrationsDir string, logger *slog.Logger) error {
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	if err := goose.Up(db, migrationsDir); err != nil {
		return fmt.Errorf("run migrations up: %w", err)
	}

	logger.Info("migrations applied successfully")
	return nil
}
