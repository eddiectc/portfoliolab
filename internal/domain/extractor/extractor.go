package extractor

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"
)

// ExtractResult holds all data sections extracted from a provider page.
// The extractor is responsible for populating all fields; if any section
// fails to parse, the entire extraction is rejected (atomic — no partial data).
type ExtractResult struct {
	// AsOfDate is the provider's "as of" date for the extracted data,
	// distinct from when the system fetched it.
	AsOfDate time.Time

	// FundInfo contains symbol and name from the provider page.
	FundInfo *FundInfo

	// FundProfile contains AUM, TER, inception date, etc.
	FundProfile *FundProfile

	// Holdings is the full list of security holdings (not limited to top 10).
	Holdings []Holding

	// NavHistory is the time series of NAV data points.
	NavHistory []NavPoint

	// Themes is the thematic breakdown (e.g. "AI", "Clean Energy").
	Themes []Theme

	// Sectors is the sector weighting breakdown.
	Sectors []SectorWeighting

	// CountryAllocation is the geographic breakdown by country.
	CountryAllocation []CountryAllocation

	// MarketCap is the market capitalization breakdown.
	MarketCap *MarketCapBreakdown

	// Characteristics is the fund characteristics data (P/E, P/B, etc.).
	Characteristics *FundCharacteristics
}

// FundInfo contains basic fund identity from the provider.
type FundInfo struct {
	Symbol string
	Name   string
}

// FundProfile contains fund-level metadata (AUM, TER, inception date, family, legal type).
type FundProfile struct {
	Family                 string
	LegalType              string
	TotalNetAssets         float64
	AnnualExpenseRatio     float64
	AnnualHoldingsTurnover float64
	InceptionDate          time.Time
}

// Holding is a single security holding with weight percentage.
type Holding struct {
	Symbol  string
	Name    string
	Percent float64
}

// NavPoint is a single NAV data point.
type NavPoint struct {
	Date string
	NAV  float64
}

// Theme is a thematic allocation entry.
type Theme struct {
	Name    string
	Percent float64
}

// SectorWeighting is a sector allocation entry.
type SectorWeighting struct {
	Sector  string
	Percent float64
}

// CountryAllocation is a geographic allocation entry.
type CountryAllocation struct {
	Country string
	Percent float64
}

// MarketCapBreakdown contains market capitalization distribution.
type MarketCapBreakdown struct {
	Total    float64
	Large    float64
	Mid      float64
	Small    float64
}

// FundCharacteristics contains valuation and other fund characteristics.
type FundCharacteristics struct {
	PriceToEarnings  float64
	PriceToBook      float64
	PriceToCashflow  float64
	PriceToSales     float64
	DividendYield    float64
}

// Extractor extracts data from a specific provider's web pages.
// Each provider (e.g. WisdomTree) implements this interface.
type Extractor interface {
	// Name returns the unique identifier for this extractor (e.g. "wisdomtree").
	Name() string

	// Extract fetches and parses data from the given source URL.
	// Returns an ExtractResult with all populated sections, or an error
	// if any section fails (atomic — no partial data).
	Extract(ctx context.Context, sourceURL string) (*ExtractResult, error)
}

// URLMatcher checks if a URL belongs to this extractor's domain.
type URLMatcher interface {
	Match(rawURL string) bool
}

// registryEntry pairs an Extractor with its URLMatcher for URL-based lookup.
type registryEntry struct {
	extractor Extractor
	matcher   URLMatcher
}

// Registry manages a collection of extractors indexed by name and URL pattern.
type Registry struct {
	mu     sync.RWMutex
	byName map[string]Extractor
	byURL  []registryEntry // ordered by registration; first match wins
}

// NewRegistry creates a new empty extractor registry.
func NewRegistry() *Registry {
	return &Registry{
		byName: make(map[string]Extractor),
	}
}

// Register adds an extractor to the registry. It is indexed by name and
// by URL pattern (the extractor must implement URLMatcher).
// Returns an error if an extractor with the same name is already registered.
func (r *Registry) Register(e Extractor) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := e.Name()
	if _, exists := r.byName[name]; exists {
		return fmt.Errorf("extractor %q already registered", name)
	}

	r.byName[name] = e
	if matcher, ok := e.(URLMatcher); ok {
		r.byURL = append(r.byURL, registryEntry{extractor: e, matcher: matcher})
	}
	return nil
}

// FindByURL returns the first registered extractor whose URLMatcher matches
// the given URL. Returns an error if no extractor matches.
func (r *Registry) FindByURL(rawURL string) (Extractor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, entry := range r.byURL {
		if entry.matcher.Match(rawURL) {
			return entry.extractor, nil
		}
	}
	return nil, fmt.Errorf("no extractor registered for URL %q", rawURL)
}

// Get returns the extractor with the given name.
// Returns an error if no extractor with that name is registered.
func (r *Registry) Get(name string) (Extractor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.byName[name]
	if !ok {
		return nil, fmt.Errorf("extractor %q not registered", name)
	}
	return e, nil
}

// Dispatcher routes extraction requests to the correct extractor based on
// the source URL. It holds a reference to the registry.
type Dispatcher struct {
	registry *Registry
}

// NewDispatcher creates a new dispatcher backed by the given registry.
func NewDispatcher(registry *Registry) *Dispatcher {
	return &Dispatcher{registry: registry}
}

// Dispatch finds the matching extractor for the source URL and calls its
// Extract method. Returns the ExtractResult or an error if no extractor
// matches or extraction fails.
func (d *Dispatcher) Dispatch(ctx context.Context, sourceURL string) (*ExtractResult, error) {
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("invalid source URL %q: %w", sourceURL, err)
	}

	extractor, err := d.registry.FindByURL(parsed.String())
	if err != nil {
		return nil, fmt.Errorf("dispatch to %q: %w", sourceURL, err)
	}

	result, err := extractor.Extract(ctx, parsed.String())
	if err != nil {
		return nil, fmt.Errorf("extract from %q (%s): %w", sourceURL, extractor.Name(), err)
	}

	return result, nil
}
