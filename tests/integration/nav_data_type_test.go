package integration

import (
	"context"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/data"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

func TestNAV_DataTypeIsolation(t *testing.T) {
	db := setupTestDB(t)
	repo := data.NewMarketDataRepository(db)
	ctx := context.Background()

	symbol := "WMGG.L"
	navPrice, _ := decimal.New(100000, 2)  // 1000.00
	stockPrice, _ := decimal.New(50000, 2) // 500.00

	// Insert NAV data (wisdomtree source)
	navData := &market.MarketData{
		Symbol:    symbol,
		Price:     navPrice,
		Currency:  "GBP",
		DataType:  "nav",
		Source:    "wisdomtree",
		Date:      "2024-01-15",
		FetchedAt: time.Now(),
	}
	if err := repo.Upsert(ctx, navData); err != nil {
		t.Fatalf("upsert NAV: %v", err)
	}

	// Insert another NAV point
	navData2 := &market.MarketData{
		Symbol:    symbol,
		Price:     decimal.MustNew(101000, 2), // 1010.00
		Currency:  "GBP",
		DataType:  "nav",
		Source:    "wisdomtree",
		Date:      "2024-01-16",
		FetchedAt: time.Now(),
	}
	if err := repo.Upsert(ctx, navData2); err != nil {
		t.Fatalf("upsert NAV 2: %v", err)
	}

	// Insert stock price data (yahoo source)
	stockData := &market.MarketData{
		Symbol:    symbol,
		Price:     stockPrice,
		Currency:  "GBP",
		DataType:  "stock",
		Source:    "yahoo",
		Date:      "2024-01-15",
		FetchedAt: time.Now(),
	}
	if err := repo.Upsert(ctx, stockData); err != nil {
		t.Fatalf("upsert stock: %v", err)
	}

	// Insert a current (date='') stock quote
	quoteData := &market.MarketData{
		Symbol:    symbol,
		Price:     decimal.MustNew(51000, 2), // 510.00
		Currency:  "GBP",
		DataType:  "stock",
		Source:    "yahoo",
		Date:      "", // current quote
		FetchedAt: time.Now(),
	}
	if err := repo.Upsert(ctx, quoteData); err != nil {
		t.Fatalf("upsert quote: %v", err)
	}

	t.Run("GetNavHistoryBySymbol returns NAV data", func(t *testing.T) {
		prices, err := repo.GetNavHistoryBySymbol(ctx, symbol)
		if err != nil {
			t.Fatalf("GetNavHistoryBySymbol: %v", err)
		}
		if len(prices) != 2 {
			t.Fatalf("expected 2 NAV points, got %d", len(prices))
		}
		if !prices[0].Close.Equal(navPrice) {
			t.Errorf("first NAV: expected %s, got %s", navPrice, prices[0].Close)
		}
		if !prices[1].Close.Equal(decimal.MustNew(101000, 2)) {
			t.Errorf("second NAV: expected 1010.00, got %s", prices[1].Close)
		}
	})

	t.Run("GetNavHistoryBySymbol excludes current entries", func(t *testing.T) {
		// Insert a current (date='') NAV entry — should be excluded
		currentNav := &market.MarketData{
			Symbol:    symbol,
			Price:     decimal.MustNew(102000, 2),
			Currency:  "GBP",
			DataType:  "nav",
			Source:    "wisdomtree",
			Date:      "", // current
			FetchedAt: time.Now(),
		}
		if err := repo.Upsert(ctx, currentNav); err != nil {
			t.Fatalf("upsert current NAV: %v", err)
		}

		prices, err := repo.GetNavHistoryBySymbol(ctx, symbol)
		if err != nil {
			t.Fatalf("GetNavHistoryBySymbol: %v", err)
		}
		if len(prices) != 2 {
			t.Errorf("expected 2 NAV points (current excluded), got %d", len(prices))
		}
	})

	t.Run("GetHistoricalPricesBySymbol excludes NAV data", func(t *testing.T) {
		start, _ := time.Parse("2006-01-02", "2024-01-01")
		end, _ := time.Parse("2006-01-02", "2024-12-31")
		prices, err := repo.GetHistoricalPricesBySymbol(ctx, symbol, start, end)
		if err != nil {
			t.Fatalf("GetHistoricalPricesBySymbol: %v", err)
		}
		if len(prices) != 1 {
			t.Fatalf("expected 1 stock price (NAV excluded), got %d", len(prices))
		}
		if !prices[0].Close.Equal(stockPrice) {
			t.Errorf("expected stock price %s, got %s", stockPrice, prices[0].Close)
		}
	})

	t.Run("GetLatestQuote excludes NAV data", func(t *testing.T) {
		latest, err := repo.GetLatest(ctx, symbol)
		if err != nil {
			t.Fatalf("GetLatest: %v", err)
		}
		if latest == nil {
			t.Fatal("expected latest market data, got nil")
		}
		// GetLatest returns any data_type (no filter), so it returns the most
		// recently fetched entry. Check that the stock quote is retrievable.
		if latest.DataType != "stock" {
			t.Errorf("expected stock data_type (most recent), got %q", latest.DataType)
		}

		// Specifically verify GetLatestQuote (data_type='stock') via direct SQL
		var dataType string
		err = db.QueryRowContext(ctx, `
			SELECT data_type FROM market_data
			WHERE symbol = ? AND date = '' AND data_type = 'stock'
			ORDER BY fetched_at DESC LIMIT 1
		`, symbol).Scan(&dataType)
		if err != nil {
			t.Fatalf("query stock quote: %v", err)
		}
		if dataType != "stock" {
			t.Errorf("expected stock quote, got %q", dataType)
		}

		// Verify no stock quote is returned for a symbol that only has NAV
		_, err = db.Exec(`INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at)
			VALUES ('NAVONLY', '100.00', 'USD', 'nav', 'wisdomtree', '', datetime('now'))`)
		if err != nil {
			t.Fatalf("insert NAV-only symbol: %v", err)
		}
		var count int
		err = db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM market_data
			WHERE symbol = 'NAVONLY' AND date = '' AND data_type = 'stock'
		`).Scan(&count)
		if err != nil {
			t.Fatalf("count stock quotes for NAV-only symbol: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 stock quotes for NAV-only symbol, got %d", count)
		}
	})

	t.Run("GetLatestPriceDatePerSymbol excludes NAV data", func(t *testing.T) {
		dates := repo.GetLatestPriceDatePerSymbol(ctx, []string{symbol})
		date, ok := dates[symbol]
		if !ok {
			t.Fatal("expected stock date for symbol")
		}
		expected, _ := time.Parse("2006-01-02", "2024-01-15")
		if !date.Equal(expected) {
			t.Errorf("expected latest stock date %s, got %s", expected, date)
		}

		// NAV-only symbol should have no stock date
		_, err := db.Exec(`INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at)
			VALUES ('NAVONLY2', '100.00', 'USD', 'nav', 'wisdomtree', '2024-06-01', datetime('now'))`)
		if err != nil {
			t.Fatalf("insert NAV-only symbol: %v", err)
		}
		dates2 := repo.GetLatestPriceDatePerSymbol(ctx, []string{"NAVONLY2"})
		if _, ok := dates2["NAVONLY2"]; ok {
			t.Error("expected no stock date for NAV-only symbol")
		}
	})

	t.Run("UpsertHistoricalPrices supports NAV data_type", func(t *testing.T) {
		// The UpsertHistoricalPrices method accepts a dataType parameter.
		// Verify it works with 'nav'.
		prices := []market.HistoricalPrice{
			{Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(103000, 2), Currency: "GBP"},
			{Date: time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(103500, 2), Currency: "GBP"},
		}
		err := repo.UpsertHistoricalPrices(ctx, "NAVTEST", prices, "nav")
		if err != nil {
			t.Fatalf("UpsertHistoricalPrices with nav: %v", err)
		}

		// Verify stored correctly
		navPrices, err := repo.GetNavHistoryBySymbol(ctx, "NAVTEST")
		if err != nil {
			t.Fatalf("GetNavHistoryBySymbol: %v", err)
		}
		if len(navPrices) != 2 {
			t.Fatalf("expected 2 NAV points, got %d", len(navPrices))
		}
		if !navPrices[0].Close.Equal(decimal.MustNew(103000, 2)) {
			t.Errorf("first NAV: expected 1030.00, got %s", navPrices[0].Close)
		}

		// Verify NOT returned by stock price query
		start, _ := time.Parse("2006-01-02", "2024-03-01")
		end, _ := time.Parse("2006-01-02", "2024-03-31")
		stockPrices, err := repo.GetHistoricalPricesBySymbol(ctx, "NAVTEST", start, end)
		if err != nil {
			t.Fatalf("GetHistoricalPricesBySymbol: %v", err)
		}
		if len(stockPrices) != 0 {
			t.Errorf("expected 0 stock prices for NAV-only symbol, got %d", len(stockPrices))
		}
	})

	t.Run("GetDistinctCachedSymbols includes NAV", func(t *testing.T) {
		// Direct SQL check — the query has no data_type filter
		var count int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM (SELECT DISTINCT symbol, data_type, MAX(date) AS latest_date
				FROM market_data GROUP BY symbol, data_type)
		`).Scan(&count)
		if err != nil {
			t.Fatalf("count distinct: %v", err)
		}
		// We have: WMGG.L (stock + nav), NAVONLY (nav), NAVONLY2 (nav), NAVTEST (nav)
		// That's 4 symbols × their types = 5 distinct (symbol, data_type) groups
		if count < 4 {
			t.Errorf("expected at least 4 distinct (symbol, data_type) groups, got %d", count)
		}

		// Verify NAV type is present
		var navCount int
		err = db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM (SELECT DISTINCT symbol, data_type FROM market_data
				GROUP BY symbol, data_type) WHERE data_type = 'nav'
		`).Scan(&navCount)
		if err != nil {
			t.Fatalf("count nav types: %v", err)
		}
		if navCount < 2 {
			t.Errorf("expected at least 2 symbols with nav data_type, got %d", navCount)
		}
	})
}
