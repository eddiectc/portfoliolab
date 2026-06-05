package efficientfrontier

import (
	"math"
	"math/rand"
	"sort"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/stats"
)

const (
	// maxSymbols is the maximum number of candidate symbols.
	maxSymbols = 10
	// numCandidates is the number of random portfolios sampled.
	numCandidates = 2000
	// frontierSampleSize is the number of points returned on the frontier curve.
	frontierSampleSize = 25
	// minTradingDays is the minimum number of daily returns required.
	minTradingDays = 60
)

// ComputeFrontier computes the efficient frontier from historical price data.
//
// Algorithm:
//  1. Compute daily returns per symbol and align by date.
//  2. Compute expected returns (annualized mean) and covariance matrix.
//  3. Compute analytical minimum variance portfolio via matrix inversion.
//  4. Sample ~2000 random portfolios on the simplex (Dirichlet distribution).
//  5. Evaluate each candidate: return = w'μ, volatility = sqrt(w'Σw).
//  6. Filter to Pareto frontier (efficient points only).
//  7. Merge analytical min-variance point into efficient set.
//  8. Identify max Sharpe ratio portfolio.
//  9. Return 20-30 frontier points sampled from the efficient set.
func ComputeFrontier(request FrontierRequest) (*FrontierResult, error) {
	symbols := request.Symbols
	n := len(symbols)

	// Validate symbol count.
	if n < 2 {
		return &FrontierResult{
			Symbols:    symbols,
			ComputedAt: time.Now().UTC(),
			Message:    "At least 2 symbols are required for frontier computation.",
		}, nil
	}
	if n > maxSymbols {
		return nil, ErrTooManySymbols
	}

	// Copy and sort symbols for deterministic output (avoid mutating caller's slice).
	symbols = make([]string, len(request.Symbols))
	copy(symbols, request.Symbols)
	sort.Strings(symbols)

	// Compute returns per symbol.
	returnsBySymbol := make(map[string][]float64, n)
	minReturns := math.MaxInt
	for _, sym := range symbols {
		prices := request.Prices[sym]
		if len(prices) < 2 {
			return &FrontierResult{
				Symbols:    symbols,
				ComputedAt: time.Now().UTC(),
				Message:    sym + ": insufficient price data. Each symbol needs at least 2 price points.",
			}, nil
		}
		rets, err := ComputeReturns(prices)
		if err != nil {
			return &FrontierResult{
				Symbols:    symbols,
				ComputedAt: time.Now().UTC(),
				Warnings:   []string{sym + ": failed to compute returns, excluded"},
				Message:    "Insufficient valid price data after processing.",
			}, nil
		}
		returnsBySymbol[sym] = rets
		if len(rets) < minReturns {
			minReturns = len(rets)
		}
	}

	// Check minimum data sufficiency.
	if minReturns < minTradingDays {
		return &FrontierResult{
			Symbols:    symbols,
			ComputedAt: time.Now().UTC(),
			Warnings:   []string{"Some symbols have fewer than " + itoa(minTradingDays) + " trading days. Results may be unreliable."},
		}, nil
	}

	// Compute covariance matrix and aligned symbols.
	// ComputeCovarianceMatrix returns annualized covariance (via stats.AnnualizedCovarianceMatrix).
	covMatrix, alignedSymbols, err := ComputeCovarianceMatrix(request.Prices)
	if err != nil {
		return nil, err
	}

	// Re-sort symbols to match covariance matrix output.
	symbols = alignedSymbols
	n = len(symbols)

	// Compute trading days from aligned data.
	tradingDays := minReturns
	if tradingDays < 2 {
		return nil, ErrInsufficientData
	}

	// Compute expected annualized returns (μ) for each symbol.
	// stats.AnnualizedReturn returns a ratio (e.g. 0.15 = 15%).
	expectedReturns := make([]float64, n)
	for i, sym := range symbols {
		rets := returnsBySymbol[sym]
		// Use the aligned count (minReturns) for consistency.
		// Take only the first minReturns returns to match the covariance period.
		alignedRet := rets
		if len(alignedRet) > minReturns {
			alignedRet = alignedRet[:minReturns]
		}
		expectedReturns[i] = stats.AnnualizedReturn(alignedRet)
	}

	// Analytical minimum variance portfolio.
	minVarWeights := ComputeMinVariance(covMatrix, n)
	var minVarReturn, minVarVol float64
	if minVarWeights != nil {
		minVarReturn = stats.PortfolioReturn(expectedReturns, minVarWeights)
		minVarVol = stats.PortfolioVolatility(covMatrix, minVarWeights)
	}

	// Grid search: sample random portfolios on the simplex.
	candidates := samplePortfolios(n, numCandidates)

	var evaluated []portfolioEval
	for _, w := range candidates {
		ret := stats.PortfolioReturn(expectedReturns, w)
		vol := stats.PortfolioVolatility(covMatrix, w)
		if vol <= 0 {
			continue
		}
		// All inputs are annualized ratios; risk-free rate is already a ratio (e.g. 0.045).
		sharpe := stats.SharpeRatio(ret, vol, request.RiskFreeRate)
		evaluated = append(evaluated, portfolioEval{
			weights:    w,
			return_:    ret,
			volatility: vol,
			sharpe:     sharpe,
		})
	}

	if len(evaluated) == 0 {
		return nil, ErrNumericalFailure
	}

	// Add analytical min-variance portfolio to evaluated set.
	if minVarWeights != nil && minVarVol > 0 {
		sharpe := stats.SharpeRatio(minVarReturn, minVarVol, request.RiskFreeRate)
		evaluated = append(evaluated, portfolioEval{minVarWeights, minVarReturn, minVarVol, sharpe})
	}

	// Filter to Pareto frontier (efficient points).
	// A point is efficient if no other point has both higher return and lower volatility.
	efficient := filterParetoFrontier(evaluated)

	if len(efficient) == 0 {
		return nil, ErrNumericalFailure
	}

	// Sort efficient set by volatility ascending.
	sort.Slice(efficient, func(i, j int) bool {
		return efficient[i].volatility < efficient[j].volatility
	})

	// Sample frontier points (uniformly by volatility range).
	frontierPoints := sampleFrontierPoints(efficient, frontierSampleSize)

	// Find max Sharpe ratio portfolio.
	maxSharpeIdx := 0
	for i, p := range evaluated {
		if p.sharpe > evaluated[maxSharpeIdx].sharpe {
			maxSharpeIdx = i
		}
	}
	best := evaluated[maxSharpeIdx]

	// Find highest return on efficient frontier.
	highestRetIdx := len(efficient) - 1 // last in sorted-by-vol order has highest return

	// Build result.
	// Convert expected returns from ratios to percentages.
	expectedReturnsPct := make([]float64, len(expectedReturns))
	for i, r := range expectedReturns {
		expectedReturnsPct[i] = roundTo2(r * 100)
	}

	result := &FrontierResult{
		FrontierPoints:   make([]FrontierPoint, len(frontierPoints)),
		Symbols:          symbols,
		ExpectedReturns:  expectedReturnsPct,
		TradingDays:      tradingDays,
		ComputedAt:       time.Now().UTC(),
	}

	// Convert ratios to percentages for output.
	// stats functions return ratios (e.g. 0.15 = 15%); result fields expect percentages.
	for i, p := range frontierPoints {
		result.FrontierPoints[i] = FrontierPoint{
			ReturnPct:     roundTo2(p.return_ * 100),
			VolatilityPct: roundTo2(p.volatility * 100),
			SharpeRatio:   roundTo4(p.sharpe),
			Weights:       roundWeights(p.weights),
		}
	}

	if minVarWeights != nil && minVarVol > 0 {
		result.MinVariance = &OptimizedPortfolio{
			Name:          "Min Variance",
			ReturnPct:     roundTo2(minVarReturn * 100),
			VolatilityPct: roundTo2(minVarVol * 100),
			SharpeRatio:   roundTo4(stats.SharpeRatio(minVarReturn, minVarVol, request.RiskFreeRate)),
			Weights:       roundWeights(minVarWeights),
		}
	}

	result.MaxSharpe = &OptimizedPortfolio{
		Name:          "Max Sharpe",
		ReturnPct:     roundTo2(best.return_ * 100),
		VolatilityPct: roundTo2(best.volatility * 100),
		SharpeRatio:   roundTo4(best.sharpe),
		Weights:       roundWeights(best.weights),
	}

	highest := efficient[highestRetIdx]
	result.HighestReturn = &OptimizedPortfolio{
		Name:          "Highest Return",
		ReturnPct:     roundTo2(highest.return_ * 100),
		VolatilityPct: roundTo2(highest.volatility * 100),
		SharpeRatio:   roundTo4(highest.sharpe),
		Weights:       roundWeights(highest.weights),
	}

	return result, nil
}



// samplePortfolios generates n-dimensional portfolios on the simplex
// using the Dirichlet(1,1,...,1) method (uniform on simplex).
func samplePortfolios(dim, count int) [][]float64 {
	rng := rand.New(rand.NewSource(42)) // deterministic for reproducibility
	portfolios := make([][]float64, 0, count)

	for i := 0; i < count; i++ {
		// Generate dim exponential(1) random numbers via -ln(U).
		sum := 0.0
		raw := make([]float64, dim)
		for j := 0; j < dim; j++ {
			raw[j] = -math.Log(rng.Float64())
			sum += raw[j]
		}
		// Normalize to simplex.
		weights := make([]float64, dim)
		for j := 0; j < dim; j++ {
			weights[j] = raw[j] / sum
		}
		portfolios = append(portfolios, weights)
	}

	return portfolios
}

// filterParetoFrontier returns only the efficient (Pareto-optimal) portfolios.
// A portfolio is efficient if no other portfolio dominates it
// (i.e. has both higher return and lower or equal volatility).
func filterParetoFrontier(evaluated []portfolioEval) []portfolioEval {
	// Sort by return ascending.
	sorted := make([]portfolioEval, len(evaluated))
	copy(sorted, evaluated)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].return_ < sorted[j].return_
	})

	var efficient []portfolioEval
	minVol := math.MaxFloat64 // minimum volatility seen so far from the right

	// Walk from highest return to lowest.
	// A point is efficient if its volatility is strictly less than all
	// points with higher return (i.e. less than the running minimum).
	for i := len(sorted) - 1; i >= 0; i-- {
		if sorted[i].volatility >= minVol {
			// Some point with higher return has lower or equal volatility.
			// This point is dominated — skip.
			continue
		}
		efficient = append(efficient, sorted[i])
		minVol = sorted[i].volatility
	}

	// Re-sort by volatility ascending for output.
	sort.Slice(efficient, func(i, j int) bool {
		return efficient[i].volatility < efficient[j].volatility
	})

	return efficient
}

// sampleFrontierPoints samples uniformly from the efficient set by volatility range.
func sampleFrontierPoints(efficient []portfolioEval, target int) []portfolioEval {
	if len(efficient) <= target {
		return efficient
	}

	// Sample evenly by index.
	step := float64(len(efficient)) / float64(target)
	sampled := make([]portfolioEval, 0, target)
	for i := 0; i < target; i++ {
		idx := int(math.Round(float64(i) * step))
		if idx >= len(efficient) {
			idx = len(efficient) - 1
		}
		sampled = append(sampled, efficient[idx])
	}
	return sampled
}

// roundTo2 rounds to 2 decimal places.
func roundTo2(v float64) float64 {
	return math.Round(v*100) / 100
}

// roundTo4 rounds to 4 decimal places.
func roundTo4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

// roundWeights rounds each weight to 4 decimal places and ensures they sum to 1.0.
func roundWeights(weights []float64) []float64 {
	rounded := make([]float64, len(weights))
	sum := 0.0
	for i, w := range weights {
		rounded[i] = math.Round(w*10000) / 10000
		sum += rounded[i]
	}
	// Adjust largest weight to ensure sum = 1.0.
	diff := 1.0 - sum
	maxIdx := 0
	for i, w := range rounded {
		if w > rounded[maxIdx] {
			maxIdx = i
		}
	}
	rounded[maxIdx] += diff
	return rounded
}

// itoa converts int to string (simple, no strconv dependency for small ints).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
