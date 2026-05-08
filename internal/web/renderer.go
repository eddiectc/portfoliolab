package web

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/govalues/decimal"
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
		"formatMoney": func(v interface{}) string {
			// Format a decimal value as money: thousands separator, 2dp, no currency symbol.
			// e.g. 1234567.8912 → "1,234,567.89"
			switch val := v.(type) {
			case fmt.Stringer:
				return formatDecimal(val.String(), 2)
			case string:
				return formatDecimal(val, 2)
			default:
				return fmt.Sprintf("%.2f", val)
			}
		},
		"formatFX": func(v interface{}) string {
			// Format a decimal value as FX rate: 4dp.
			// e.g. 1.2345 → "1.2345"
			switch val := v.(type) {
			case fmt.Stringer:
				return formatDecimal(val.String(), 4)
			case string:
				return formatDecimal(val, 4)
			default:
				return fmt.Sprintf("%.4f", val)
			}
		},
		"formatPct": func(v interface{}) string {
			// Format a decimal value as percentage: 2dp.
			// e.g. 12.345 → "12.35"
			switch val := v.(type) {
			case fmt.Stringer:
				return formatDecimal(val.String(), 2)
			case string:
				return formatDecimal(val, 2)
			default:
				return fmt.Sprintf("%.2f", val)
			}
		},
		"rowClass": func(costBasis, realizedPnL, marketValue string) string {
			// Returns a CSS class for row-level P&L color accent.
			// Computes total P&L ratio = (realized_pnl + market_value) / |cost_basis|
			// "row-profit" if ≥ 5%, "row-loss" if ≤ -5%, "" otherwise.
			cb, err := decimal.Parse(costBasis)
			if err != nil || cb.Equal(decimal.Zero) {
				return ""
			}
			rp, err := decimal.Parse(realizedPnL)
			if err != nil {
				rp = decimal.Zero
			}
			var mv decimal.Decimal
			if marketValue != "" {
				mv, err = decimal.Parse(marketValue)
				if err != nil {
					mv = decimal.Zero
				}
			}
			totalPnL, _ := rp.Add(mv)
			absCost := cb.Abs()
			pct, _ := totalPnL.Quo(absCost)
			// pct is a ratio (e.g. 0.05 = 5%). Compare against ±0.05.
			threshold, _ := decimal.Parse("0.05")
			if !pct.Less(threshold) {
				return "row-profit"
			}
			negThreshold := threshold.Neg()
			if pct.Less(negThreshold) || pct.Equal(negThreshold) {
				return "row-loss"
			}
			return ""
		},
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
		"contains": func(substr, s string) bool {
			return strings.Contains(s, substr)
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

// formatDecimal parses a decimal string and formats it with the given number of
// decimal places and thousands separators. Negative values are prefixed with "-".
// e.g. formatDecimal("-1234567.8912", 2) → "-1,234,567.89"
//      formatDecimal("1.2345", 4) → "1.2345"
func formatDecimal(s string, decimals int) string {
	if s == "" {
		return ""
	}

	// Parse the decimal string manually.
	neg := false
	idx := 0
	if len(s) > 0 && s[0] == '-' {
		neg = true
		idx = 1
	}

	// Split into integer and fractional parts.
	intPart := s[idx:]
	fracPart := ""
	if dotIdx := strings.Index(intPart, "."); dotIdx >= 0 {
		fracPart = intPart[dotIdx+1:]
		intPart = intPart[:dotIdx]
	}

	// Pad or truncate fractional part to desired decimals.
	for len(fracPart) < decimals {
		fracPart += "0"
	}
	fracPart = fracPart[:decimals]

	// Add thousands separators to integer part.
	formatted := addThousandsSeparator(intPart)

	result := formatted
	if decimals > 0 {
		result += "." + fracPart
	}
	if neg {
		result = "-" + result
	}
	return result
}

// addThousandsSeparator inserts commas every 3 digits from the right.
// e.g. "1234567" → "1,234,567"
func addThousandsSeparator(s string) string {
	if len(s) <= 3 {
		return s
	}
	var result strings.Builder
	result.Grow(len(s) + (len(s)-1)/3)
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteByte(',')
		}
		result.WriteRune(r)
	}
	return result.String()
}
