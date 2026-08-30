package wisdomtree

// React Flight payload decoder for the new WisdomTree site (2026 relaunch).
//
// The new site is a Next.js app: fund page data is embedded in the HTML as a
// sequence of self.__next_f.push([1,"<chunk>"]) string chunks (the React Server
// Components "Flight" wire format). Chunks continue one logical "row" at a time:
// a row starts with a hex id prefix (e.g. "79:"), followed by a JSON document;
// long rows may be split across several push calls, in which case the
// continuation chunks carry no prefix. Other row types exist on the page
// (module preload rows like "1e:I[...]", plain string rows, orphaned
// continuation fragments) — the decoder ignores everything that is not a
// section table or ranking section.

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// rankClassification values identifying ranking sections in the flight payload.
const (
	RankClassificationSector = "Sector"
	RankClassificationTheme  = "EU Thematic Bucket"
	RankClassificationFund   = "Fund"
)

var (
	// flightChunkRe matches the payload string of one self.__next_f.push call.
	flightChunkRe = regexp.MustCompile(`self\.__next_f\.push\(\[1,"((?:[^"\\]|\\.)*)"\]\)`)

	// flightRowPrefixRe matches the RSC row id prefix, e.g. "79:".
	flightRowPrefixRe = regexp.MustCompile(`^[0-9a-f]+:`)
)

// Flight is the decoded content of one fund page's flight payload: the section
// tables and ranking sections found across all chunks.
type Flight struct {
	tables   []*Table
	sections []*Section
}

// Table is a flight "section table": a small label/value grid with column
// headers. The second column header usually carries the data date
// (e.g. "As of 28/08/2026").
type Table struct {
	AriaLabel   string // e.g. "Net Asset Value Table"
	FirstColumn string // first column header, e.g. "Net Asset Value"
	AsOf        string // second column header, e.g. "As of 28/08/2026" (may not be a date)
	Rows        []KV   // string label/value rows, in page order
	asOfDate    *time.Time
}

// KV is one label/value cell pair from a table row. Rows whose label cell is
// an embedded component rather than plain text are not included.
type KV struct {
	Label string
	Value string
}

// Value returns the value of the first row with the given label, and whether
// the label was found.
func (t *Table) Value(label string) (string, bool) {
	for _, kv := range t.Rows {
		if kv.Label == label {
			return kv.Value, true
		}
	}
	return "", false
}

// AsOfDate parses the date from the table's second column header. ok is false
// when the header carries no parsable date (e.g. "Weight (%)").
func (t *Table) AsOfDate() (time.Time, bool) {
	if t.asOfDate == nil {
		return time.Time{}, false
	}
	return *t.asOfDate, true
}

// Section is a ranking component: Sector Breakdown, Theme Breakdown, or the
// embedded holdings ranking.
type Section struct {
	SectionID      string
	Title          string // e.g. "Sector Breakdown"
	Classification string // rankClassification of the entries, e.g. "Sector"
	Entries        []RankingEntry
}

// RankingEntry is one row of a section ranking.
type RankingEntry struct {
	Name   string // e.g. "Information Technology"
	Rank   int
	Weight float64 // fraction of fund, e.g. 0.584 = 58.4%
	Date   string  // ISO dt, e.g. "2026-08-27T00:00:00.000Z"
}

// DecodeFlight decodes all self.__next_f.push chunks in a raw page body.
// Tolerant by design: chunks or rows that fail to unescape or parse as JSON
// are skipped, row shapes that carry no table or ranking section are ignored,
// and a page with no flight data yields an empty Flight rather than an error.
// Callers look up the specific tables and sections they need; absence returns
// nil.
func DecodeFlight(pageBody string) *Flight {
	f := &Flight{}
	pending := ""
	flush := func() {
		if pending != "" {
			f.collect(pending)
		}
		pending = ""
	}
	for _, m := range flightChunkRe.FindAllStringSubmatch(pageBody, -1) {
		unescaped, ok := unescapeFlightString(m[1])
		if !ok {
			flush() // corrupted chunk: drop the in-progress row
			continue
		}
		if cut := flightRowPrefixRe.FindStringIndex(unescaped); cut != nil {
			flush()
			pending = unescaped[cut[1]:]
		} else if pending != "" {
			pending += unescaped
		}
		// else: orphaned continuation with no pending row — ignore
	}
	flush()
	return f
}

// unescapeFlightString unescapes a flight chunk's JS string body. Flight
// chunks use JSON-compatible escapes (\", \\, \n, \uXXXX), so the body is
// decoded as a JSON string. ok is false when the body is not a valid string
// (the caller drops the chunk).
func unescapeFlightString(s string) (string, bool) {
	var out string
	if err := json.Unmarshal([]byte("\""+s+"\""), &out); err != nil {
		return "", false
	}
	return out, true
}

// collect walks one assembled row's JSON document and extracts any section
// tables and ranking sections it contains. Rows that do not parse as JSON
// (module preload rows, plain string rows) are ignored.
func (f *Flight) collect(data string) {
	var tree any
	if err := json.Unmarshal([]byte(data), &tree); err != nil {
		return
	}
	f.walk(tree)
}

func (f *Flight) walk(node any) {
	switch v := node.(type) {
	case map[string]any:
		if tbl, ok := tableFrom(v); ok {
			f.tables = append(f.tables, tbl)
		}
		if sec, ok := sectionFrom(v); ok {
			f.sections = append(f.sections, sec)
		}
		for _, val := range v {
			f.walk(val)
		}
	case []any:
		for _, val := range v {
			f.walk(val)
		}
	}
}

// Table returns the first table whose ariaLabel or first-column name matches
// any of the given names, or nil when no table matches.
func (f *Flight) Table(names ...string) *Table {
	for _, t := range f.tables {
		for _, n := range names {
			if t.AriaLabel == n || t.FirstColumn == n {
				return t
			}
		}
	}
	return nil
}

// SectionByClassification returns the first ranking section whose entries
// carry the given rankClassification (see RankClassification* constants), or
// nil when no section matches.
func (f *Flight) SectionByClassification(classification string) *Section {
	for _, s := range f.sections {
		if s.Classification == classification {
			return s
		}
	}
	return nil
}

// tableFrom builds a Table from a flight object carrying "columns" and "rows".
func tableFrom(obj map[string]any) (*Table, bool) {
	cols, okCols := obj["columns"].([]any)
	rows, okRows := obj["rows"].([]any)
	if !okCols || !okRows {
		return nil, false
	}
	t := &Table{
		AriaLabel: stringField(obj, "ariaLabel"),
		AsOf:      asOfHeader(cols),
		Rows:      []KV{},
	}
	if len(cols) > 0 {
		if m, ok := cols[0].(map[string]any); ok {
			t.FirstColumn = stringField(m, "name")
		}
	}
	for _, r := range rows {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		label, okL := rm["0"].(string)
		value, okV := rm["1"].(string)
		if !okL || !okV {
			// Label or value is an embedded component (e.g. the Market
			// Capitalisation breakdown bars) — not usable as plain text.
			continue
		}
		t.Rows = append(t.Rows, KV{Label: label, Value: value})
	}
	if d, ok := parseAsOfDate(t.AsOf); ok {
		t.asOfDate = &d
	}
	return t, true
}

// asOfHeader returns the second column header of a table (usually the
// "As of <date>" date header), or "" when the table has fewer than two
// columns.
func asOfHeader(cols []any) string {
	if len(cols) < 2 {
		return ""
	}
	if m, ok := cols[1].(map[string]any); ok {
		return stringField(m, "name")
	}
	return ""
}

// sectionFrom builds a Section from a flight object carrying a "ranking"
// array of constituent entries.
func sectionFrom(obj map[string]any) (*Section, bool) {
	ranking, ok := obj["ranking"].([]any)
	if !ok {
		return nil, false
	}
	s := &Section{
		SectionID: stringField(obj, "sectionId"),
		Title:     stringField(obj, "sectionTitle"),
		Entries:   make([]RankingEntry, 0, len(ranking)),
	}
	for _, e := range ranking {
		em, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if s.Classification == "" {
			s.Classification = stringField(em, "rankClassification")
		}
		s.Entries = append(s.Entries, RankingEntry{
			Name:   stringField(em, "constituentName"),
			Rank:   intField(em, "constituentRanking"),
			Weight: floatField(em, "pctWeight"),
			Date:   stringField(em, "dt"),
		})
	}
	return s, true
}

// parseAsOfDate parses the date in a table's second column header. Two site
// formats appear: UCITS pages are zero-padded day-first
// ("As of 28/08/2026"), US pages are unpadded month-first
// ("As of 8/27/2026"). When neither part exceeds 12 the order is ambiguous;
// it is resolved by padding (zero-padded → day-first, unpadded →
// month-first), which matches how the site renders each region.
func parseAsOfDate(header string) (time.Time, bool) {
	s := strings.TrimPrefix(header, "As of ")
	parts := strings.Split(s, "/")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	a, errA := strconv.Atoi(parts[0])
	b, errB := strconv.Atoi(parts[1])
	c, errC := strconv.Atoi(parts[2])
	if errA != nil || errB != nil || errC != nil || a < 1 || b < 1 || c < 1 {
		return time.Time{}, false
	}
	var month, day int
	switch {
	case a > 12:
		day, month = a, b
	case b > 12:
		month, day = a, b
	case len(parts[0]) == 2 && len(parts[1]) == 2:
		day, month = a, b
	default:
		month, day = a, b
	}
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, false
	}
	return time.Date(c, time.Month(month), day, 0, 0, 0, 0, time.UTC), true
}

func stringField(m map[string]any, key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

func intField(m map[string]any, key string) int {
	if f, ok := m[key].(float64); ok {
		return int(f)
	}
	return 0
}

func floatField(m map[string]any, key string) float64 {
	if f, ok := m[key].(float64); ok {
		return f
	}
	return 0
}
