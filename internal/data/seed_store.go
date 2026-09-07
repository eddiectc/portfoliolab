package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/seed"
)

// SeedStore writes the f030 sample dataset. The whole write runs in one
// database transaction: any error rolls back everything, so a partial seed
// can never remain. Symbol mappings and the model portfolio are reused when
// they already exist (never modified) so deleting and re-seeding the sample
// portfolio does not duplicate shared data.
type SeedStore struct {
	db *sql.DB
}

// NewSeedStore creates a seed store on the given database.
func NewSeedStore(db *sql.DB) *SeedStore {
	return &SeedStore{db: db}
}

// Seed persists the dataset atomically (see package docs). Returns the
// resulting counts for the startup log, or seed.ErrPortfolioExists when any
// portfolio already exists (nothing is written).
func (s *SeedStore) Seed(ctx context.Context, d *seed.Dataset) (*seed.Result, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin seed transaction: %w", err)
	}
	// Rolling back a committed transaction is a no-op.
	defer func() { _ = tx.Rollback() }()

	// Guard: the sample seed only runs on a deployment with no portfolios.
	var one int
	err = tx.QueryRowContext(ctx, "SELECT 1 FROM portfolios LIMIT 1").Scan(&one)
	if err == nil {
		return nil, seed.ErrPortfolioExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check existing portfolios: %w", err)
	}

	nowStr := time.Now().Format(time.RFC3339)

	// Portfolio.
	portfolioRes, err := tx.ExecContext(ctx,
		`INSERT INTO portfolios (name, currency, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		d.Portfolio.Name, d.Portfolio.Currency, nowStr, nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("insert portfolio %q: %w", d.Portfolio.Name, err)
	}
	portfolioID, err := portfolioRes.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("portfolio insert id: %w", err)
	}

	// Symbol mappings (mappings only — details are fetched by the startup
	// background refresh, D9). Reuse by internal_symbol; never update.
	mappings, reused := 0, 0
	for i := range d.Symbols {
		sm := &d.Symbols[i]
		var id int64
		err := tx.QueryRowContext(ctx,
			"SELECT id FROM symbol_mappings WHERE internal_symbol = ?", sm.InternalSymbol,
		).Scan(&id)
		if err == nil {
			reused++
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("check symbol mapping %s: %w", sm.InternalSymbol, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO symbol_mappings (internal_symbol, market_data_symbol, is_benchmark, data_source_url, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			sm.InternalSymbol, sm.MarketDataSymbol, sm.IsBenchmark, toSQLNullString(sm.DataSourceURL), nowStr, nowStr,
		); err != nil {
			return nil, fmt.Errorf("insert symbol mapping %s: %w", sm.InternalSymbol, err)
		}
		mappings++
	}

	// Model portfolio: reuse by name (entries live in the `entries` JSON
	// column; a reused model is not touched).
	modelReused := false
	err = tx.QueryRowContext(ctx,
		"SELECT id FROM model_portfolios WHERE name = ?", d.Model.Name,
	).Scan(&sql.NullInt64{})
	switch {
	case err == nil:
		modelReused = true
	case errors.Is(err, sql.ErrNoRows):
		entriesJSON, err := json.Marshal(d.Model.Entries)
		if err != nil {
			return nil, fmt.Errorf("marshal model entries: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO model_portfolios (name, entries, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			d.Model.Name, string(entriesJSON), nowStr, nowStr,
		); err != nil {
			return nil, fmt.Errorf("insert model portfolio %q: %w", d.Model.Name, err)
		}
	default:
		return nil, fmt.Errorf("check model portfolio %q: %w", d.Model.Name, err)
	}

	// Accounts and their transactions (deposits first per account).
	transactions := 0
	for i := range d.Accounts {
		acc := &d.Accounts[i]
		accountRes, err := tx.ExecContext(ctx,
			`INSERT INTO accounts (name, portfolio_id, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			acc.Account.Name, portfolioID, nowStr, nowStr,
		)
		if err != nil {
			return nil, fmt.Errorf("insert account %q: %w", acc.Account.Name, err)
		}
		accountID, err := accountRes.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("account insert id: %w", err)
		}
		for j := range acc.Transactions {
			txn := &acc.Transactions[j]
			var lotID any
			if txn.LotID != nil {
				lotID = *txn.LotID
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO transactions (account_id, date, type, symbol, quantity, price, currency, net_cash, lot_id, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				accountID, txn.Date.Format(time.RFC3339), txn.Type, txn.Symbol,
				txn.Quantity.String(), txn.Price.String(), txn.Currency, txn.NetCash.String(),
				lotID, nowStr, nowStr,
			); err != nil {
				return nil, fmt.Errorf("insert transaction %s %s: %w", txn.Type, txn.Symbol, err)
			}
			transactions++
		}
	}

	// Portfolio target allocations (the model's targets are its entries,
	// already stored above).
	for i := range d.PortfolioTargets {
		target := &d.PortfolioTargets[i]
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO target_allocations (portfolio_id, symbol, target_pct, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			portfolioID, target.Symbol, target.TargetPct.String(), nowStr, nowStr,
		); err != nil {
			return nil, fmt.Errorf("insert target allocation %s: %w", target.Symbol, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit seed: %w", err)
	}

	return &seed.Result{
		PortfolioID:    portfolioID,
		Accounts:       len(d.Accounts),
		Transactions:   transactions,
		Mappings:       mappings,
		MappingsReused: reused,
		ModelReused:    modelReused,
	}, nil
}
