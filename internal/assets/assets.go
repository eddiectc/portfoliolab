// Package assets embeds all runtime assets (database migrations, HTML
// templates, static files) into the binary so deployment only requires
// the binary itself.
package assets

import (
	"embed"
	"io/fs"
)

//go:embed all:migrations
var migrationsFS embed.FS

//go:embed all:templates
var templatesFS embed.FS

//go:embed all:static
var staticFS embed.FS

// Migrations is the embedded migrations/ directory.
var Migrations = mustSub(migrationsFS, "migrations")

// Templates is the embedded templates/ directory.
var Templates = mustSub(templatesFS, "templates")

// Static is the embedded static/ directory (css, js).
var Static = mustSub(staticFS, "static")

func mustSub(fsys embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
