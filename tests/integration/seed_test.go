package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/api"
	"codeberg.org/eddiectc/portfoliolab/internal/data"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/seed"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
)

// seedWithLotIDs mimics seed.Service.SeedDemo's lot ID assignment so the
// store tests exercise the same dataset the service writes.
func seedWithLotIDs() *seed.Dataset {
	d := seed.NewDataset()
	for i := range d.Accounts {
		for j := range d.Accounts[i].Transactions {
			tx := &d.Accounts[i].Transactions[j]
			if tx.Type == "buy" || tx.Type == "sell" {
				lotID := transaction.GenerateLotID()
				tx.LotID = &lotID
			}
		}
	}
	return d
}

func TestSeedStoreWritesDataset(t *testing.T) {
	db := setupTestDB(t)
	store := data.NewSeedStore(db)
	ctx := context.Background()

	res, err := store.Seed(ctx, seedWithLotIDs())
	if err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
	if res.Accounts != 2 || res.Transactions != 10 || res.Mappings != 4 || res.MappingsReused != 0 || res.ModelReused {
		t.Errorf("result = %+v, want accounts 2, transactions 10, mappings 4, reused 0, modelReused false", res)
	}

	var portfolioName, portfolioCurrency string
	if err := db.QueryRow(`SELECT name, currency FROM portfolios`).Scan(&portfolioName, &portfolioCurrency); err != nil {
		t.Fatalf("portfolio query: %v", err)
	}
	if portfolioName != "Sample" || portfolioCurrency != "GBP" {
		t.Errorf("portfolio = %s/%s, want Sample/GBP", portfolioName, portfolioCurrency)
	}

	// Symbol mappings: all four, with D9 data (source URLs on the two
	// Vanguard benchmarks only).
	var mappings int
	if err := db.QueryRow(`SELECT COUNT(*) FROM symbol_mappings`).Scan(&mappings); err != nil {
		t.Fatalf("count mappings: %v", err)
	}
	if mappings != 4 {
		t.Errorf("symbol_mappings rows = %d, want 4", mappings)
	}
	checkMapping := func(symbol string, wantBenchmark bool, wantURLPrefix string) {
		t.Helper()
		var benchmark bool
		var url sql.NullString
		if err := db.QueryRow(`SELECT is_benchmark, data_source_url FROM symbol_mappings WHERE internal_symbol = ?`, symbol).
			Scan(&benchmark, &url); err != nil {
			t.Fatalf("mapping %s: %v", symbol, err)
		}
		if benchmark != wantBenchmark {
			t.Errorf("%s is_benchmark = %v, want %v", symbol, benchmark, wantBenchmark)
		}
		if wantURLPrefix == "" {
			if url.Valid {
				t.Errorf("%s data_source_url = %q, want NULL", symbol, url.String)
			}
		} else if !url.Valid || !strings.HasPrefix(url.String, wantURLPrefix) {
			t.Errorf("%s data_source_url = %v, want prefix %q", symbol, url.String, wantURLPrefix)
		}
	}
	checkMapping("VWRP.L", true, "https://www.vanguardinvestor.co.uk/investments/vanguard-ftse-all-world-ucits-etf")
	checkMapping("VUTA.L", true, "https://www.vanguardinvestor.co.uk/investments/vanguard-usd-treasury-bond-ucits-etf")
	checkMapping("GOOG", false, "")
	checkMapping("BRK-B", false, "")

	// Model portfolio with entries.
	var modelCount int
	var entries string
	if err := db.QueryRow(`SELECT COUNT(*) FROM model_portfolios`).Scan(&modelCount); err != nil {
		t.Fatalf("count models: %v", err)
	}
	if modelCount != 1 {
		t.Fatalf("model_portfolios rows = %d, want 1", modelCount)
	}
	if err := db.QueryRow(`SELECT entries FROM model_portfolios WHERE name = 'Sample Model'`).Scan(&entries); err != nil {
		t.Fatalf("model entries: %v", err)
	}
	if !strings.Contains(entries, "VWRP.L") || !strings.Contains(entries, "70") ||
		!strings.Contains(entries, "VUTA.L") || !strings.Contains(entries, "30") {
		t.Errorf("model entries JSON = %s, want VWRP.L 70 + VUTA.L 30", entries)
	}

	// Transactions: 10 total; spot-check the tricky signed values.
	var txCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&txCount); err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if txCount != 10 {
		t.Errorf("transactions rows = %d, want 10", txCount)
	}
	spotCheck := func(symbol, txType, wantQty, wantPrice, wantNetCash string) {
		t.Helper()
		var qty, price, netCash string
		err := db.QueryRow(`SELECT quantity, price, net_cash FROM transactions WHERE symbol = ? AND type = ?`, symbol, txType).
			Scan(&qty, &price, &netCash)
		if err != nil {
			t.Fatalf("transaction %s %s: %v", txType, symbol, err)
		}
		if qty != wantQty || price != wantPrice || netCash != wantNetCash {
			t.Errorf("tx %s %s = qty %s price %s net_cash %s, want %s %s %s", txType, symbol, qty, price, netCash, wantQty, wantPrice, wantNetCash)
		}
	}
	spotCheck("VWRP.L", "sell", "-18", "144.60", "2602.80") // sell: negative qty, positive net cash
	spotCheck("$CASH-GBP", "deposit", "100000.00", "1.00", "100000.00")
	spotCheck("$CASH-USD", "deposit", "30000.00", "1.00", "30000.00")
	spotCheck("VWRP.L", "buy", "666", "105.00", "-69930.00")

	// Target allocations.
	var targets int
	if err := db.QueryRow(`SELECT COUNT(*) FROM target_allocations`).Scan(&targets); err != nil {
		t.Fatalf("count targets: %v", err)
	}
	if targets != 2 {
		t.Errorf("target_allocations rows = %d, want 2", targets)
	}

	// Lot IDs: exactly 8 non-null, all LOT- prefixed.
	var withLots, withoutLots int
	if err := db.QueryRow(`SELECT
		SUM(CASE WHEN lot_id IS NOT NULL THEN 1 ELSE 0 END),
		SUM(CASE WHEN lot_id IS NULL THEN 1 ELSE 0 END) FROM transactions`).Scan(&withLots, &withoutLots); err != nil {
		t.Fatalf("lot id counts: %v", err)
	}
	if withLots != 8 || withoutLots != 2 {
		t.Errorf("lot_id non-null = %d, null = %d, want 8/2", withLots, withoutLots)
	}

	// Second seed on a populated deployment must be skipped, not duplicate.
	if _, err := store.Seed(ctx, seedWithLotIDs()); !errors.Is(err, seed.ErrPortfolioExists) {
		t.Errorf("second Seed() error = %v, want ErrPortfolioExists", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&txCount); err != nil {
		t.Fatalf("count transactions after reseed: %v", err)
	}
	if txCount != 10 {
		t.Errorf("transactions after reseed = %d, want 10 (unchanged)", txCount)
	}
}

func TestSeedStoreReusesMappingsAndModel(t *testing.T) {
	db := setupTestDB(t)
	store := data.NewSeedStore(db)
	ctx := context.Background()

	first, err := store.Seed(ctx, seedWithLotIDs())
	if err != nil {
		t.Fatalf("first Seed() error = %v", err)
	}

	// Delete the sample portfolio through the real API (cascades to
	// accounts, transactions, targets; mappings and model remain).
	router := newTestRouter(t, db)
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/portfolios/%d", first.PortfolioID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent && w.Code != http.StatusOK {
		t.Fatalf("DELETE portfolio status = %d, want 200/204", w.Code)
	}

	second, err := store.Seed(ctx, seedWithLotIDs())
	if err != nil {
		t.Fatalf("second Seed() error = %v", err)
	}
	if second.Mappings != 0 || second.MappingsReused != 4 || !second.ModelReused {
		t.Errorf("second result = %+v, want mappings 0, reused 4, modelReused true", second)
	}

	// Shared tables were not duplicated.
	var m, mp int
	if err := db.QueryRow(`SELECT COUNT(*) FROM symbol_mappings`).Scan(&m); err != nil {
		t.Fatalf("count mappings: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM model_portfolios`).Scan(&mp); err != nil {
		t.Fatalf("count models: %v", err)
	}
	if m != 4 || mp != 1 {
		t.Errorf("after reseed: mappings = %d, models = %d, want 4 and 1", m, mp)
	}
	// The new portfolio's data is present again.
	var accounts, txs int
	if err := db.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&accounts); err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&txs); err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if accounts != 2 || txs != 10 {
		t.Errorf("after reseed: accounts = %d, transactions = %d, want 2 and 10", accounts, txs)
	}
}

// newSeedService builds the real seed service the way the app bootstrap does
// (real store + real position service from the real repositories).
func newSeedService(t *testing.T, db *sql.DB) *seed.Service {
	t.Helper()
	portfolioRepo := data.NewPortfolioRepository(db)
	accountRepo := data.NewAccountRepository(db)
	positionSvc := position.NewService(
		data.NewPositionRepository(db),
		data.NewTransactionRepository(db),
		data.NewAccountChecker(accountRepo),
		data.NewPortfolioChecker(portfolioRepo),
		data.NewAccountLister(accountRepo),
		data.NewPortfolioCurrencyChecker(portfolio.NewService(portfolioRepo)),
	)
	return seed.NewService(data.NewSeedStore(db), positionSvc, testLogger())
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestRouterSeedsSampleDataByDefault(t *testing.T) {
	db := setupTestDB(t)

	// Build the router the same way main.go does (default options → seed on),
	// with a stubbed market fetcher so no network call is made. The seed runs
	// synchronously before Router returns, i.e. before market cache Start.
	_, _ = api.Router(db, testLogger(),
		api.WithTemplatesDir("../../templates"),
		api.WithMarketDataFetcher(stubMarketFetcher{}))

	var name, currency string
	if err := db.QueryRow(`SELECT name, currency FROM portfolios`).Scan(&name, &currency); err != nil {
		t.Fatalf("seeded portfolio: %v", err)
	}
	if name != "Sample" || currency != "GBP" {
		t.Errorf("seeded portfolio = %s/%s, want Sample/GBP", name, currency)
	}
	if n := countRows(t, db, "transactions"); n != 10 {
		t.Errorf("transactions = %d, want 10", n)
	}
	if n := countRows(t, db, "positions"); n != 6 {
		t.Errorf("positions = %d, want 6 (recalculated during seed)", n)
	}

	// Second construction on the now-populated DB: skip, no duplicates.
	_, _ = api.Router(db, testLogger(),
		api.WithTemplatesDir("../../templates"),
		api.WithMarketDataFetcher(stubMarketFetcher{}))
	if n := countRows(t, db, "portfolios"); n != 1 {
		t.Errorf("portfolios after second Router = %d, want 1", n)
	}
	if n := countRows(t, db, "transactions"); n != 10 {
		t.Errorf("transactions after second Router = %d, want 10", n)
	}
}

func TestSeedSkipsWhenUserPortfolioExists(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a user portfolio through the real API first.
	router := newTestRouter(t, db)
	body := strings.NewReader(`{"name": "My Portfolio", "currency": "EUR"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", body)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST /api/portfolios status = %d, want 201", w.Code)
	}

	if err := newSeedService(t, db).SeedDemo(ctx); err != nil {
		t.Fatalf("SeedDemo() with existing user data error = %v, want nil (skip)", err)
	}

	if n := countRows(t, db, "portfolios"); n != 1 {
		t.Errorf("portfolios = %d, want 1 (user's only)", n)
	}
	for _, table := range []string{"accounts", "transactions", "symbol_mappings", "model_portfolios", "target_allocations", "positions", "symbol_details"} {
		if n := countRows(t, db, table); n != 0 {
			t.Errorf("%s rows = %d, want 0 (seed must not touch a deployment with user data)", table, n)
		}
	}
}

func TestSeedStoreAtomicity(t *testing.T) {
	db := setupTestDB(t)
	store := data.NewSeedStore(db)

	// Corrupted fixture: duplicate target symbol violates
	// UNIQUE(portfolio_id, symbol).
	d := seedWithLotIDs()
	dupe := d.PortfolioTargets[0]
	d.PortfolioTargets = append(d.PortfolioTargets, dupe)

	if _, err := store.Seed(context.Background(), d); err == nil {
		t.Fatal("Seed() with duplicate target symbol succeeded, want UNIQUE violation")
	}

	for _, table := range []string{"portfolios", "accounts", "transactions", "symbol_mappings", "model_portfolios", "target_allocations", "positions", "symbol_details"} {
		if n := countRows(t, db, table); n != 0 {
			t.Errorf("%s rows after failed seed = %d, want 0 (full rollback)", table, n)
		}
	}
}

func TestSeedStoreSymbolOverlap(t *testing.T) {
	db := setupTestDB(t)
	store := data.NewSeedStore(db)
	ctx := context.Background()

	// A user mapping whose market data symbol overlaps a sample one.
	if _, err := db.Exec(`INSERT INTO symbol_mappings (internal_symbol, market_data_symbol, is_benchmark, created_at, updated_at) VALUES ('google-2', 'GOOG', 0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert user mapping: %v", err)
	}

	res, err := store.Seed(ctx, seedWithLotIDs())
	if err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
	if res.Mappings != 4 || res.MappingsReused != 0 {
		t.Errorf("result = %+v, want mappings 4 (new), reused 0 (reused by internal_symbol only)", res)
	}
	if n := countRows(t, db, "symbol_mappings"); n != 5 {
		t.Errorf("symbol_mappings rows = %d, want 5 (user + 4 sample)", n)
	}
	// User mapping untouched.
	var internal, market string
	var benchmark bool
	var url sql.NullString
	if err := db.QueryRow(`SELECT internal_symbol, market_data_symbol, is_benchmark, data_source_url FROM symbol_mappings WHERE id = 1`).
		Scan(&internal, &market, &benchmark, &url); err != nil {
		t.Fatalf("user mapping: %v", err)
	}
	if internal != "google-2" || market != "GOOG" || benchmark || url.Valid {
		t.Errorf("user mapping modified: %s -> %s (benchmark %v, url %v)", internal, market, benchmark, url.String)
	}
	// Sample created its own GOOG mapping.
	if n := countRows(t, db, "symbol_mappings"); n != 5 {
		t.Fatalf("mappings = %d, want 5", n)
	}
	var sampleMarket string
	if err := db.QueryRow(`SELECT market_data_symbol FROM symbol_mappings WHERE internal_symbol = 'GOOG'`).Scan(&sampleMarket); err != nil {
		t.Fatalf("sample GOOG mapping: %v", err)
	}
	if sampleMarket != "GOOG" {
		t.Errorf("sample GOOG market_data_symbol = %s, want GOOG", sampleMarket)
	}
}

// TestSeedServiceFullPathWithRecalculation exercises the startup path:
// seed service + real store + real position recalculation on an in-memory
// DB, then verifies the derived positions (including cash) via the API and
// DB.
func TestSeedServiceFullPathWithRecalculation(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	svc := newSeedService(t, db)
	if err := svc.SeedDemo(ctx); err != nil {
		t.Fatalf("SeedDemo() error = %v", err)
	}

	// Portfolio API sees the seeded portfolio.
	router := newTestRouter(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/portfolios status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"Sample"`) || !strings.Contains(w.Body.String(), `"GBP"`) {
		t.Errorf("portfolios response = %s, want Sample/GBP", w.Body.String())
	}

	// Derived positions: quantities, average prices, cash balances.
	checkPosition := func(symbol, wantQty string, wantAvg string, avgEpsilon float64) {
		t.Helper()
		var qty, avg string
		err := db.QueryRow(`SELECT quantity, COALESCE(avg_open_price, '') FROM positions
			WHERE symbol = ? AND is_closed = 0`, symbol).Scan(&qty, &avg)
		if err != nil {
			t.Fatalf("position %s: %v", symbol, err)
		}
		if qty != wantQty {
			t.Errorf("%s quantity = %s, want %s", symbol, qty, wantQty)
		}

		if wantAvg != "" {
			got, err := strconv.ParseFloat(avg, 64)
			if err != nil {
				t.Fatalf("parse %s avg %q: %v", symbol, avg, err)
			}
			want, _ := strconv.ParseFloat(wantAvg, 64)
			if got < want-avgEpsilon || got > want+avgEpsilon {
				t.Errorf("%s avg_open_price = %s, want ≈ %s", symbol, avg, wantAvg)
			}
		}
	}
	checkPosition("VWRP.L", "648", "105.00", 0.000001)
	checkPosition("VUTA.L", "1630", "19.6966871", 0.000001)
	checkPosition("GOOG", "44", "148.8454545", 0.000001)
	checkPosition("BRK-B", "13", "412.8076923", 0.000001)

	checkCash := func(symbol, want string) {
		t.Helper()
		var qty string
		if err := db.QueryRow(`SELECT quantity FROM positions WHERE symbol = ?`, symbol).Scan(&qty); err != nil {
			t.Fatalf("cash position %s: %v", symbol, err)
		}
		got, err := decimal.Parse(qty)
		if err != nil {
			t.Fatalf("parse cash %s %q: %v", symbol, qty, err)
		}
		wantDec, _ := decimal.Parse(want)
		if !got.Equal(wantDec) {
			t.Errorf("cash %s = %s, want %s", symbol, qty, want)
		}
	}
	checkCash("$CASH-GBP", "567.20")
	checkCash("$CASH-USD", "18084.30")

	// Realized P&L: the rebalance sell is a partial sale, so the open
	// VWRP.L position carries realized_pnl 0 by design (realized P&L only
	// applies to fully closed positions — see position_computation.go).
	var realized string
	if err := db.QueryRow(`SELECT realized_pnl FROM positions WHERE symbol = 'VWRP.L' AND is_closed = 0`).Scan(&realized); err != nil {
		t.Fatalf("realized pnl: %v", err)
	}
	if realized != "0" {
		t.Errorf("VWRP.L realized_pnl = %s, want 0 (partial sale, position still open)", realized)
	}

	// Idempotency: a second SeedDemo on the now-populated deployment skips
	// everything and leaves all rows untouched.
	if err := svc.SeedDemo(ctx); err != nil {
		t.Fatalf("second SeedDemo() error = %v", err)
	}
	if n := countRows(t, db, "portfolios"); n != 1 {
		t.Errorf("portfolios after second SeedDemo = %d, want 1", n)
	}
	if n := countRows(t, db, "transactions"); n != 10 {
		t.Errorf("transactions after second SeedDemo = %d, want 10 (unchanged)", n)
	}
	if n := countRows(t, db, "symbol_mappings"); n != 4 {
		t.Errorf("symbol_mappings after second SeedDemo = %d, want 4 (no duplicates)", n)
	}
	if n := countRows(t, db, "positions"); n != 6 {
		t.Errorf("positions after second SeedDemo = %d, want 6 (4 open symbols + 2 cash, unchanged)", n)
	}
	if n := countRows(t, db, "symbol_details"); n != 0 {
		t.Errorf("symbol_details after second SeedDemo = %d, want 0 (D9)", n)
	}
}

func TestSeedServiceFreshSeedLeavesNoDetailsOrClosedPositions(t *testing.T) {
	db := setupTestDB(t)
	if err := newSeedService(t, db).SeedDemo(context.Background()); err != nil {
		t.Fatalf("SeedDemo() error = %v", err)
	}
	// D9: the seed embeds no symbol details.
	if n := countRows(t, db, "symbol_details"); n != 0 {
		t.Errorf("symbol_details rows = %d, want 0 (D9)", n)
	}
	// The rebalance sell is a partial sale, so nothing is fully closed.
	var closed int
	if err := db.QueryRow(`SELECT COUNT(*) FROM positions WHERE is_closed = 1`).Scan(&closed); err != nil {
		t.Fatalf("count closed positions: %v", err)
	}
	if closed != 0 {
		t.Errorf("closed positions = %d, want 0", closed)
	}
	// 6 open positions: 4 symbols + 2 cash.
	var open int
	if err := db.QueryRow(`SELECT COUNT(*) FROM positions WHERE is_closed = 0`).Scan(&open); err != nil {
		t.Fatalf("count open positions: %v", err)
	}
	if open != 6 {
		t.Errorf("open positions = %d, want 6", open)
	}
}
