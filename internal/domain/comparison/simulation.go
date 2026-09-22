package comparison

import (
	"sort"
	"time"

	"github.com/eddiectc/portfoliolab/internal/market"
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

// Allocation strategies for SimulateEquityCurve.
const (
	StrategyBuyAndHold = "buy_and_hold" // each position is held from the start (default)
	StrategyRebalanced = "rebalanced"   // positions are re-anchored to target weights at interval boundaries
)

// Rebalance intervals for the rebalanced strategy.
const (
	IntervalMonthly   = "monthly"   // 1st of each month
	IntervalQuarterly = "quarterly" // Jan/Apr/Jul/Oct 1
	IntervalAnnual    = "annual"    // Jan 1 of each year
)

// SimulateEquityCurveInput holds the parameters for an equity curve
// simulation.
type SimulateEquityCurveInput struct {
	StartingValue decimal.Decimal                     // initial investment amount
	Weights       []ModelPortfolioWeight              // symbol weights (must sum to ~1.0)
	PricesBySym   map[string][]market.HistoricalPrice // marketSym -> price series (sorted ASC)
	BaseCurrency  string                              // output currency
	DateFrom      time.Time                           // output start (clipped to data availability)
	DateTo        time.Time                           // output end
	FxRates       map[string][]market.HistoricalPrice // FX pair (e.g. "GBP/USD") -> price series
	// Strategy selects the allocation strategy: StrategyBuyAndHold (default)
	// or StrategyRebalanced. Empty is treated as buy-and-hold.
	Strategy string
	// RebalanceInterval selects the rebalance cadence: IntervalMonthly,
	// IntervalQuarterly, or IntervalAnnual. Only used when Strategy is
	// StrategyRebalanced; an unknown value falls back to buy-and-hold
	// behavior.
	RebalanceInterval string
	// RebalanceThresholdPct is an optional drift threshold in percentage
	// points (e.g. 5 = 5pp). When set, a boundary only triggers a rebalance
	// if some component's actual weight deviates from its target weight by
	// at least this amount. Nil means rebalance unconditionally at every
	// boundary.
	RebalanceThresholdPct *decimal.Decimal
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
// weighted positions, converted to the base currency. NavPerUnit is the
// time-weighted NAV per unit (cash-flow-independent). For model portfolios
// (single initial deposit), NavPerUnit == PortfolioValue. For real portfolios,
// it is computed by the performance layer via unitization.
type EquityCurvePoint struct {
	Date           time.Time        `json:"date"`
	PortfolioValue decimal.Decimal  `json:"portfolio_value"`
	NavPerUnit     *decimal.Decimal `json:"nav_per_unit"`
}

// SimulateEquityCurve computes an equity curve for a model portfolio. It
// allocates the starting value across symbols according to the given
// weights, then for each trading day in the period computes the total
// portfolio value from the daily close prices.
//
// The default buy-and-hold strategy holds every position from the start.
// The rebalanced strategy re-anchors positions to their target weights on
// the first trading day on or after each interval boundary (strictly after
// the first trading day of the period), subject to an optional drift
// threshold.
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
	periodClipped := (!input.DateFrom.IsZero() && clipFrom.After(input.DateFrom)) ||
		(!input.DateTo.IsZero() && clipTo.Before(input.DateTo))

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
			EquityCurve:           []EquityCurvePoint{},
			Warnings:              warnings,
			LimitedHistorySymbols: limitedSymbols,
			PeriodClipped:         periodClipped,
		}
	}

	// Collect all dates in the clipped range.
	dates := collectDatesInRange(input.Weights, priceLookup, clipFrom, clipTo)
	if len(dates) == 0 {
		return &SimulateEquityCurveOutput{
			EquityCurve:           []EquityCurvePoint{},
			Warnings:              warnings,
			LimitedHistorySymbols: limitedSymbols,
			PeriodClipped:         periodClipped,
		}
	}

	// Compute daily portfolio values.
	curve := computeDailyValues(input.StartingValue, input.Weights, input.BaseCurrency, dates, priceLookup, fxLookup,
		input.Strategy, input.RebalanceInterval, input.RebalanceThresholdPct)

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

// ffLookup is a generic forward-fill lookup: sorted dates + value map.
// Given a dateKey, returns the value for that date or the nearest previous
// date (forward-fill). Returns (zero, false) if no value exists on or before
// the given date.
type ffLookup[T any] struct {
	dates  []string // sorted ascending
	values map[string]T
}

func newFfLookup[T any](dates []string, values map[string]T) *ffLookup[T] {
	sort.Strings(dates)
	return &ffLookup[T]{dates: dates, values: values}
}

func (f *ffLookup[T]) lookup(dateKey string) (T, bool) {
	idx := sort.SearchStrings(f.dates, dateKey)
	// Exact match.
	if idx < len(f.dates) && f.dates[idx] == dateKey {
		return f.values[dateKey], true
	}
	// Forward-fill from previous date.
	if idx > 0 {
		return f.values[f.dates[idx-1]], true
	}
	// Backward-fill from first known date.
	if len(f.dates) > 0 {
		return f.values[f.dates[0]], true
	}
	var zero T
	return zero, false
}

// buildFfPriceLookup builds forward-fill lookups from the price date-map.
func buildFfPriceLookup(lookup map[string]map[string]market.HistoricalPrice) map[string]*ffLookup[market.HistoricalPrice] {
	ff := make(map[string]*ffLookup[market.HistoricalPrice])
	for symbol, dateMap := range lookup {
		dates := make([]string, 0, len(dateMap))
		for d := range dateMap {
			dates = append(dates, d)
		}
		ff[symbol] = newFfLookup(dates, dateMap)
	}
	return ff
}

// buildFxLookup builds forward-fill FX lookups from FX price series.
func buildFxLookup(fxRates map[string][]market.HistoricalPrice) map[string]*ffLookup[decimal.Decimal] {
	lookup := make(map[string]*ffLookup[decimal.Decimal])
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
		lookup[pair] = newFfLookup(dates, rates)
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
// Buy-and-hold (default): for each symbol
//
//	shares = (startingValue * weight) / basePrice
//	value[date] = shares * price[date]
//
// where basePrice is the first available close price for that symbol.
// This gives value[date] = allocated * (price[date] / basePrice).
//
// Rebalanced: positions evolve buy-and-hold between rebalance days. On each
// rebalance day (first trading day on or after an interval boundary, strictly
// after the first trading day of the period) they are re-anchored so that
// each position's base-currency value equals totalValue * target weight
// (FX-converted at that day's rate). An optional drift threshold suppresses
// a boundary rebalance when no position has drifted from its target by at
// least the threshold.
//
// Missing prices are forward-filled from the last known close, so gaps
// in market data (weekends, holidays, sparse sources) do not cause
// artificial portfolio value drops.
func computeDailyValues(startingValue decimal.Decimal, weights []ModelPortfolioWeight, baseCurrency string, dates []time.Time, priceLookup map[string]map[string]market.HistoricalPrice, fxLookup map[string]*ffLookup[decimal.Decimal], strategy string, rebalanceInterval string, thresholdPct *decimal.Decimal) []EquityCurvePoint {
	curve := make([]EquityCurvePoint, 0, len(dates))

	// Build forward-fill price lookup for gap handling.
	ff := buildFfPriceLookup(priceLookup)

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

	if strategy == StrategyRebalanced {
		return computeRebalancedValues(startingValue, weights, baseCurrency, dates, ff, basePrices, fxLookup, rebalanceInterval, thresholdPct)
	}

	for _, date := range dates {
		dateKey := date.Format("2006-01-02")
		var totalValue decimal.Decimal

		for _, w := range weights {
			if _, ok := priceLookup[w.MarketSym]; !ok {
				continue
			}
			// Forward-fill: use exact price or last known close.
			price, ok := ff[w.MarketSym].lookup(dateKey)
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
			NavPerUnit:     &totalValue, // model portfolio: single deposit, NAV == value
		})
	}

	return curve
}

// positionState tracks a rebalanced position between anchor dates.
type positionState struct {
	allocated decimal.Decimal        // base-currency amount anchored at `anchor` (pre-FX)
	anchor    market.HistoricalPrice // anchor close (period start or last rebalance)
}

// computeRebalancedValues computes the equity curve for a rebalanced model
// portfolio. Positions are allocated to their target weights at the start of
// the period and re-anchored to target weights on each rebalance day.
//
// Between rebalances each position evolves exactly like buy-and-hold:
//
//	value[date] = allocated * (price[date] / anchorPrice)
//
// where anchorPrice is the close at the period start or the most recent
// rebalance. On a rebalance day, each position's allocated amount is reset
// so its base-currency value equals totalValue * target weight that day
// (FX-converted at that day's rate). When a drift threshold is set, a
// rebalance only occurs if at least one position's actual weight deviates
// from its target by at least the threshold; otherwise the boundary is
// skipped.
func computeRebalancedValues(startingValue decimal.Decimal, weights []ModelPortfolioWeight, baseCurrency string, dates []time.Time, ff map[string]*ffLookup[market.HistoricalPrice], basePrices map[string]market.HistoricalPrice, fxLookup map[string]*ffLookup[decimal.Decimal], rebalanceInterval string, thresholdPct *decimal.Decimal) []EquityCurvePoint {
	curve := make([]EquityCurvePoint, 0, len(dates))

	rebalanceDays := rebalanceDays(rebalanceInterval, dates)
	thresholdFrac, hasThreshold := thresholdAsFraction(thresholdPct)

	states := make(map[string]*positionState, len(weights))
	for _, w := range weights {
		bp, ok := basePrices[w.MarketSym]
		if !ok || !bp.Close.IsPos() {
			continue
		}
		allocated, _ := startingValue.Mul(w.Weight)
		states[w.MarketSym] = &positionState{allocated: allocated, anchor: bp}
	}

	for _, date := range dates {
		dateKey := date.Format("2006-01-02")
		var totalValue decimal.Decimal
		dayValues := make(map[string]decimal.Decimal, len(states))

		for _, w := range weights {
			st, ok := states[w.MarketSym]
			if !ok {
				continue
			}
			price, ok := ff[w.MarketSym].lookup(dateKey)
			if !ok || !st.anchor.Close.IsPos() {
				continue
			}
			// Position value: allocated * (price[date] / anchorPrice).
			priceF, _ := price.Close.Float64()
			baseF, _ := st.anchor.Close.Float64()
			ratio, _ := decimal.NewFromFloat64(priceF / baseF)
			value, _ := st.allocated.Mul(ratio)

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

			dayValues[w.MarketSym] = value
			totalValue, _ = totalValue.Add(value)
		}

		curve = append(curve, EquityCurvePoint{
			Date:           date,
			PortfolioValue: totalValue,
			NavPerUnit:     &totalValue, // model portfolio: single deposit, NAV == value
		})

		// Re-anchor to target weights on rebalance days when drift warrants it.
		if rebalanceDays[dateKey] && totalValue.IsPos() && driftExceeds(weights, dayValues, totalValue, thresholdFrac, hasThreshold) {
			reanchorToTargets(states, weights, dayValues, totalValue, ff, fxLookup, baseCurrency, dateKey)
		}
	}

	return curve
}

// rebalanceDays returns the set of trading-day keys on which a rebalance
// occurs: the first trading day on or after each calendar interval boundary
// that falls strictly after the first trading day of the period. An unknown
// interval yields no rebalance days (buy-and-hold behavior).
func rebalanceDays(interval string, dates []time.Time) map[string]bool {
	days := make(map[string]bool)
	if len(dates) < 2 {
		return days
	}
	first := dates[0]
	for _, boundary := range intervalBoundaries(interval, first, dates[len(dates)-1]) {
		if !boundary.After(first) {
			continue
		}
		idx := sort.Search(len(dates), func(i int) bool { return !dates[i].Before(boundary) })
		if idx < len(dates) && dates[idx].After(first) {
			days[dates[idx].Format("2006-01-02")] = true
		}
	}
	return days
}

// intervalBoundaries lists the calendar dates on which the given interval
// begins, for every year spanning [from, to] (one year padded on each side).
// An unknown interval yields nil.
func intervalBoundaries(interval string, from, to time.Time) []time.Time {
	var months []time.Month
	switch interval {
	case IntervalAnnual:
		months = []time.Month{time.January}
	case IntervalQuarterly:
		months = []time.Month{time.January, time.April, time.July, time.October}
	case IntervalMonthly:
		months = []time.Month{time.January, time.February, time.March, time.April, time.May, time.June,
			time.July, time.August, time.September, time.October, time.November, time.December}
	default:
		return nil
	}
	boundaries := make([]time.Time, 0, len(months)*(to.Year()-from.Year()+3))
	for y := from.Year() - 1; y <= to.Year()+1; y++ {
		for _, m := range months {
			boundaries = append(boundaries, time.Date(y, m, 1, 0, 0, 0, 0, time.UTC))
		}
	}
	return boundaries
}

// thresholdAsFraction converts an optional drift threshold given in
// percentage points (e.g. 5 = 5pp) to a fraction (0.05). A nil threshold
// means "rebalance unconditionally at every boundary".
func thresholdAsFraction(pct *decimal.Decimal) (decimal.Decimal, bool) {
	if pct == nil {
		return decimal.Zero, false
	}
	hundred, _ := decimal.New(100, 0)
	frac, err := pct.Quo(hundred)
	if err != nil {
		return decimal.Zero, false
	}
	return frac, true
}

// driftExceeds reports whether at least one position's actual weight deviates
// from its target weight by at least the threshold (a fraction). Without a
// threshold, every boundary triggers a rebalance.
func driftExceeds(weights []ModelPortfolioWeight, dayValues map[string]decimal.Decimal, totalValue decimal.Decimal, thresholdFrac decimal.Decimal, hasThreshold bool) bool {
	if !hasThreshold {
		return true
	}
	for _, w := range weights {
		value, ok := dayValues[w.MarketSym]
		if !ok {
			continue
		}
		actual, err := value.Quo(totalValue)
		if err != nil {
			continue
		}
		diff, err := actual.Sub(w.Weight)
		if err != nil {
			continue
		}
		if diff.Abs().Cmp(thresholdFrac) >= 0 {
			return true
		}
	}
	return false
}

// reanchorToTargets re-anchors every position to its target weight as of the
// given date: allocated is set so the position's base-currency value equals
// totalValue * target weight on that date, and the anchor price becomes the
// day's close. FX-converted positions are back-converted with the day's FX
// rate. Positions with no value that day (e.g. missing FX data) are left
// unchanged.
func reanchorToTargets(states map[string]*positionState, weights []ModelPortfolioWeight, dayValues map[string]decimal.Decimal, totalValue decimal.Decimal, ff map[string]*ffLookup[market.HistoricalPrice], fxLookup map[string]*ffLookup[decimal.Decimal], baseCurrency string, dateKey string) {
	for _, w := range weights {
		if _, ok := dayValues[w.MarketSym]; !ok {
			continue
		}
		price, ok := ff[w.MarketSym].lookup(dateKey)
		if !ok {
			continue
		}
		target, _ := totalValue.Mul(w.Weight)
		allocated := target
		if w.Currency != "" && w.Currency != baseCurrency {
			rate, ok := fxRateOn(fxLookup, market.FormatFxPair(w.Currency, baseCurrency), dateKey)
			if !ok || !rate.IsPos() {
				// Cannot anchor reliably; keep the previous state.
				continue
			}
			back, err := target.Quo(rate)
			if err != nil {
				continue
			}
			allocated = back
		}
		st := states[w.MarketSym]
		st.allocated = allocated
		st.anchor = price
	}
}

// fxRateOn returns the forward-filled FX rate for the pair on or before the
// given date.
func fxRateOn(lookup map[string]*ffLookup[decimal.Decimal], pair, dateKey string) (decimal.Decimal, bool) {
	ffx, ok := lookup[pair]
	if !ok {
		return decimal.Zero, false
	}
	return ffx.lookup(dateKey)
}

// convertWithFxLookup converts a value using the FX rate lookup with forward-fill.
// Returns (0, false) if no FX data exists.
func convertWithFxLookup(lookup map[string]*ffLookup[decimal.Decimal], pair string, value decimal.Decimal, date time.Time) (decimal.Decimal, bool) {
	ff, ok := lookup[pair]
	if !ok || len(ff.dates) == 0 {
		return decimal.Zero, false
	}
	dateKey := date.Format("2006-01-02")
	rate, ok := ff.lookup(dateKey)
	if !ok {
		return decimal.Zero, false
	}
	converted, _ := value.Mul(rate)
	return converted, true
}
