package web

import (
	"crypto/sha256"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"github.com/govalues/decimal"
)

// Renderer parses and executes HTML templates.
type Renderer struct {
	mu        sync.Mutex
	templates map[string]*template.Template
	baseDir   string
	cssHash   string // short hash of style.css for cache busting
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

	// Compute a short hash of the CSS file for cache busting.
	if hash, err := computeCSSHash(filepath.Join(baseDir, "static", "css", "style.css")); err == nil {
		r.cssHash = hash
	}

	return r, nil
}

// computeCSSHash reads the CSS file and returns a short hex hash (first 8 chars).
func computeCSSHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)[:8], nil
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
		"cssHash": func() string {
			return r.cssHash
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
		"slice": func(elements ...interface{}) []interface{} {
			return elements
		},
		"contains": func(substr, s string) bool {
			return strings.Contains(s, substr)
		},
		"sign": func(s string) string {
			// Returns "positive", "negative", or "" for a decimal string.
			if s == "" || s == "0" || s == "0.00" {
				return ""
			}
			if len(s) > 0 && s[0] == '-' {
				return "negative"
			}
			return "positive"
		},
		"getSign": func(s string) int {
			// Returns 1 (positive), -1 (negative), or 0 (zero/empty) for a decimal string.
			if s == "" || s == "0" || s == "0.00" {
				return 0
			}
			if len(s) > 0 && s[0] == '-' {
				return -1
			}
			return 1
		},
		"formatFloatPct": func(v *float64) string {
			// Format a *float64 as a percentage string (2dp), or "—" if nil.
			if v == nil {
				return "—"
			}
			return fmt.Sprintf("%.2f%%", *v)
		},
		"formatFloatPctRaw": func(v *float64) string {
			// Format a *float64 as a raw number string (4dp) for heat class lookup, or "" if nil.
			if v == nil {
				return ""
			}
			return fmt.Sprintf("%.4f", *v)
		},
		"getHeatClassAbsolute": func(s string) string {
			// Returns a CSS class for absolute return coloring.
			// Positive ≥ 1% → heat-positive-strong, 0 < x < 1% → heat-positive
			// Negative ≤ -1% → heat-negative-strong, -1% < x < 0 → heat-negative
			// Near zero → heat-neutral
			if s == "" {
				return "heat-empty"
			}
			// Parse as float for comparison.
			var val float64
			fmt.Sscanf(s, "%f", &val)
			if val >= 1.0 {
				return "heat-positive-strong"
			} else if val > 0 {
				return "heat-positive"
			} else if val <= -1.0 {
				return "heat-negative-strong"
			} else if val < 0 {
				return "heat-negative"
			}
			return "heat-neutral"
		},
		"getHeatClassLowerBetter": func(s string) string {
			// Returns a CSS class for metrics where lower is better (drawdown, volatility).
			// Negates the value so higher → red, lower → green.
			if s == "" {
				return "heat-empty"
			}
			var val float64
			fmt.Sscanf(s, "%f", &val)
			// Negate so the same thresholds apply but inverted.
			val = -val
			if val >= 1.0 {
				return "heat-positive-strong"
			} else if val > 0 {
				return "heat-positive"
			} else if val <= -1.0 {
				return "heat-negative-strong"
			} else if val < 0 {
				return "heat-negative"
			}
			return "heat-neutral"
		},
		"getHeatClassDiff": func(s string) string {
			// Returns a CSS class for diff (relative) coloring.
			// Positive (outperformed) ≥ 1% → heat-outperform-strong, 0 < x < 1% → heat-outperform
			// Negative (underperformed) ≤ -1% → heat-underperform-strong, -1% < x < 0 → heat-underperform
			// Near zero → heat-even
			if s == "" {
				return "heat-empty"
			}
			var val float64
			fmt.Sscanf(s, "%f", &val)
			if val >= 1.0 {
				return "heat-outperform-strong"
			} else if val > 0 {
				return "heat-outperform"
			} else if val <= -1.0 {
				return "heat-underperform-strong"
			} else if val < 0 {
				return "heat-underperform"
			}
			return "heat-even"
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
		"safeJS": func(s string) template.JS {
			// Marks a string as safe JavaScript (e.g. pre-serialized JSON).
			return template.JS(s)
		},
		"weightPct": func(v interface{}) string {
			// Convert a decimal.Decimal fraction (0.0-1.0) to a percentage string (2dp).
			// Used for MergedHolding and WeightDifferenceHolding display.
			switch val := v.(type) {
			case decimal.Decimal:
				f, _ := val.Float64()
				return fmt.Sprintf("%.2f", f*100)
			case fmt.Stringer:
				d, err := decimal.Parse(val.String())
				if err != nil {
					return "0.00"
				}
				f, _ := d.Float64()
				return fmt.Sprintf("%.2f", f*100)
			default:
				return fmt.Sprintf("%.2f", v)
			}
		},
		"mapKeys": func(m map[string]float64) []string {
			// Return sorted keys of a map for template iteration.
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			return keys
		},
		"floatPct": func(v float64) string {
			// Format a float64 fraction (0.0-1.0) as a percentage string (2dp).
			return fmt.Sprintf("%.2f", v*100)
		},
		"float2": func(v float64) string {
			// Format a float64 value with 2 decimal places (no percentage conversion).
			// Used for display values like Sharpe ratio that are not percentages.
			return fmt.Sprintf("%.2f", v)
		},
		"allocationCompare": func(a, b map[string]float64) []AllocationCompareEntry {
			// Merge two allocation breakdown maps into sorted comparison entries.
			// Sorted by max(a, b) descending, then alphabetically for ties.
			catSet := make(map[string]bool)
			for k := range a {
				catSet[k] = true
			}
			for k := range b {
				catSet[k] = true
			}
			entries := make([]AllocationCompareEntry, 0, len(catSet))
			for cat := range catSet {
				va := a[cat]
				vb := b[cat]
				entries = append(entries, AllocationCompareEntry{
					Category: cat,
					ValueA:   va,
					ValueB:   vb,
				})
			}
			sort.Slice(entries, func(i, j int) bool {
				maxI := entries[i].ValueA
				if entries[i].ValueB > maxI {
					maxI = entries[i].ValueB
				}
				maxJ := entries[j].ValueA
				if entries[j].ValueB > maxJ {
					maxJ = entries[j].ValueB
				}
				if maxI != maxJ {
					return maxI > maxJ
				}
				return entries[i].Category < entries[j].Category
			})
			return entries
		},
		"fxRateDisplay": func(posCurrency, baseCurrency string, rate interface{}) string {
			// Returns "PAIR RATE" in market convention (e.g. "GBP/USD 1.3000").
			// If rate is nil or zero, returns "—".
			var d *decimal.Decimal
			switch v := rate.(type) {
			case *decimal.Decimal:
				d = v
			case decimal.Decimal:
				d = &v
			default:
				return "—"
			}
			if d == nil || d.Equal(decimal.Zero) {
				return "—"
			}
			display := position.ConventionFxRate(posCurrency, baseCurrency, d)
			if display == nil {
				return "—"
			}
			return fmt.Sprintf("%s %s", display.Pair, formatDecimal(display.Rate.String(), 4))
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
	// Execute by the base filename - e.g., "list.html"
	baseName := filepath.Base(name) + ".html"
	return t.ExecuteTemplate(w, baseName, data)
}

// AllocationCompareEntry holds a single row in a sorted allocation comparison
// (e.g. sector or country allocation between two portfolios).
// Values are fractions (0.0-1.0); use floatPct for display.
type AllocationCompareEntry struct {
	Category string
	ValueA   float64
	ValueB   float64
}

// FilterEncoder is implemented by filter structs that need their fields
// serialized into query parameters for URL preservation (e.g. pagination links).
type FilterEncoder interface {
	// QueryParams returns a URL fragment like "&key=val&key2=val2" or "" if empty.
	QueryParams() string
}

// PageData holds common data passed to all page templates.
type PageData struct {
	Title   string
	Flash   string // One-time message (e.g., "Portfolio created")
	Error   string // Form validation error
	CSSHash string // short hash of style.css for cache busting
}

// StaticHandler returns an HTTP handler that serves static files from the given directory.
func StaticHandler(staticDir string) http.Handler {
	fs := http.Dir(staticDir)
	return http.StripPrefix("/static/", http.FileServer(fs))
}

// formatDecimal parses a decimal string and formats it with the given number of
// decimal places and thousands separators. Negative values are prefixed with "-".
// e.g. formatDecimal("-1234567.8912", 2) → "-1,234,567.89"
//
//	formatDecimal("1.2345", 4) → "1.2345"
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
