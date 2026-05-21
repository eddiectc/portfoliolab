package comparison

import (
	"sort"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// ModelPortfolioWeight maps a symbol to its target weight fraction (0.0–1.0,
// summing to 1.0). Used as input to SimulateEquityCurve.
type ModelPortfolioWeight struct {
	Symbol    string          // internal symbol
	Weight    decimal.Decimal // fraction of portfolio (e.g. 0.50 = 50%)
	Currency  string          // denominated currency of the symbol
	MarketSym string          // market data provider symbol (e.g. Yahoo ticker)
}

// SimulateEquityCurveInput holds the parameters for a buy-and-hold equity
// curve simulation.
type SimulateEquityCurveInput struct {
	StartingValue decimal.Decimal       // initial investment amount
	Weights       []ModelPortfolioWeight // symbol weights (must sum to ~1.0)
	PricesBySym   map[string][]market.HistoricalPrice // marketSym -> price series (sorted ASC)
	BaseCurrency  string                // output currency
	DateFrom      time.Time             // output start (clipped to data availability)
	DateTo        time.Time             // output end
	FxRates       map[string][]market.HistoricalPrice // FX pair (e.g. "GBP/USD") -> price series
}

// SimulateEquityCurveOutput holds the results of the simulation.
type SimulateEquityCurveOutput struct {
	EquityCurve []EquityCurvePoint // daily portfolio values
	DateFrom    time.Time          // actual start date (may be clipped)
	DateTo      time.Time          // actual end date (may be clipped)
	Warnings    []string           // symbols with limited/missing data
	// LimitedHistorySymbols lists symbols whose data does not cover the
	// full requested period. The consumer should display a warning.
	LimitedHistorySymbols []string
	// PeriodClipped is true when the effective period (intersection of
	// available data) is shorter than the requested period. The consumer
	// should display the actual effective period to the user.
	PeriodClipped bool
}

// EquityCurvePoint is a single data point on the equity curve.
// Date is the trading day. PortfolioValue is the total market value of all
// weighted positions, converted to the base currency.
type EquityCurvePoint struct {
	Date           time.Time       `json:"date"`
	PortfolioValue decimal.Decimal `json:"portfolio_value"`
}

// SimulateEquityCurve computes a buy-and-hold equity curve for a model
// portfolio. It allocates the starting value across symbols according to
// the given weights, then for each trading day in the period computes the
// total portfolio value from the daily close prices.
//
// The period is clipped to the intersection of available data across all
// symbols. Symbols with no price data are listed in warnings.
//
// FX conversion: if a symbol's currency differs from baseCurrency, the
// corresponding FX pair is looked up. Historical rates are used when
// available; if no historical rate exists the function falls back to the
// latest available rate and records a warning.
//
// Returns nil output (empty curve) if input is invalid (no weights, zero
// starting value, or no price data available).
func SimulateEquityCurve(input SimulateEquityCurveInput) *SimulateEquityCurveOutput {
	if len(input.Weights) == 0 || !input.StartingValue.IsPos() {
		return &SimulateEquityCurveOutput{
			EquityCurve: []EquityCurvePoint{},
			Warnings:    []string{"no weights or zero starting value"},
		}
	}

	// Build price lookup: marketSym -> dateKey -> price.
	priceLookup := buildPriceLookup(input.PricesBySym)

	// Build FX lookup: pair -> forward-fill structure.
	fxLookup := buildFxLookup(input.FxRates)

	// Determine FX pairs needed (symbols whose currency != baseCurrency).
	fxPairs := neededFxPairs(input.Weights, input.BaseCurrency)

	// Clip period to the intersection of available data.
	clipFrom, clipTo, limitedSymbols, missingSymbols := clipPeriod(input.Weights, input.PricesBySym, input.DateFrom, input.DateTo)

	// Detect if the effective period is shorter than requested.
	periodClipped := false
	if !input.DateFrom.IsZero() && clipFrom.After(input.DateFrom) {
		periodClipped = true
	}
	if !input.DateTo.IsZero() && clipTo.Before(input.DateTo) {
		periodClipped = true
	}

	var warnings []string
	if periodClipped {
		warnings = append(warnings,
			"period clipped: "+clipFrom.Format("2006-01-02")+" to "+clipTo.Format("2006-01-02")+
			" (requested "+input.DateFrom.Format("2006-01-02")+" to "+input.DateTo.Format("2006-01-02")+")")
	}
	for _, sym := range missingSymbols {
		warnings = append(warnings, "no price data for "+sym)
	}
	for _, sym := range limitedSymbols {
		warnings = append(warnings, sym+": limited price data (period clipped)")
	}

	// Warn about FX pairs with no data.
	for _, pair := range fxPairs {
		if _, ok := fxLookup[pair]; !ok {
			warnings = append(warnings, "no FX data for "+pair+" (values not converted)")
		}
	}

	// If no overlap period, return empty.
	if clipFrom.After(clipTo) {
		return &SimulateEquityCurveOutput{
			EquityCurve:             []EquityCurvePoint{},
			Warnings:                warnings,
			LimitedHistorySymbols:   limitedSymbols,
			PeriodClipped:           periodClipped,
		}
	}

	// Collect all dates in the clipped range.
	dates := collectDatesInRange(input.Weights, priceLookup, clipFrom, clipTo)
	if len(dates) == 0 {
		return &SimulateEquityCurveOutput{
			EquityCurve:             []EquityCurvePoint{},
			Warnings:                warnings,
			LimitedHistorySymbols:   limitedSymbols,
			PeriodClipped:           periodClipped,
		}
	}

	// Compute daily portfolio values.
	curve := computeDailyValues(input.StartingValue, input.Weights, input.BaseCurrency, dates, priceLookup, fxLookup)

	return &SimulateEquityCurveOutput{
		EquityCurve:           curve,
		DateFrom:              clipFrom,
		DateTo:                clipTo,
		Warnings:              warnings,
		LimitedHistorySymbols: limitedSymbols,
		PeriodClipped:         periodClipped,
	}
}

// buildPriceLookup builds a two-level map: symbol -> dateKey -> price.
func buildPriceLookup(pricesBySym map[string][]market.HistoricalPrice) map[string]map[string]market.HistoricalPrice {
	lookup := make(map[string]map[string]market.HistoricalPrice)
	for sym, prices := range pricesBySym {
		lookup[sym] = make(map[string]market.HistoricalPrice, len(prices))
		for _, p := range prices {
			key := p.Date.Format("2006-01-02")
			lookup[sym][key] = p
		}
	}
	return lookup
}

// fxLookupFF holds a forward-fill lookup for FX rates.
type fxLookupFF struct {
	dates []string // sorted date keys
	rates map[string]decimal.Decimal
}

// buildFxLookup builds forward-fill FX lookups from FX price series.
func buildFxLookup(fxRates map[string][]market.HistoricalPrice) map[string]*fxLookupFF {
	lookup := make(map[string]*fxLookupFF)
	for pair, prices := range fxRates {
		if len(prices) == 0 {
			continue
		}
		dates := make([]string, len(prices))
		rates := make(map[string]decimal.Decimal, len(prices))
		for i, p := range prices {
			key := p.Date.Format("2006-01-02")
			dates[i] = key
			rates[key] = p.Close
		}
		sort.Strings(dates)
		lookup[pair] = &fxLookupFF{dates: dates, rates: rates}
	}
	return lookup
}

// neededFxPairs returns the unique FX pairs needed for symbols whose
// currency differs from the base currency.
func neededFxPairs(weights []ModelPortfolioWeight, baseCurrency string) []string {
	set := make(map[string]bool)
	for _, w := range weights {
		if w.Currency != "" && w.Currency != baseCurrency {
			pair := market.FormatFxPair(w.Currency, baseCurrency)
			set[pair] = true
		}
	}
	pairs := make([]string, 0, len(set))
	for pair := range set {
		pairs = append(pairs, pair)
	}
	sort.Strings(pairs)
	return pairs
}

// clipPeriod determines the overlapping date range across all symbols and
// identifies symbols with limited or missing data.
//
// Symbols are checked against the *requested* period (dateFrom/dateTo), not
// the clipped period. This ensures a symbol with only 1Y of data is flagged
// as limited even when the user requests 3Y and the effective period is
// clipped to 1Y.
//
// Returns (clipFrom, clipTo, limitedSymbols, missingSymbols).
// If no symbols have data, clipFrom > clipTo (empty range).
func clipPeriod(weights []ModelPortfolioWeight, pricesBySym map[string][]market.HistoricalPrice, dateFrom, dateTo time.Time) (time.Time, time.Time, []string, []string) {
	var earliest time.Time
	var latest time.Time
	var limitedSymbols, missingSymbols []string
	hadData := false

	for _, w := range weights {
		prices, ok := pricesBySym[w.MarketSym]
		if !ok || len(prices) == 0 {
			missingSymbols = append(missingSymbols, w.Symbol)
			continue
		}

		symEarliest := prices[0].Date
		symLatest := prices[len(prices)-1].Date

		if !hadData {
			earliest = symEarliest
			latest = symLatest
			hadData = true
		} else {
			if symEarliest.After(earliest) {
				earliest = symEarliest
			}
			if symLatest.Before(latest) {
				latest = symLatest
			}
		}
	}

	if !hadData {
		return time.Time{}, time.Time{}, nil, missingSymbols
	}

	// Determine the effective (requested) period for coverage checks.
	// Use the user's requested dates when provided, otherwise the raw data range.
	checkFrom := earliest
	checkTo := latest
	if !dateFrom.IsZero() {
		checkFrom = dateFrom
	}
	if !dateTo.IsZero() {
		checkTo = dateTo
	}

	// Check which symbols have limited data relative to the requested period.
	for _, w := range weights {
		prices, ok := pricesBySym[w.MarketSym]
		if !ok || len(prices) == 0 {
			continue // already in missingSymbols
		}
		symEarliest := prices[0].Date
		symLatest := prices[len(prices)-1].Date

		coversFrom := !symEarliest.After(checkFrom)
		coversTo := !symLatest.Before(checkTo)
		if !coversFrom || !coversTo {
			limitedSymbols = append(limitedSymbols, w.Symbol)
		}
	}

	// Apply user-specified date constraints to the output period.
	if !dateFrom.IsZero() && dateFrom.After(earliest) {
		earliest = dateFrom
	}
	if !dateTo.IsZero() && dateTo.Before(latest) {
		latest = dateTo
	}

	return earliest, latest, limitedSymbols, missingSymbols
}

// collectDatesInRange collects all unique trading dates across all symbols
// within [from, to].
func collectDatesInRange(weights []ModelPortfolioWeight, priceLookup map[string]map[string]market.HistoricalPrice, from, to time.Time) []time.Time {
	dateSet := make(map[string]bool)
	for _, w := range weights {
		prices, ok := priceLookup[w.MarketSym]
		if !ok {
			continue
		}
		for dateKey, p := range prices {
			if (p.Date.Equal(from) || p.Date.After(from)) && (p.Date.Equal(to) || p.Date.Before(to)) {
				dateSet[dateKey] = true
			}
		}
	}

	dates := make([]time.Time, 0, len(dateSet))
	for dateKey := range dateSet {
		t, err := time.Parse("2006-01-02", dateKey)
		if err != nil {
			continue
		}
		dates = append(dates, t)
	}
	sort.Slice(dates, func(i, j int) bool {
		return dates[i].Before(dates[j])
	})
	return dates
}

// computeDailyValues computes the portfolio value for each date.
//
// For each symbol the buy-and-hold logic is:
//   shares = (startingValue * weight) / basePrice
//   value[date] = shares * price[date]
// where basePrice is the first available close price for that symbol.
// This gives value[date] = allocated * (price[date] / basePrice).
func computeDailyValues(startingValue decimal.Decimal, weights []ModelPortfolioWeight, baseCurrency string, dates []time.Time, priceLookup map[string]map[string]market.HistoricalPrice, fxLookup map[string]*fxLookupFF) []EquityCurvePoint {
	curve := make([]EquityCurvePoint, 0, len(dates))

	// Pre-compute base prices (first available price for each symbol).
	basePrices := make(map[string]market.HistoricalPrice)
	for _, w := range weights {
		prices, ok := priceLookup[w.MarketSym]
		if !ok {
			continue
		}
		// Find the earliest price.
		var earliest market.HistoricalPrice
		var found bool
		for _, p := range prices {
			if !found || p.Date.Before(earliest.Date) {
				earliest = p
				found = true
			}
		}
		if found {
			basePrices[w.MarketSym] = earliest
		}
	}

	for _, date := range dates {
		dateKey := date.Format("2006-01-02")
		var totalValue decimal.Decimal

		for _, w := range weights {
			prices, ok := priceLookup[w.MarketSym]
			if !ok {
				continue
			}
			price, ok := prices[dateKey]
			if !ok {
				continue
			}
			basePrice, ok := basePrices[w.MarketSym]
			if !ok || !basePrice.Close.IsPos() {
				continue
			}

			// Compute allocated amount: startingValue * weight.
			allocated, _ := startingValue.Mul(w.Weight)

			// Buy-and-hold value: allocated * (price[date] / basePrice).
			priceF, _ := price.Close.Float64()
			baseF, _ := basePrice.Close.Float64()
			ratio, _ := decimal.NewFromFloat64(priceF / baseF)
			value, _ := allocated.Mul(ratio)

			// Convert to base currency if needed.
			if price.Currency != "" && price.Currency != baseCurrency {
				pair := market.FormatFxPair(price.Currency, baseCurrency)
				converted, ok := convertWithFxLookup(fxLookup, pair, value, date)
				if !ok {
					// No FX data — skip this symbol's value.
					continue
				}
				value = converted
			}

			totalValue, _ = totalValue.Add(value)
		}

		curve = append(curve, EquityCurvePoint{
			Date:           date,
			PortfolioValue: totalValue,
		})
	}

	return curve
}

// convertWithFxLookup converts a value using the FX rate lookup with forward-fill.
// Returns (0, false) if no FX data exists.
func convertWithFxLookup(lookup map[string]*fxLookupFF, pair string, value decimal.Decimal, date time.Time) (decimal.Decimal, bool) {
	ff, ok := lookup[pair]
	if !ok || len(ff.dates) == 0 {
		return decimal.Zero, false
	}
	dateKey := date.Format("2006-01-02")
	idx := sort.SearchStrings(ff.dates, dateKey)

	// Exact match.
	if idx < len(ff.dates) && ff.dates[idx] == dateKey {
		rate := ff.rates[dateKey]
		converted, _ := value.Mul(rate)
		return converted, true
	}

	// Forward-fill from previous date.
	if idx > 0 {
		rate := ff.rates[ff.dates[idx-1]]
		converted, _ := value.Mul(rate)
		return converted, true
	}

	// Backward-fill from first known date.
	rate := ff.rates[ff.dates[0]]
	converted, _ := value.Mul(rate)
	return converted, true
}
