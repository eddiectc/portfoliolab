package analysis

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/stats"
	"github.com/eddiectc/portfoliolab/internal/market"
	"github.com/eddiectc/portfoliolab/internal/types/symbol"
)

const (
	// S&P 500 benchmark reference values for value/growth comparison.
	sp500RefPE = 20.0
	sp500RefPB = 4.0

	// Tilt threshold: within this fraction of benchmark → "neutral".
	tiltThresholdFraction = 0.15

	// Size classification thresholds (in USD).
	largeCapThreshold = 10_000_000_000 // $10B
	midCapThreshold   = 2_000_000_000  // $2B

	// S&P 500 reference values for quality comparison.
	// Lower P/CF and P/Sales = higher quality.
	sp500RefPCF = 10.0
	sp500RefPS  = 2.5

	// Quality threshold: within this fraction of benchmark → "neutral".
	qualityThresholdFraction = 0.20

	// Momentum thresholds (in % return).
	momentumPositiveThreshold = 2.0  // above this → positive
	momentumNegativeThreshold = -2.0 // below this → negative

	// Volatility thresholds (annualized %).
	volLowThreshold  = 10.0
	volHighThreshold = 20.0

	// Trading days per year for annualization.
	tradingDaysPerYear = 252
)

// ComputeFactorExposure computes proxy-based factor exposure metrics from
// cached valuation data and, when provided, historical price data.
//
// Static factors (always computed from SymbolDetails):
//   - Value vs Growth: portfolio-weighted P/E and P/B from EquityValuation,
//     compared to S&P 500 reference values.
//   - Size tilt: large/mid/small cap from FundProfile.TotalNetAssets.
//   - Concentration: HHI from underlying holdings (ETF look-through).
//   - Quality: portfolio-weighted P/CF and P/Sales vs S&P 500 reference.
//   - Cost: portfolio-weighted expense ratio and holdings turnover.
//
// Time-series factors (computed when pricesBySymbol is non-empty):
//   - Momentum: portfolio-weighted 3M/6M/12M returns.
//   - Volatility: portfolio-weighted annualized volatility.
func ComputeFactorExposure(positions []PositionWithDetails, pricesBySymbol map[string][]market.HistoricalPrice) *FactorExposureResult {
	if len(positions) == 0 {
		return &FactorExposureResult{
			Message: "No positions to analyze. Factor exposure requires at least one position.",
		}
	}

	var (
		peWeightedSum, pbWeightedSum                 float64
		peTrackedWeight, pbTrackedWeight             float64
		pcfWeightedSum, psWeightedSum                float64
		pcfTrackedWeight, psTrackedWeight            float64
		expenseWeightedSum, turnWeightedSum          float64
		expenseTrackedWeight, turnTrackedWeight      float64
		largeCapWeight, midCapWeight, smallCapWeight float64
		sizeTrackedWeight                            float64
		hhiSum                                       float64
		topWeight                                    float64 // as fraction 0-1
		warnings                                     []string
		peCount, pbCount, pcfCount, psCount          int
	)

	for _, p := range positions {
		if p.SymbolDetails == nil {
			warnings = append(warnings, p.Symbol+": no symbol details available — using position weight for concentration only")
			// Still include in concentration: position weight as single holding.
			holdingWeight := p.PortfolioWeight / 100.0
			hhiSum += holdingWeight * holdingWeight
			if holdingWeight > topWeight {
				topWeight = holdingWeight
			}
			continue
		}

		// Value vs Growth: weighted P/E and P/B from EquityValuation.
		valuation := p.SymbolDetails.EquityValuation
		if valuation != nil {
			pe := valuation.PriceToEarnings
			pb := valuation.PriceToBook
			pcf := valuation.PriceToCashflow
			ps := valuation.PriceToSales
			if pe > 0 {
				peWeightedSum += p.PortfolioWeight * pe
				peTrackedWeight += p.PortfolioWeight
				peCount++
			}
			if pb > 0 {
				pbWeightedSum += p.PortfolioWeight * pb
				pbTrackedWeight += p.PortfolioWeight
				pbCount++
			}
			// Quality: P/CF and P/Sales (lower = better quality).
			if pcf > 0 {
				pcfWeightedSum += p.PortfolioWeight * pcf
				pcfTrackedWeight += p.PortfolioWeight
				pcfCount++
			}
			if ps > 0 {
				psWeightedSum += p.PortfolioWeight * ps
				psTrackedWeight += p.PortfolioWeight
				psCount++
			}
		}

		// Size classification based on TotalNetAssets.
		// Cost: expense ratio and turnover from FundProfile.
		if p.SymbolDetails.FundProfile != nil {
			fund := p.SymbolDetails.FundProfile
			netAssets := fund.TotalNetAssets
			if netAssets > 0 {
				if netAssets >= largeCapThreshold {
					largeCapWeight += p.PortfolioWeight
				} else if netAssets >= midCapThreshold {
					midCapWeight += p.PortfolioWeight
				} else {
					smallCapWeight += p.PortfolioWeight
				}
				sizeTrackedWeight += p.PortfolioWeight
			}
			if fund.AnnualExpenseRatio > 0 {
				expenseWeightedSum += p.PortfolioWeight * fund.AnnualExpenseRatio
				expenseTrackedWeight += p.PortfolioWeight
			}
			if fund.AnnualHoldingsTurnover > 0 {
				turnWeightedSum += p.PortfolioWeight * fund.AnnualHoldingsTurnover
				turnTrackedWeight += p.PortfolioWeight
			}
		}

		// Concentration: decompose into underlying holdings.
		if p.SymbolDetails.QuoteType == "ETF" && len(p.SymbolDetails.TopHoldings) > 0 {
			for _, h := range p.SymbolDetails.TopHoldings {
				holdingWeight := (p.PortfolioWeight / 100.0) * (h.Percent / 100.0)
				hhiSum += holdingWeight * holdingWeight
				if holdingWeight > topWeight {
					topWeight = holdingWeight
				}
			}
		} else if p.SymbolDetails.QuoteType != "ETF" {
			// Individual stock: position weight as fraction.
			holdingWeight := p.PortfolioWeight / 100.0
			hhiSum += holdingWeight * holdingWeight
			if holdingWeight > topWeight {
				topWeight = holdingWeight
			}
		} else {
			// ETF with no holdings data — use position weight directly.
			holdingWeight := p.PortfolioWeight / 100.0
			hhiSum += holdingWeight * holdingWeight
			if holdingWeight > topWeight {
				topWeight = holdingWeight
			}
			warnings = append(warnings, p.Symbol+": no holdings data for concentration calculation")
		}
	}

	// Compute weighted P/E and P/B.
	var weightedPE, weightedPB float64
	if peTrackedWeight > 0 {
		weightedPE = stats.RoundTo2(peWeightedSum / peTrackedWeight)
	}
	if pbTrackedWeight > 0 {
		weightedPB = stats.RoundTo2(pbWeightedSum / pbTrackedWeight)
	}

	// Determine value/growth tilt.
	peTilt := classifyTilt(weightedPE, sp500RefPE)
	pbTilt := classifyTilt(weightedPB, sp500RefPB)
	tilt := combineTilts(peTilt, pbTilt)

	if peCount == 0 && pbCount == 0 {
		tilt = "unavailable"
		warnings = append(warnings, "no valuation data available for any position — value/growth tilt unavailable")
	}

	// Size tilt percentages (relative to total portfolio weight, not tracked weight).
	// This way the percentages sum to <100% when coverage is partial — the gap is the signal.
	var sizeTiltStr string
	var largeCapPct, midCapPct, smallCapPct float64
	largeCapPct = stats.RoundTo2(largeCapWeight)
	midCapPct = stats.RoundTo2(midCapWeight)
	smallCapPct = stats.RoundTo2(smallCapWeight)
	if sizeTrackedWeight > 0 {
		sizeTiltStr = classifySizeTilt(largeCapPct, midCapPct, smallCapPct)
	}

	if sizeTrackedWeight == 0 {
		warnings = append(warnings, "no size data available for any position — size tilt unavailable")
	} else if sizeTrackedWeight < 100 {
		warnings = append(warnings, fmt.Sprintf("size data available for only %.2f%% of portfolio — size tilt may not reflect full portfolio", sizeTrackedWeight))
	}

	// HHI and interpretation.
	// HHI operates in 0–1 range with meaningful differences at 4+ decimal places,
	// so use roundTo4 instead of roundTo2 (which would zero out diversified portfolios).
	hhi := stats.RoundTo4(hhiSum)
	interpretation := classifyHHI(hhi)

	// Top holding as percentage of portfolio.
	topHoldingPct := stats.RoundTo2(topWeight * 100)

	// Quality: weighted P/CF and P/Sales.
	var weightedPCF, weightedPS float64
	if pcfTrackedWeight > 0 {
		weightedPCF = stats.RoundTo2(pcfWeightedSum / pcfTrackedWeight)
	}
	if psTrackedWeight > 0 {
		weightedPS = stats.RoundTo2(psWeightedSum / psTrackedWeight)
	}

	// Quality tilt: lower P/CF and P/Sales = higher quality.
	// classifyInvertedTilt: below benchmark → "high-quality", above → "low-quality".
	pcfQuality := classifyInvertedTilt(weightedPCF, sp500RefPCF)
	psQuality := classifyInvertedTilt(weightedPS, sp500RefPS)
	qualityTilt := combineTilts(pcfQuality, psQuality)
	if pcfCount == 0 && psCount == 0 {
		qualityTilt = "unavailable"
		warnings = append(warnings, "no quality data (P/CF, P/Sales) available — quality tilt unavailable")
	}
	// Remap tilt labels for quality context.
	qualityTilt = remapQualityTilt(qualityTilt)

	// Cost: weighted expense ratio and turnover.
	var weightedExpense, weightedTurnover float64
	if expenseTrackedWeight > 0 {
		weightedExpense = stats.RoundTo2(expenseWeightedSum / expenseTrackedWeight)
	}
	if turnTrackedWeight > 0 {
		weightedTurnover = stats.RoundTo2(turnWeightedSum / turnTrackedWeight)
	}

	if expenseTrackedWeight == 0 && turnTrackedWeight == 0 {
		warnings = append(warnings, "no cost data (expense ratio, turnover) available — cost metrics unavailable")
	} else if expenseTrackedWeight < 100 || turnTrackedWeight < 100 {
		coverage := math.Max(expenseTrackedWeight, turnTrackedWeight)
		warnings = append(warnings, fmt.Sprintf("cost data available for only %.2f%% of portfolio — cost metrics may not reflect full portfolio", coverage))
	}

	// Momentum and volatility from price history.
	momentum := computeMomentum(positions, pricesBySymbol, &warnings)
	volatility := computeVolatility(positions, pricesBySymbol, &warnings)

	return &FactorExposureResult{
		ValueGrowthTilt: FactorValueGrowth{
			WeightedPE: weightedPE,
			WeightedPB: weightedPB,
			Tilt:       tilt,
		},
		SizeTilt: FactorSizeTilt{
			LargeCapPct: largeCapPct,
			MidCapPct:   midCapPct,
			SmallCapPct: smallCapPct,
			Tilt:        sizeTiltStr,
		},
		Concentration: FactorConcentration{
			HHI:            hhi,
			Interpretation: interpretation,
		},
		TopHoldingWeightPct: topHoldingPct,
		Quality: FactorQuality{
			WeightedPCF: weightedPCF,
			WeightedPS:  weightedPS,
			Tilt:        qualityTilt,
		},
		Cost: FactorCost{
			WeightedExpenseRatio: weightedExpense,
			WeightedTurnover:     weightedTurnover,
		},
		Momentum:   momentum,
		Volatility: volatility,
		Warnings:   warnings,
	}
}

// computeMomentum computes portfolio-weighted 3M/6M/12M returns from price history.
func computeMomentum(positions []PositionWithDetails, pricesBySymbol map[string][]market.HistoricalPrice, warnings *[]string) FactorMomentum {
	if len(pricesBySymbol) == 0 {
		return FactorMomentum{Tilt: "unavailable"}
	}

	now := time.Now()
	threeMonthsAgo := now.AddDate(0, -3, 0)
	sixMonthsAgo := now.AddDate(0, -6, 0)
	twelveMonthsAgo := now.AddDate(-1, 0, 0)

	var (
		return3M, return6M, return12M                      float64
		trackedWeight3M, trackedWeight6M, trackedWeight12M float64
	)

	for _, p := range positions {
		prices, ok := pricesBySymbol[p.Symbol]
		if !ok || len(prices) < 2 {
			continue
		}

		// Sort prices by date ascending.
		sorted := make([]market.HistoricalPrice, len(prices))
		copy(sorted, prices)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Date.Before(sorted[j].Date)
		})

		// Find the start and end prices for each window.
		start3M, end3M, okStart3M, okEnd3M := findReturnRange(sorted, threeMonthsAgo, now)
		start6M, end6M, okStart6M, okEnd6M := findReturnRange(sorted, sixMonthsAgo, now)
		start12M, end12M, okStart12M, okEnd12M := findReturnRange(sorted, twelveMonthsAgo, now)

		if okStart3M && okEnd3M && start3M > 0 && end3M > 0 {
			ret := (end3M/start3M - 1.0) * 100.0
			return3M += p.PortfolioWeight * ret
			trackedWeight3M += p.PortfolioWeight
		}
		if okStart6M && okEnd6M && start6M > 0 && end6M > 0 {
			ret := (end6M/start6M - 1.0) * 100.0
			return6M += p.PortfolioWeight * ret
			trackedWeight6M += p.PortfolioWeight
		}
		if okStart12M && okEnd12M && start12M > 0 && end12M > 0 {
			ret := (end12M/start12M - 1.0) * 100.0
			return12M += p.PortfolioWeight * ret
			trackedWeight12M += p.PortfolioWeight
		}
	}

	var avg3M, avg6M, avg12M float64
	if trackedWeight3M > 0 {
		avg3M = stats.RoundTo2(return3M / trackedWeight3M)
	}
	if trackedWeight6M > 0 {
		avg6M = stats.RoundTo2(return6M / trackedWeight6M)
	}
	if trackedWeight12M > 0 {
		avg12M = stats.RoundTo2(return12M / trackedWeight12M)
	}

	tilt := classifyMomentum(avg3M, avg6M, avg12M)

	// Warn on partial price coverage.
	maxTracked := math.Max(trackedWeight3M, math.Max(trackedWeight6M, trackedWeight12M))
	if maxTracked > 0 && maxTracked < 100 {
		*warnings = append(*warnings, fmt.Sprintf("momentum data available for only %.2f%% of portfolio — momentum may not reflect full portfolio", maxTracked))
	}

	return FactorMomentum{
		Return3M:  avg3M,
		Return6M:  avg6M,
		Return12M: avg12M,
		Tilt:      tilt,
	}
}

// findReturnRange finds the closest price on/before start and closest price
// on/before end in a sorted price series. The boolean flags indicate successful
// Float64 conversion. Returns (0, false) if no matching date or conversion fails.
func findReturnRange(sorted []market.HistoricalPrice, start, end time.Time) (float64, float64, bool, bool) {
	var startPrice, endPrice float64
	var foundStart, foundEnd bool
	for _, p := range sorted {
		// For start: keep overwriting to get the last (most recent) price <= start.
		if p.Date.Before(start) || p.Date.Equal(start) {
			startPrice, foundStart = p.Close.Float64()
		}
		// For end: keep overwriting to get the last (most recent) price <= end.
		if !p.Date.After(end) {
			endPrice, foundEnd = p.Close.Float64()
		}
	}
	return startPrice, endPrice, foundStart, foundEnd
}

// classifyMomentum returns the momentum tilt based on 3M/6M/12M returns.
func classifyMomentum(r3, r6, r12 float64) string {
	// If all windows are zero, no data.
	if r3 == 0 && r6 == 0 && r12 == 0 {
		return "unavailable"
	}

	// Count positive/negative signals across available windows.
	var positive, negative int
	if r3 != 0 {
		if r3 > momentumPositiveThreshold {
			positive++
		} else if r3 < momentumNegativeThreshold {
			negative++
		}
	}
	if r6 != 0 {
		if r6 > momentumPositiveThreshold {
			positive++
		} else if r6 < momentumNegativeThreshold {
			negative++
		}
	}
	if r12 != 0 {
		if r12 > momentumPositiveThreshold {
			positive++
		} else if r12 < momentumNegativeThreshold {
			negative++
		}
	}

	if positive > negative {
		return "positive"
	}
	if negative > positive {
		return "negative"
	}
	return "neutral"
}

// computeVolatility computes portfolio-weighted annualized volatility from daily returns.
func computeVolatility(positions []PositionWithDetails, pricesBySymbol map[string][]market.HistoricalPrice, warnings *[]string) FactorVolatility {
	if len(pricesBySymbol) == 0 {
		return FactorVolatility{Tilt: "unavailable"}
	}

	var (
		volWeightedSum   float64
		volTrackedWeight float64
	)

	for _, p := range positions {
		prices, ok := pricesBySymbol[p.Symbol]
		if !ok || len(prices) < 2 {
			continue
		}

		// Sort prices by date ascending.
		sorted := make([]market.HistoricalPrice, len(prices))
		copy(sorted, prices)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Date.Before(sorted[j].Date)
		})

		// Compute daily returns.
		var dailyReturns []float64
		for i := 1; i < len(sorted); i++ {
			prev, _ := sorted[i-1].Close.Float64()
			curr, _ := sorted[i].Close.Float64()
			if prev > 0 {
				dailyReturns = append(dailyReturns, (curr-prev)/prev)
			}
		}

		if len(dailyReturns) < 2 {
			continue
		}

		// Compute standard deviation of daily returns.
		var sum float64
		for _, r := range dailyReturns {
			sum += r
		}
		mean := sum / float64(len(dailyReturns))

		var variance float64
		for _, r := range dailyReturns {
			diff := r - mean
			variance += diff * diff
		}
		variance /= float64(len(dailyReturns))
		dailyStdDev := math.Sqrt(variance)

		// Annualize: daily std dev * sqrt(252).
		annualizedVol := dailyStdDev * math.Sqrt(tradingDaysPerYear) * 100.0 // as percentage

		volWeightedSum += p.PortfolioWeight * annualizedVol
		volTrackedWeight += p.PortfolioWeight
	}

	var avgVol float64
	if volTrackedWeight > 0 {
		avgVol = stats.RoundTo2(volWeightedSum / volTrackedWeight)
	}

	tilt := classifyVolatility(avgVol)

	// Warn on partial price coverage.
	if volTrackedWeight > 0 && volTrackedWeight < 100 {
		*warnings = append(*warnings, fmt.Sprintf("volatility data available for only %.2f%% of portfolio — volatility may not reflect full portfolio", volTrackedWeight))
	}

	return FactorVolatility{
		AnnualizedVol: avgVol,
		Tilt:          tilt,
	}
}

// classifyVolatility returns the volatility tilt based on annualized volatility %.
func classifyVolatility(vol float64) string {
	if vol == 0 {
		return "unavailable"
	}
	if vol <= volLowThreshold {
		return "low"
	}
	if vol <= volHighThreshold {
		return "medium"
	}
	return "high"
}

// classifyInvertedTilt is like classifyTilt but inverted: below benchmark → "value"
// (which is remapped to "high-quality" in the quality context), above → "growth"
// (remapped to "low-quality").
func classifyInvertedTilt(value, benchmark float64) string {
	if value == 0 {
		return ""
	}
	threshold := benchmark * qualityThresholdFraction
	if value < benchmark-threshold {
		return "value" // below benchmark = high quality
	}
	if value > benchmark+threshold {
		return "growth" // above benchmark = low quality
	}
	return "" // neutral
}

// remapQualityTilt remaps the generic tilt labels to quality-specific ones.
func remapQualityTilt(tilt string) string {
	switch tilt {
	case "value":
		return "high-quality"
	case "growth":
		return "low-quality"
	case "neutral":
		return "neutral"
	default:
		return tilt
	}
}

// classifyTilt returns "value", "growth", or "" (neutral/no data) based on
// comparing the given metric to the benchmark reference.
//
// A metric within ±15% of the benchmark is considered neutral.
func classifyTilt(value, benchmark float64) string {
	if value == 0 {
		return ""
	}
	threshold := benchmark * tiltThresholdFraction
	if value < benchmark-threshold {
		return "value"
	}
	if value > benchmark+threshold {
		return "growth"
	}
	return "" // neutral
}

// combineTilts combines P/E and P/B tilt classifications into a single tilt.
//
// If both agree, that tilt is returned. If one is empty (no data), the other
// is used. If both are empty, "neutral" is returned. If they disagree,
// "neutral" is returned.
func combineTilts(peTilt, pbTilt string) string {
	if peTilt == "" && pbTilt == "" {
		return "neutral"
	}
	if peTilt == pbTilt {
		return peTilt
	}
	if peTilt == "" {
		return pbTilt
	}
	if pbTilt == "" {
		return peTilt
	}
	return "neutral" // disagree
}

// classifySizeTilt returns the dominant size category or "mixed".
//
// A category must exceed 50% of tracked weight to be the dominant tilt.
func classifySizeTilt(large, mid, small float64) string {
	if large > 50 {
		return "large"
	}
	if mid > 50 {
		return "mid"
	}
	if small > 50 {
		return "small"
	}
	return "mixed"
}

// classifyHHI interprets the Herfindahl-Hirschman Index value.
//
// HHI is computed as the sum of squared weights (as fractions 0-1).
// Lower values indicate better diversification.
func classifyHHI(hhi float64) string {
	if hhi < 0.02 {
		return "well-diversified"
	}
	if hhi <= 0.05 {
		return "moderately-concentrated"
	}
	return "highly-concentrated"
}

// --- Test helpers ---

// getETFWithValuation returns a PositionWithDetails configured as an ETF
// with the given valuation data, net assets, and top holdings, for testing.
func getETFWithValuation(sym string, portfolioWeight float64, pe, pb float64, netAssets float64, holdings []symbol.TopHolding) PositionWithDetails {
	return PositionWithDetails{
		Symbol:          sym,
		PortfolioWeight: portfolioWeight,
		SymbolDetails: &symbol.SymbolDetails{
			QuoteType: "ETF",
			EquityValuation: &symbol.EquityValuation{
				PriceToEarnings: pe,
				PriceToBook:     pb,
			},
			FundProfile: &symbol.FundProfile{
				TotalNetAssets: netAssets,
			},
			TopHoldings: holdings,
		},
	}
}

// getETFWithFullData returns a PositionWithDetails configured as an ETF
// with valuation, quality (P/CF, P/Sales), cost (expense ratio, turnover),
// size, and holdings data, for testing.
func getETFWithFullData(sym string, portfolioWeight float64, pe, pb, pcf, ps float64, netAssets float64, expenseRatio, turnover float64, holdings []symbol.TopHolding) PositionWithDetails {
	return PositionWithDetails{
		Symbol:          sym,
		PortfolioWeight: portfolioWeight,
		SymbolDetails: &symbol.SymbolDetails{
			QuoteType: "ETF",
			EquityValuation: &symbol.EquityValuation{
				PriceToEarnings: pe,
				PriceToBook:     pb,
				PriceToCashflow: pcf,
				PriceToSales:    ps,
			},
			FundProfile: &symbol.FundProfile{
				TotalNetAssets:         netAssets,
				AnnualExpenseRatio:     expenseRatio,
				AnnualHoldingsTurnover: turnover,
			},
			TopHoldings: holdings,
		},
	}
}

// getStockWithNoValuation returns a PositionWithDetails configured as an
// individual stock without valuation data (stocks don't have EquityValuation
// in the cached symbol details), for testing.
func getStockWithNoValuation(sym string, portfolioWeight float64) PositionWithDetails {
	return PositionWithDetails{
		Symbol:          sym,
		PortfolioWeight: portfolioWeight,
		SymbolDetails: &symbol.SymbolDetails{
			QuoteType: "EQUITY",
		},
	}
}
