package wisdomtree

import (
	"testing"
	"time"
)

// TestDecodeFlight_QGRW_Tables decodes the UCITS QGRW page fixture: section
// tables found by ariaLabel and by first-column name, both UCITS date formats
// on one page, disambiguation of two tables sharing a first column name, and
// missing tables/labels.
func TestDecodeFlight_QGRW_Tables(t *testing.T) {
	f := DecodeFlight(loadFixture(t, "flight_qgrw_tables.html"))

	t.Run("overview table by ariaLabel", func(t *testing.T) {
		ov := f.Table("Overview Table")
		if ov == nil {
			t.Fatal("Overview Table not found")
		}
		if got, _ := ov.Value("ISIN"); got != "IE000YGEAK03" {
			t.Errorf("ISIN = %q, want IE000YGEAK03", got)
		}
		if got, _ := ov.Value("Inception Date"); got != "16 April 2024" {
			t.Errorf("Inception Date = %q, want %q", got, "16 April 2024")
		}
		if got, _ := ov.Value("Base Currency"); got != "USD" {
			t.Errorf("Base Currency = %q, want USD", got)
		}
	})

	t.Run("table by first-column name", func(t *testing.T) {
		ov := f.Table("Product Overview")
		if ov == nil {
			t.Fatal("Product Overview table not found by first-column name")
		}
		if ov.AriaLabel != "Overview Table" {
			t.Errorf("AriaLabel = %q, want %q", ov.AriaLabel, "Overview Table")
		}
	})

	t.Run("shared first-column name resolves to first table", func(t *testing.T) {
		// Both "Country Allocation Table" and "Listings and Codes Table"
		// have a first column named "Country"; the allocation table appears
		// earlier in the page and must win.
		ca := f.Table("Country")
		if ca == nil {
			t.Fatal("Country table not found")
		}
		if got, ok := ca.Value("United States"); !ok || got != "99.54%" {
			t.Errorf("United States = %q (ok=%v), want 99.54%% (Country Allocation, not Listings)", got, ok)
		}
	})

	t.Run("nav and fees values", func(t *testing.T) {
		nav := f.Table("Net Asset Value")
		if nav == nil {
			t.Fatal("Net Asset Value table not found")
		}
		if got, _ := nav.Value("NAV"); got != "US$42.974" {
			t.Errorf("NAV = %q, want US$42.974", got)
		}
		if got, _ := nav.Value("Total AUM of fund"); got != "US$47,442,965" {
			t.Errorf("Total AUM of fund = %q, want US$47,442,965", got)
		}
		if got, ok := nav.Value("Unknown Label"); ok || got != "" {
			t.Errorf("unknown label: got %q ok=%v, want zero value", got, ok)
		}

		fees := f.Table("Fees Table")
		if fees == nil {
			t.Fatal("Fees Table not found")
		}
		if got, _ := fees.Value("Total expense ratio (TER)"); got != "0.33%" {
			t.Errorf("TER = %q, want 0.33%%", got)
		}
	})

	t.Run("UCITS as-of dates", func(t *testing.T) {
		nav := f.Table("Net Asset Value Table")
		if nav == nil {
			t.Fatal("Net Asset Value Table not found")
		}
		d, ok := nav.AsOfDate()
		if !ok {
			t.Fatalf("NAV AsOfDate not parsed, header %q", nav.AsOf)
		}
		if want := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC); !d.Equal(want) {
			t.Errorf("NAV AsOfDate = %v, want %v", d, want)
		}

		mc := f.Table("Market Capitalisation Table")
		if mc == nil {
			t.Fatal("Market Capitalisation Table not found")
		}
		d, ok = mc.AsOfDate()
		if !ok {
			t.Fatalf("Market Capitalisation AsOfDate not parsed, header %q", mc.AsOf)
		}
		if want := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC); !d.Equal(want) {
			t.Errorf("Market Cap AsOfDate = %v, want %v", d, want)
		}
		if got, _ := mc.Value("Total Market Capitalisation ($ Trillion)"); got != "34.81" {
			t.Errorf("Total Market Capitalisation = %q, want 34.81", got)
		}
	})

	t.Run("non-date second column", func(t *testing.T) {
		ca := f.Table("Country Allocation Table")
		if ca == nil {
			t.Fatal("Country Allocation Table not found")
		}
		if ca.AsOf != "Weight (%)" {
			t.Errorf("AsOf = %q, want %q", ca.AsOf, "Weight (%)")
		}
		if _, ok := ca.AsOfDate(); ok {
			t.Error("AsOfDate should not parse for non-date header")
		}
	})

	t.Run("missing table", func(t *testing.T) {
		if f.Table("Hypothetical Table") != nil {
			t.Error("expected nil for absent table")
		}
	})
}

// TestDecodeFlight_EZM_Tables decodes the US EZM page fixture: US date format,
// US-specific tables, tables absent from US pages, and the embedded holdings
// section.
func TestDecodeFlight_EZM_Tables(t *testing.T) {
	f := DecodeFlight(loadFixture(t, "flight_ezm_tables.html"))

	ov := f.Table("Product Overview")
	if ov == nil {
		t.Fatal("Product Overview table not found")
	}
	if got, _ := ov.Value("CUSIP"); got != "97717W570" {
		t.Errorf("CUSIP = %q, want 97717W570", got)
	}
	if got, _ := ov.Value("Expense Ratio"); got != "0.38%" {
		t.Errorf("Expense Ratio = %q, want 0.38%%", got)
	}
	if got, _ := ov.Value("Inception Date"); got != "2/23/2007" {
		t.Errorf("Inception Date = %q, want 2/23/2007", got)
	}

	t.Run("US as-of dates", func(t *testing.T) {
		d, ok := ov.AsOfDate()
		if !ok {
			t.Fatalf("Overview AsOfDate not parsed, header %q", ov.AsOf)
		}
		if want := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC); !d.Equal(want) {
			t.Errorf("Overview AsOfDate = %v, want %v", d, want)
		}

		mc := f.Table("Market Capitalization")
		if mc == nil {
			t.Fatal("Market Capitalization table not found")
		}
		d, ok = mc.AsOfDate()
		if !ok {
			t.Fatalf("Market Capitalization AsOfDate not parsed, header %q", mc.AsOf)
		}
		if want := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC); !d.Equal(want) {
			t.Errorf("Market Capitalization AsOfDate = %v, want %v", d, want)
		}
	})

	t.Run("US-specific table", func(t *testing.T) {
		ti := f.Table("Trading Information")
		if ti == nil {
			t.Fatal("Trading Information table not found")
		}
		if got, _ := ti.Value("Lead Market Maker"); got != "Susquehanna" {
			t.Errorf("Lead Market Maker = %q, want Susquehanna", got)
		}
	})

	t.Run("site value quirks are preserved verbatim", func(t *testing.T) {
		// The site literally renders a doubled dollar sign in places; the
		// decoder must not "fix" or mangle site strings.
		nav := f.Table("Net Asset Value")
		if nav == nil {
			t.Fatal("Net Asset Value table not found")
		}
		if got, _ := nav.Value("NAV"); got != "$$76.033" {
			t.Errorf("NAV = %q, want $$76.033 (verbatim)", got)
		}
		if got, _ := ov.Value("Total Assets (000)"); got != "$$946,609.72" {
			t.Errorf("Total Assets (000) = %q, want $$946,609.72 (verbatim)", got)
		}
	})

	t.Run("tables absent from US pages", func(t *testing.T) {
		if f.Table("Fees Table") != nil {
			t.Error("Fees Table should be absent from US pages")
		}
		if f.Table("Fees") != nil {
			t.Error("Fees table should be absent from US pages")
		}
	})

	t.Run("embedded holdings section", func(t *testing.T) {
		s := f.SectionByClassification(RankClassificationFund)
		if s == nil {
			t.Fatal("Fund holdings section not found")
		}
		if len(s.Entries) != 100 {
			t.Errorf("entries = %d, want 100", len(s.Entries))
		}
	})
}

// TestDecodeFlight_Sections decodes the sector and theme section fixtures for
// both regions, and checks that an absent classification returns nil.
func TestDecodeFlight_Sections(t *testing.T) {
	t.Run("UCITS sector", func(t *testing.T) {
		f := DecodeFlight(loadFixture(t, "flight_qgrw_sector.html"))
		s := f.SectionByClassification(RankClassificationSector)
		if s == nil {
			t.Fatal("Sector section not found")
		}
		if s.SectionID != "sector-breakdown" || s.Title != "Sector Breakdown" {
			t.Errorf("section = %s/%s, want sector-breakdown/Sector Breakdown", s.SectionID, s.Title)
		}
		if len(s.Entries) != 9 {
			t.Fatalf("entries = %d, want 9", len(s.Entries))
		}
		first := s.Entries[0]
		if first.Name != "Information Technology" || first.Rank != 1 {
			t.Errorf("first entry = %+v, want Information Technology #1", first)
		}
		if first.Weight != 0.584027 {
			t.Errorf("first weight = %v, want 0.584027", first.Weight)
		}
		if first.Date != "2026-08-27T00:00:00.000Z" {
			t.Errorf("first date = %q, want 2026-08-27T00:00:00.000Z", first.Date)
		}
		if f.SectionByClassification(RankClassificationTheme) != nil {
			t.Error("theme section should be absent from a sector fixture")
		}
	})

	t.Run("US sector", func(t *testing.T) {
		f := DecodeFlight(loadFixture(t, "flight_ezm_sector.html"))
		s := f.SectionByClassification(RankClassificationSector)
		if s == nil {
			t.Fatal("Sector section not found")
		}
		if len(s.Entries) != 12 {
			t.Fatalf("entries = %d, want 12", len(s.Entries))
		}
		first := s.Entries[0]
		if first.Name != "Financials" || first.Weight != 0.19322 {
			t.Errorf("first entry = %+v, want Financials 0.19322", first)
		}
		if first.Date != "2026-08-28T00:00:00.000Z" {
			t.Errorf("first date = %q, want 2026-08-28T00:00:00.000Z", first.Date)
		}
	})

	t.Run("theme", func(t *testing.T) {
		f := DecodeFlight(loadFixture(t, "flight_wmgt_theme.html"))
		s := f.SectionByClassification(RankClassificationTheme)
		if s == nil {
			t.Fatal("Theme section not found")
		}
		if s.Title != "Theme Breakdown" {
			t.Errorf("title = %q, want Theme Breakdown", s.Title)
		}
		if len(s.Entries) != 19 {
			t.Fatalf("entries = %d, want 19", len(s.Entries))
		}
		first := s.Entries[0]
		if first.Name != "Grid Infrastructure" || first.Weight != 0.081002 {
			t.Errorf("first entry = %+v, want Grid Infrastructure 0.081002", first)
		}
		if f.SectionByClassification(RankClassificationSector) != nil {
			t.Error("sector section should be absent from a theme fixture")
		}
	})
}

// TestDecodeFlight_Tolerant checks that the decoder survives unknown chunk and
// row shapes without dropping the valid tables that follow them.
func TestDecodeFlight_Tolerant(t *testing.T) {
	// The body mixes an orphaned continuation fragment, a module preload row,
	// a plain string row, and a chunk with a JS-only escape (\x) that is not a
	// valid JSON escape; the decoder must drop all of them and still find the
	// table row that follows.
	body := `<!DOCTYPE html>
<html><body><script>
self.__next_f.push([1,",\"orphan\",\"continuation\","])
self.__next_f.push([1,"1e:I[826504,[\"/_next/static/chunks/x.js\"]]"])
self.__next_f.push([1,"7a:\"<!DOCTYPE html><html>plain string row</html>\""])
self.__next_f.push([1,"2f:bad \xZZ escape"])
self.__next_f.push([1,"42:[\"$\",\"div\",null,{\"id\":\"test-section\",\"columns\":[{\"name\":\"Test Table\"},{\"name\":\"As of 05/04/2026\"}],\"rows\":[{\"0\":\"Label\",\"1\":\"Value\"}],\"ariaLabel\":\"Test Table\"}]"])
</script></body></html>`

	f := DecodeFlight(body)
	tbl := f.Table("Test Table")
	if tbl == nil {
		t.Fatal("Test Table not found after garbage chunks")
	}
	if got, ok := tbl.Value("Label"); !ok || got != "Value" {
		t.Errorf("Label = %q (ok=%v), want Value", got, ok)
	}
	d, ok := tbl.AsOfDate()
	if !ok {
		t.Fatal("AsOfDate not parsed")
	}
	// Ambiguous 05/04: both parts <= 12, zero-padded → day-first (d/m/y).
	if want := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC); !d.Equal(want) {
		t.Errorf("AsOfDate = %v, want %v (padded → day-first)", d, want)
	}
	if f.SectionByClassification(RankClassificationSector) != nil {
		t.Error("no ranking section expected")
	}
}

// TestDecodeFlight_Empty checks that a page with no flight payload yields an
// empty Flight rather than an error.
func TestDecodeFlight_Empty(t *testing.T) {
	f := DecodeFlight("<html><body><p>no flight data</p></body></html>")
	if f == nil {
		t.Fatal("expected non-nil Flight for empty page")
	}
	if f.Table("Overview Table") != nil {
		t.Error("expected nil table for empty page")
	}
	if f.SectionByClassification(RankClassificationSector) != nil {
		t.Error("expected nil section for empty page")
	}
}

// TestParseFlightAsOfDate covers both site date formats and the ambiguous
// case for the flight table header parser (distinct from Phase 1's
// ParseAsOfDate, which is removed in task 6).
func TestParseFlightAsOfDate(t *testing.T) {
	tests := []struct {
		header string
		want   time.Time
		ok     bool
	}{
		// UCITS: zero-padded day-first.
		{"As of 28/08/2026", time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC), true},
		// US: unpadded month-first.
		{"As of 8/27/2026", time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC), true},
		{"As of 2/23/2007", time.Date(2007, 2, 23, 0, 0, 0, 0, time.UTC), true},
		// Ambiguous (both parts <= 12): padding resolves the order.
		{"As of 05/04/2026", time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC), true},
		{"As of 4/5/2026", time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC), true},
		// Non-date or malformed headers.
		{"Weight (%)", time.Time{}, false},
		{"", time.Time{}, false},
		{"As of 2026-08-28", time.Time{}, false},
		{"As of 32/08/2026", time.Time{}, false},
		{"As of 13/14/2026", time.Time{}, false},
	}
	for _, tc := range tests {
		d, ok := parseAsOfDate(tc.header)
		if ok != tc.ok {
			t.Errorf("parseAsOfDate(%q) ok = %v, want %v", tc.header, ok, tc.ok)
			continue
		}
		if ok && !d.Equal(tc.want) {
			t.Errorf("parseAsOfDate(%q) = %v, want %v", tc.header, d, tc.want)
		}
	}
}
