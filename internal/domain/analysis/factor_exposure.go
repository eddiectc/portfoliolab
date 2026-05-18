package analysis

import (
	"fmt"

	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
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
)

// ComputeFactorExposure computes proxy-based factor exposure metrics from
// cached valuation data (P/E, P/B, market cap, concentration).
//
// Value vs Growth: portfolio-weighted P/E and P/B from EquityValuation,
// compared to S&P 500 reference values. Positions without valuation data
// are excluded from the weighted average.
//
// Size tilt: large/mid/small cap classification based on FundProfile.
// TotalNetAssets. Positions without size data are excluded.
//
// Concentration: Herfindahl-Hirschman Index (HHI) computed from underlying
// holdings weights. For ETFs, looks through to top holdings. For individual
// stocks, uses the position weight directly.
//
// Top holding weight: largest single underlying holding as % of portfolio.
func ComputeFactorExposure(positions []PositionWithDetails) *FactorExposureResult {
	if len(positions) == 0 {
		return &FactorExposureResult{
			Message: "No positions to analyze. Factor exposure requires at least one position.",
		}
	}

	var (
		peWeightedSum, pbWeightedSum    float64
		peTrackedWeight, pbTrackedWeight float64
		largeCapWeight, midCapWeight, smallCapWeight float64
		sizeTrackedWeight float64
		hhiSum float64
		topWeight float64 // as fraction 0-1
		warnings []string
		peCount, pbCount int
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
		}

		// Size classification based on TotalNetAssets.
		if p.SymbolDetails.FundProfile != nil {
			netAssets := p.SymbolDetails.FundProfile.TotalNetAssets
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
		weightedPE = roundTo2(peWeightedSum / peTrackedWeight)
	}
	if pbTrackedWeight > 0 {
		weightedPB = roundTo2(pbWeightedSum / pbTrackedWeight)
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
	largeCapPct = roundTo2(largeCapWeight)
	midCapPct = roundTo2(midCapWeight)
	smallCapPct = roundTo2(smallCapWeight)
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
	hhi := roundTo4(hhiSum)
	interpretation := classifyHHI(hhi)

	// Top holding as percentage of portfolio.
	topHoldingPct := roundTo2(topWeight * 100)

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
		Warnings:            warnings,
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
