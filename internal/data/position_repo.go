package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/data/queries"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"github.com/govalues/decimal"
)

// PositionRepository provides data access for positions, lots, and lot consumptions,
// delegating to sqlc-generated queries.
type PositionRepository struct {
	q     *queries.Queries
	db    queries.DBTX
	sqlDB *sql.DB
}

// NewPositionRepository creates a new position repository.
func NewPositionRepository(db *sql.DB) *PositionRepository {
	return &PositionRepository{
		q:     queries.New(),
		db:    db,
		sqlDB: db,
	}
}

// toPosition converts a sqlc Position to a domain Position.
func toPosition(p queries.Position) (*position.Position, error) {
	openDate, err := parseTime(p.OpenDate)
	if err != nil {
		return nil, fmt.Errorf("parse open_date: %w", err)
	}
	createdAt, err := parseTime(p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	updatedAt, err := parseTime(p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}

	quantity, err := decimal.Parse(p.Quantity)
	if err != nil {
		return nil, fmt.Errorf("parse quantity: %w", err)
	}
	costBasis, err := decimal.Parse(p.CostBasis)
	if err != nil {
		return nil, fmt.Errorf("parse cost_basis: %w", err)
	}

	var avgOpenPrice *decimal.Decimal
	if p.AvgOpenPrice.Valid && p.AvgOpenPrice.String != "" {
		v, err := decimal.Parse(p.AvgOpenPrice.String)
		if err != nil {
			return nil, fmt.Errorf("parse avg_open_price: %w", err)
		}
		avgOpenPrice = &v
	}

	var avgClosePrice *decimal.Decimal
	if p.AvgClosePrice.Valid && p.AvgClosePrice.String != "" {
		v, err := decimal.Parse(p.AvgClosePrice.String)
		if err != nil {
			return nil, fmt.Errorf("parse avg_close_price: %w", err)
		}
		avgClosePrice = &v
	}

	realizedPnL, err := decimal.Parse(p.RealizedPnl)
	if err != nil {
		return nil, fmt.Errorf("parse realized_pnl: %w", err)
	}

	var realizedPnlBase *decimal.Decimal
	if p.RealizedPnlBase.Valid && p.RealizedPnlBase.String != "" {
		v, err := decimal.Parse(p.RealizedPnlBase.String)
		if err != nil {
			return nil, fmt.Errorf("parse realized_pnl_base: %w", err)
		}
		realizedPnlBase = &v
	}

	var fxRateUsed *decimal.Decimal
	if p.FxRateUsed.Valid && p.FxRateUsed.String != "" {
		v, err := decimal.Parse(p.FxRateUsed.String)
		if err != nil {
			return nil, fmt.Errorf("parse fx_rate_used: %w", err)
		}
		fxRateUsed = &v
	}

	var closeDate *time.Time
	if p.CloseDate.Valid && p.CloseDate.String != "" {
		t, err := parseTime(p.CloseDate.String)
		if err != nil {
			return nil, fmt.Errorf("parse close_date: %w", err)
		}
		closeDate = &t
	}

	avgOpen := decimal.Zero
	if avgOpenPrice != nil {
		avgOpen = *avgOpenPrice
	}

	return &position.Position{
		ID:              p.ID,
		AccountID:       p.AccountID,
		Symbol:          p.Symbol,
		Currency:        p.Currency,
		Quantity:        quantity,
		CostBasis:       costBasis,
		AvgOpenPrice:    avgOpen,
		AvgClosePrice:   avgClosePrice,
		RealizedPnL:     realizedPnL,
		RealizedPnlBase: realizedPnlBase,
		FxRateUsed:      fxRateUsed,
		FxRateFallback:  p.FxRateFallback,
		OpenDate:        openDate,
		CloseDate:       closeDate,
		IsClosed:        p.IsClosed == 1,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}, nil
}

// toPositionSlice converts a slice of sqlc Positions to domain Positions.
func toPositionSlice(items []queries.Position) ([]position.Position, error) {
	result := make([]position.Position, len(items))
	for i, p := range items {
		d, err := toPosition(p)
		if err != nil {
			return nil, fmt.Errorf("parse position %d: %w", p.ID, err)
		}
		result[i] = *d
	}
	return result, nil
}

// toLot converts a sqlc Lot to a domain Lot.
func toLot(l queries.Lot) (*position.Lot, error) {
	openDate, err := parseTime(l.OpenDate)
	if err != nil {
		return nil, fmt.Errorf("parse open_date: %w", err)
	}
	createdAt, err := parseTime(l.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	updatedAt, err := parseTime(l.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}

	quantity, err := decimal.Parse(l.Quantity)
	if err != nil {
		return nil, fmt.Errorf("parse quantity: %w", err)
	}
	costBasis, err := decimal.Parse(l.CostBasis)
	if err != nil {
		return nil, fmt.Errorf("parse cost_basis: %w", err)
	}

	var sellPrice *decimal.Decimal
	if l.SellPrice.Valid && l.SellPrice.String != "" {
		v, err := decimal.Parse(l.SellPrice.String)
		if err != nil {
			return nil, fmt.Errorf("parse sell_price: %w", err)
		}
		sellPrice = &v
	}

	realizedPnL, err := decimal.Parse(l.RealizedPnl)
	if err != nil {
		return nil, fmt.Errorf("parse realized_pnl: %w", err)
	}

	var closeDate *time.Time
	if l.CloseDate.Valid && l.CloseDate.String != "" {
		t, err := parseTime(l.CloseDate.String)
		if err != nil {
			return nil, fmt.Errorf("parse close_date: %w", err)
		}
		closeDate = &t
	}

	return &position.Lot{
		ID:          l.ID,
		LotID:       l.LotID,
		AccountID:   l.AccountID,
		Symbol:      l.Symbol,
		LotType:     l.LotType,
		Quantity:    quantity,
		CostBasis:   costBasis,
		SellPrice:   sellPrice,
		RealizedPnL: realizedPnL,
		OpenDate:    openDate,
		CloseDate:   closeDate,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

// toLotConsumption converts a sqlc LotConsumption to a domain LotConsumption.
func toLotConsumption(c queries.LotConsumption) (*position.LotConsumption, error) {
	createdAt, err := parseTime(c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}

	qtyConsumed, err := decimal.Parse(c.QuantityConsumed)
	if err != nil {
		return nil, fmt.Errorf("parse quantity_consumed: %w", err)
	}
	costBasisConsumed, err := decimal.Parse(c.CostBasisConsumed)
	if err != nil {
		return nil, fmt.Errorf("parse cost_basis_consumed: %w", err)
	}
	realizedPnL, err := decimal.Parse(c.RealizedPnl)
	if err != nil {
		return nil, fmt.Errorf("parse realized_pnl: %w", err)
	}

	return &position.LotConsumption{
		ID:                c.ID,
		SellLotID:         c.SellLotID,
		BuyLotID:          c.BuyLotID,
		QuantityConsumed:  qtyConsumed,
		CostBasisConsumed: costBasisConsumed,
		RealizedPnL:       realizedPnL,
		CreatedAt:         createdAt,
	}, nil
}

// toLotConsumptionSlice converts a slice of sqlc LotConsumptions to domain LotConsumptions.
func toLotConsumptionSlice(items []queries.LotConsumption) ([]position.LotConsumption, error) {
	result := make([]position.LotConsumption, len(items))
	for i, c := range items {
		d, err := toLotConsumption(c)
		if err != nil {
			return nil, fmt.Errorf("parse lot_consumption %d: %w", c.ID, err)
		}
		result[i] = *d
	}
	return result, nil
}

// --- Position CRUD ---

// CreatePosition inserts a new position and returns it with the generated ID.
func (r *PositionRepository) CreatePosition(ctx context.Context, p *position.Position) error {
	result, err := r.q.CreatePosition(ctx, r.db, queries.CreatePositionParams{
		AccountID:       p.AccountID,
		Symbol:          p.Symbol,
		Currency:        p.Currency,
		Quantity:        p.Quantity.String(),
		CostBasis:       p.CostBasis.String(),
		AvgOpenPrice:    toNullStringPtr(&p.AvgOpenPrice),
		AvgClosePrice:   toNullStringPtr(p.AvgClosePrice),
		RealizedPnl:     p.RealizedPnL.String(),
		RealizedPnlBase: toNullStringPtr(p.RealizedPnlBase),
		FxRateUsed:      toNullStringPtr(p.FxRateUsed),
		FxRateFallback:  p.FxRateFallback,
		OpenDate:        p.OpenDate.Format(time.RFC3339),
		CloseDate:       toNullStringTime(p.CloseDate),
		IsClosed:        boolToInt(p.IsClosed),
		CreatedAt:       p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       p.UpdatedAt.Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("insert position: %w", err)
	}
	p.ID = result.ID
	return nil
}

// GetOpenPositions retrieves open positions for a single account with pagination.
func (r *PositionRepository) GetOpenPositions(ctx context.Context, accountID int64, limit, offset int) ([]position.Position, error) {
	items, err := r.q.GetOpenPositionsByAccount(ctx, r.db, queries.GetOpenPositionsByAccountParams{
		AccountID: accountID,
		Limit:     int64(limit),
		Offset:    int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("get open positions for account %d: %w", accountID, err)
	}
	return toPositionSlice(items)
}

// GetClosedPositions retrieves closed positions for a single account with pagination.
func (r *PositionRepository) GetClosedPositions(ctx context.Context, accountID int64, limit, offset int) ([]position.Position, error) {
	items, err := r.q.GetClosedPositionsByAccount(ctx, r.db, queries.GetClosedPositionsByAccountParams{
		AccountID: accountID,
		Limit:     int64(limit),
		Offset:    int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("get closed positions for account %d: %w", accountID, err)
	}
	return toPositionSlice(items)
}

// --- Lot CRUD ---

// CreateLot inserts a new lot and returns it with the generated ID.
func (r *PositionRepository) CreateLot(ctx context.Context, l *position.Lot) error {
	result, err := r.q.CreateLot(ctx, r.db, queries.CreateLotParams{
		LotID:       l.LotID,
		AccountID:   l.AccountID,
		Symbol:      l.Symbol,
		LotType:     l.LotType,
		Quantity:    l.Quantity.String(),
		CostBasis:   l.CostBasis.String(),
		SellPrice:   toNullStringPtr(l.SellPrice),
		RealizedPnl: l.RealizedPnL.String(),
		OpenDate:    l.OpenDate.Format(time.RFC3339),
		CloseDate:   toNullStringTime(l.CloseDate),
		CreatedAt:   l.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   l.UpdatedAt.Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("insert lot: %w", err)
	}
	l.ID = result.ID
	return nil
}

// GetLotByLotID retrieves a lot by its lot_id.
func (r *PositionRepository) GetLotByLotID(ctx context.Context, lotID string) (*position.Lot, error) {
	l, err := r.q.GetLotByLotID(ctx, r.db, lotID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, position.ErrLotNotFound
		}
		return nil, fmt.Errorf("get lot by lot_id %s: %w", lotID, err)
	}
	return toLot(l)
}

// --- Lot Consumption CRUD ---

// CreateConsumption inserts a new lot consumption and returns it with the generated ID.
func (r *PositionRepository) CreateConsumption(ctx context.Context, c *position.LotConsumption) error {
	result, err := r.q.CreateLotConsumption(ctx, r.db, queries.CreateLotConsumptionParams{
		SellLotID:         c.SellLotID,
		BuyLotID:          c.BuyLotID,
		QuantityConsumed:  c.QuantityConsumed.String(),
		CostBasisConsumed: c.CostBasisConsumed.String(),
		RealizedPnl:       c.RealizedPnL.String(),
		CreatedAt:         c.CreatedAt.Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("insert lot_consumption: %w", err)
	}
	c.ID = result.ID
	return nil
}

// GetConsumptionsBySellLot retrieves consumptions for a sell lot.
func (r *PositionRepository) GetConsumptionsBySellLot(ctx context.Context, sellLotID string) ([]position.LotConsumption, error) {
	items, err := r.q.GetConsumptionsBySellLot(ctx, r.db, sellLotID)
	if err != nil {
		return nil, fmt.Errorf("get consumptions for sell lot %s: %w", sellLotID, err)
	}
	return toLotConsumptionSlice(items)
}

// --- Bulk Operations ---

// DeleteAllForAccount removes all positions, lots, and lot consumptions for an account.
func (r *PositionRepository) DeleteAllForAccount(ctx context.Context, accountID int64) error {
	err := r.q.DeleteAllLotConsumptionsForAccount(ctx, r.db, queries.DeleteAllLotConsumptionsForAccountParams{
		AccountID:   accountID,
		AccountID_2: accountID,
	})
	if err != nil {
		return fmt.Errorf("delete lot consumptions for account %d: %w", accountID, err)
	}
	_, err = r.q.DeleteAllLotsForAccount(ctx, r.db, accountID)
	if err != nil {
		return fmt.Errorf("delete lots for account %d: %w", accountID, err)
	}
	_, err = r.q.DeleteAllPositionsForAccount(ctx, r.db, accountID)
	if err != nil {
		return fmt.Errorf("delete positions for account %d: %w", accountID, err)
	}
	return nil
}

// Recalculate deletes old position data and inserts new data within a single
// database transaction. Called by the position service after CalculatePositions.
func (r *PositionRepository) Recalculate(ctx context.Context, accountID int64, result *position.CalculateResult) error {
	tx, err := r.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete old data.
	if _, err := tx.ExecContext(ctx, "DELETE FROM lot_consumptions WHERE sell_lot_id IN (SELECT l.lot_id FROM lots l WHERE l.account_id = ?) OR buy_lot_id IN (SELECT l.lot_id FROM lots l WHERE l.account_id = ?)", accountID, accountID); err != nil {
		return fmt.Errorf("delete lot consumptions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM lots WHERE account_id = ?", accountID); err != nil {
		return fmt.Errorf("delete lots: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM positions WHERE account_id = ?", accountID); err != nil {
		return fmt.Errorf("delete positions: %w", err)
	}

	now := time.Now()

	// Insert lots.
	for i := range result.Lots {
		l := &result.Lots[i]
		l.CreatedAt = now
		l.UpdatedAt = now
		sellPrice := ""
		if l.SellPrice != nil {
			sellPrice = l.SellPrice.String()
		}
		closeDate := ""
		if l.CloseDate != nil {
			closeDate = l.CloseDate.Format(time.RFC3339)
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO lots (lot_id, account_id, symbol, lot_type, quantity, cost_basis, sell_price, realized_pnl, open_date, close_date, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			l.LotID, l.AccountID, l.Symbol, l.LotType, l.Quantity.String(),
			l.CostBasis.String(), sellPrice, l.RealizedPnL.String(),
			l.OpenDate.Format(time.RFC3339), closeDate,
			l.CreatedAt.Format(time.RFC3339), l.UpdatedAt.Format(time.RFC3339),
		)
		if err != nil {
			return fmt.Errorf("insert lot %s: %w", l.LotID, err)
		}
	}

	// Insert consumptions.
	for i := range result.Consumptions {
		c := &result.Consumptions[i]
		c.CreatedAt = now
		_, err := tx.ExecContext(ctx,
			`INSERT INTO lot_consumptions (sell_lot_id, buy_lot_id, quantity_consumed, cost_basis_consumed, realized_pnl, created_at)
				 VALUES (?, ?, ?, ?, ?, ?)`,
			c.SellLotID, c.BuyLotID, c.QuantityConsumed.String(),
			c.CostBasisConsumed.String(), c.RealizedPnL.String(),
			c.CreatedAt.Format(time.RFC3339),
		)
		if err != nil {
			return fmt.Errorf("insert consumption: %w", err)
		}
	}

	// Insert positions (open, closed, cash).
	allPositions := append(append(result.OpenPositions, result.ClosedPositions...), result.CashPositions...)
	for i := range allPositions {
		p := &allPositions[i]
		p.CreatedAt = now
		p.UpdatedAt = now
		avgOpenPrice := p.AvgOpenPrice.String()
		avgClosePrice := ""
		if p.AvgClosePrice != nil {
			avgClosePrice = p.AvgClosePrice.String()
		}
		realizedPnlBase := ""
		if p.RealizedPnlBase != nil {
			realizedPnlBase = p.RealizedPnlBase.String()
		}
		fxRateUsed := ""
		if p.FxRateUsed != nil {
			fxRateUsed = p.FxRateUsed.String()
		}
		fxRateFallback := int64(0)
		if p.FxRateFallback {
			fxRateFallback = 1
		}
		closeDate := ""
		if p.CloseDate != nil {
			closeDate = p.CloseDate.Format(time.RFC3339)
		}
		isClosed := int64(0)
		if p.IsClosed {
			isClosed = 1
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO positions (account_id, symbol, currency, quantity, cost_basis, avg_open_price, avg_close_price, realized_pnl, realized_pnl_base, fx_rate_used, fx_rate_fallback, open_date, close_date, is_closed, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			p.AccountID, p.Symbol, p.Currency, p.Quantity.String(),
			p.CostBasis.String(), avgOpenPrice, avgClosePrice,
			p.RealizedPnL.String(), realizedPnlBase, fxRateUsed, fxRateFallback,
			p.OpenDate.Format(time.RFC3339), closeDate, isClosed,
			p.CreatedAt.Format(time.RFC3339), p.UpdatedAt.Format(time.RFC3339),
		)
		if err != nil {
			return fmt.Errorf("insert position: %w", err)
		}
	}

	return tx.Commit()
}

// --- Helper functions ---

// toNullStringPtr converts a *decimal.Decimal to sql.NullString.
func toNullStringPtr(d *decimal.Decimal) sql.NullString {
	if d == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: d.String(), Valid: true}
}

// toNullStringTime converts a *time.Time to sql.NullString.
func toNullStringTime(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: t.Format(time.RFC3339), Valid: true}
}

// boolToInt converts a bool to int64 (true → 1, false → 0).
func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
