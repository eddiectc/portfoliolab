package data

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"

	_ "modernc.org/sqlite"

	"github.com/pressly/goose/v3"
)

// MigrateUp runs all pending database migrations found in the given FS.
// The FS is expected to expose the migration files at its root (e.g. the
// embedded internal/assets.Migrations).
func MigrateUp(db *sql.DB, migrationsFS fs.FS, logger *slog.Logger) error {
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	goose.SetBaseFS(migrationsFS)

	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("run migrations up: %w", err)
	}

	logger.Info("migrations applied successfully")
	return nil
}
