package web

import (
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Renderer parses and executes HTML templates.
type Renderer struct {
	mu        sync.Mutex
	templates map[string]*template.Template
	baseDir   string
}

// NewRenderer creates a new Renderer and parses all templates from the given directory.
func NewRenderer(baseDir string) (*Renderer, error) {
	r := &Renderer{
		templates: make(map[string]*template.Template),
		baseDir:   baseDir,
	}

	if err := r.parseTemplates(); err != nil {
		return nil, err
	}

	return r, nil
}

// parseTemplates walks the templates directory and parses each page template
// with the shared base and partials.
func (r *Renderer) parseTemplates() error {
	funcMap := template.FuncMap{
		"eq": func(a, b interface{}) bool {
			switch a := a.(type) {
			case int:
				if b, ok := b.(int); ok {
					return a == b
				}
			case int64:
				if b, ok := b.(int64); ok {
					return a == b
				}
			case string:
				if b, ok := b.(string); ok {
					return a == b
				}
			}
			return false
		},
		"add": func(a, b int) int {
			return a + b
		},
		"queryPreserve": func(filter interface{}) template.HTMLAttr {
			// Returns filter query params preserved for pagination links.
			// Returns template.HTML to prevent double-escaping of & in hrefs.
			// Expects the filter to implement FilterEncoder.
			if enc, ok := filter.(FilterEncoder); ok {
				if params := enc.QueryParams(); params != "" {
					return template.HTMLAttr(params)
				}
			}
			return ""
		},
	}

	// Collect layout files (base + partials) and page files separately
	var layoutFiles []string
	var pageFiles []string

	err := filepath.Walk(r.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".html") {
			return nil
		}

		rel, _ := filepath.Rel(r.baseDir, path)
		// Layout files: base.html and anything in partials/
		if rel == "base.html" || strings.HasPrefix(rel, "partials"+string(filepath.Separator)) {
			layoutFiles = append(layoutFiles, path)
		} else {
			pageFiles = append(pageFiles, path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Parse layout templates (base + partials)
	var layout *template.Template
	if len(layoutFiles) > 0 {
		layout, err = template.New("").Funcs(funcMap).ParseFiles(layoutFiles...)
		if err != nil {
			return err
		}
	}

	// For each page, parse layout + page together
	for _, pageFile := range pageFiles {
		name := r.templateName(pageFile)

		// Parse layout + this page file together
		files := append(layoutFiles, pageFile)
		t, err := template.New("").Funcs(funcMap).ParseFiles(files...)
		if err != nil {
			return err
		}

		r.templates[name] = t
	}

	// If no page files but layout exists, store layout for direct use
	if len(pageFiles) == 0 && layout != nil {
		r.templates["base"] = layout
	}

	slog.Info("templates loaded", "count", len(r.templates), "dir", r.baseDir)
	return nil
}

// templateName extracts the template name from a file path.
// e.g., "templates/portfolio/list.html" -> "portfolio/list"
func (r *Renderer) templateName(path string) string {
	rel, _ := filepath.Rel(r.baseDir, path)
	rel = filepath.ToSlash(rel)
	return strings.TrimSuffix(rel, ".html")
}

// Render executes the named page template with the given data.
// Each page template defines "content" and includes {{template "base" .}}.
func (r *Renderer) Render(w http.ResponseWriter, name string, data interface{}) error {
	r.mu.Lock()
	t, ok := r.templates[name]
	r.mu.Unlock()

	if !ok {
		http.Error(w, "template not found: "+name, http.StatusInternalServerError)
		return nil
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Execute by the base filename — e.g., "list.html"
	baseName := filepath.Base(name) + ".html"
	return t.ExecuteTemplate(w, baseName, data)
}

// FilterEncoder is implemented by filter structs that need their fields
// serialized into query parameters for URL preservation (e.g. pagination links).
type FilterEncoder interface {
	// QueryParams returns a URL fragment like "&key=val&key2=val2" or "" if empty.
	QueryParams() string
}

// PageData holds common data passed to all page templates.
type PageData struct {
	Title    string
	Flash    string // One-time message (e.g., "Portfolio created")
	Error    string // Form validation error
}

// StaticHandler returns an HTTP handler that serves static files from the given directory.
func StaticHandler(staticDir string) http.Handler {
	fs := http.Dir(staticDir)
	return http.StripPrefix("/static/", http.FileServer(fs))
}
