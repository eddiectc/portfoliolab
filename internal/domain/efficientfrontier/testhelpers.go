package efficientfrontier

import (
	"math"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// marketHistoricalPrice is a simplified alias used in test fixtures.
// It is the same as market.HistoricalPrice but used for type clarity in tests.
type marketHistoricalPrice = market.HistoricalPrice

// testDec converts a float64 to decimal.Decimal for test fixtures.
// This is a duplicate of the `dec` function in returns_test.go.
// It must live here (non-_test.go) because makeTestPrices is in a non-test file,
// and Go only compiles _test.go files during test builds.
func testDec(f float64) decimal.Decimal {
	d, _ := decimal.NewFromFloat64(f)
	return d
}

// makeTestPrices creates test price data for the given number of symbols.
// Each symbol gets a different starting price and oscillation pattern
// to produce meaningful returns and correlations.
func makeTestPrices(numSymbols int) map[string][]market.HistoricalPrice {
	now := time.Now()
	prices := make(map[string][]market.HistoricalPrice)

	baseNames := []string{"AAPL", "MSFT", "GOOGL", "AMZN", "META", "NVDA", "TSLA", "JPM", "V", "JNJ"}
	patterns := [][]float64{
		{0.01, -0.005, 0.008, -0.003, 0.002, -0.001, 0.006},
		{0.005, -0.008, 0.01, -0.002, 0.003, -0.004, 0.007},
		{-0.003, 0.01, -0.006, 0.004, -0.002, 0.008, -0.001},
		{0.008, -0.002, 0.003, -0.007, 0.005, -0.003, 0.001},
		{-0.005, 0.007, -0.001, 0.006, -0.004, 0.002, -0.008},
		{0.003, -0.006, 0.009, -0.002, 0.001, -0.005, 0.004},
		{-0.007, 0.004, -0.003, 0.008, -0.001, 0.006, -0.002},
		{0.002, -0.004, 0.005, -0.008, 0.007, -0.001, 0.003},
		{-0.001, 0.008, -0.005, 0.003, -0.006, 0.004, -0.007},
		{0.006, -0.003, 0.001, -0.005, 0.008, -0.004, 0.002},
	}

	for i := 0; i < numSymbols; i++ {
		sym := baseNames[i%len(baseNames)]
		pattern := patterns[i%len(patterns)]
		startPrice := 100.0 + float64(i)*50.0

		// Generate 260 price points (~1 year of trading days).
		changes := make([]float64, 0, 260)
		for len(changes) < 260 {
			for _, c := range pattern {
				if len(changes) >= 260 {
					break
				}
				// Add small noise to avoid perfect periodicity.
				changes = append(changes, c*(1+0.1*math.Sin(float64(len(changes)))))
			}
		}

		price := startPrice
		for j := 0; j < 260; j++ {
			if j > 0 {
				price *= (1 + changes[j])
			}
			prices[sym] = append(prices[sym], market.HistoricalPrice{
				Date:     now.AddDate(0, 0, -(260 - j)),
				Close:    testDec(price),
				Currency: "USD",
			})
		}
	}

	return prices
}
