package position

import (
	"strconv"
	"strings"
	"time"

	"github.com/govalues/decimal"
)

// Position represents an open or closed position for a symbol in an account.
// CostBasis, AvgOpenPrice, AvgClosePrice are all derived from net_cash
// (which includes commission/fee), not quantity × price.
//
// For buys: net_cash is negative (cash outflow), so cost_basis and avg_open_price
// are negative reflecting money spent.
// For sells: net_cash is positive (cash inflow), so avg_close_price is positive
// reflecting money received.
type Position struct {
	ID              int64            `json:"id"`
	AccountID       int64            `json:"account_id"`
	AccountName     string           `json:"account_name"`
	Symbol          string           `json:"symbol"`
	Currency        string           `json:"currency"`
	Quantity        decimal.Decimal  `json:"quantity"`
	CostBasis       decimal.Decimal  `json:"cost_basis"`
	AvgOpenPrice    decimal.Decimal  `json:"avg_open_price"`
	AvgClosePrice   *decimal.Decimal `json:"avg_close_price,omitempty"`
	RealizedPnL     decimal.Decimal  `json:"realized_pnl"`
	RealizedPnlPct  *decimal.Decimal `json:"realized_pnl_pct,omitempty"`  // P&L as % of cost basis
	RealizedPnlBase *decimal.Decimal `json:"realized_pnl_base,omitempty"`
	FxRateUsed      *decimal.Decimal `json:"fx_rate_used,omitempty"`
	FxRateFallback  bool             `json:"fx_rate_fallback"`
	OpenDate        time.Time        `json:"open_date"`
	CloseDate       *time.Time       `json:"close_date,omitempty"`
	IsClosed        bool             `json:"is_closed"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

// PositionWithMarket extends Position with live market data for open positions.
type PositionWithMarket struct {
	Position
	MarketPrice         *decimal.Decimal `json:"market_price,omitempty"`
	MarketValue         decimal.Decimal  `json:"market_value"`
	UnrealizedPnL       decimal.Decimal  `json:"unrealized_pnl"`
	UnrealizedPnlPct    *decimal.Decimal `json:"unrealized_pnl_pct,omitempty"`
	CostBasisBase       *decimal.Decimal `json:"cost_basis_base,omitempty"`
	MarketValueBase     *decimal.Decimal `json:"market_value_base,omitempty"`
	UnrealizedPnLBase   *decimal.Decimal `json:"unrealized_pnl_base,omitempty"`
	BaseCurrency        string           `json:"base_currency,omitempty"`
	MarketDataAvailable   bool           `json:"market_data_available"`
}

// Lot represents a buy or sell lot, grouping one or more transactions
// that share the same lot_id.
type Lot struct {
	ID          int64            `json:"id"`
	LotID       string           `json:"lot_id"`
	AccountID   int64            `json:"account_id"`
	Symbol      string           `json:"symbol"`
	LotType     string           `json:"lot_type"` // "buy" or "sell"
	Quantity    decimal.Decimal  `json:"quantity"`
	CostBasis   decimal.Decimal  `json:"cost_basis"`
	SellPrice   *decimal.Decimal `json:"sell_price,omitempty"`
	RealizedPnL decimal.Decimal  `json:"realized_pnl"`
	OpenDate    time.Time        `json:"open_date"`
	CloseDate   *time.Time       `json:"close_date,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// LotConsumption records how much of a buy lot was consumed by a sell lot,
// and the realized P&L for that portion.
type LotConsumption struct {
	ID                int64            `json:"id"`
	SellLotID         string           `json:"sell_lot_id"`
	BuyLotID          string           `json:"buy_lot_id"`
	QuantityConsumed  decimal.Decimal  `json:"quantity_consumed"`
	CostBasisConsumed decimal.Decimal  `json:"cost_basis_consumed"`
	RealizedPnL       decimal.Decimal  `json:"realized_pnl"`
	CreatedAt         time.Time        `json:"created_at"`
}

// LotWithDetails is a Lot with its consumptions and source transactions,
// returned by drill-down queries.
type LotWithDetails struct {
	Lot
	Consumptions []LotConsumption `json:"consumptions"`
	Transactions []TransactionRef `json:"transactions"`
}

// TransactionRef is a lightweight reference to a source transaction,
// returned alongside lot details for drill-down.
type TransactionRef struct {
	ID       int64            `json:"id"`
	Date     time.Time        `json:"date"`
	Type     string           `json:"type"`
	Symbol   string           `json:"symbol"`
	Quantity decimal.Decimal  `json:"quantity"`
	Price    decimal.Decimal  `json:"price"`
	Currency string           `json:"currency"`
	NetCash  decimal.Decimal  `json:"net_cash"`
}

// ListFilters holds optional filter criteria for listing positions.
// Nil fields are treated as "no filter" for that dimension.
type ListFilters struct {
	AccountID *int64
	PortfolioID *int64
	AccountIDs *[]int64
}

// QueryParams serializes non-nil filter fields into a URL query fragment
// like "&account_id=1&portfolio_id=2". Returns "" if all fields are nil.
// Implements web.FilterEncoder for type-safe query preservation in templates.
func (f *ListFilters) QueryParams() string {
	var parts []string
	if f.AccountID != nil {
		parts = append(parts, "account_id="+strconv.FormatInt(*f.AccountID, 10))
	}
	if f.PortfolioID != nil {
		parts = append(parts, "portfolio_id="+strconv.FormatInt(*f.PortfolioID, 10))
	}
	if f.AccountIDs != nil && len(*f.AccountIDs) > 0 {
		for _, id := range *f.AccountIDs {
			parts = append(parts, "account_ids="+strconv.FormatInt(id, 10))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "&" + strings.Join(parts, "&")
}

// PaginationQuery returns the page/limit query params for pagination links.
func (f *ListFilters) PaginationQuery() string {
	return f.QueryParams()
}

// --- Errors ---

// ErrPositionNotFound indicates the requested position does not exist.
var ErrPositionNotFound = &PositionError{Code: "position_not_found", Message: "position not found"}

// ErrLotNotFound indicates the requested lot does not exist.
var ErrLotNotFound = &PositionError{Code: "lot_not_found", Message: "lot not found"}

// ErrLotSymbolMismatch indicates the lot belongs to a different symbol.
var ErrLotSymbolMismatch = &PositionError{Code: "lot_symbol_mismatch", Message: "lot belongs to a different symbol"}

// ErrLotAccountMismatch indicates the lot belongs to a different account.
var ErrLotAccountMismatch = &PositionError{Code: "lot_account_mismatch", Message: "lot belongs to a different account"}

// ErrLotTypeMismatch indicates the lot type is incompatible with the operation.
var ErrLotTypeMismatch = &PositionError{Code: "lot_type_mismatch", Message: "lot type mismatch"}

// ErrInvalidLotID indicates the lot ID format is invalid.
var ErrInvalidLotID = &PositionError{Code: "invalid_lot_id", Message: "invalid lot ID"}

// PositionError is a typed error for position domain operations.
type PositionError struct {
	Code    string
	Message string
}

func (e *PositionError) Error() string {
	return e.Message
}

// --- Calculator interfaces (stubs — logic implemented in Tasks 4a-4e) ---

// LotGroup groups transactions that share the same lot_id into a single
// buy or sell lot with aggregated quantities and prices.
type LotGroup struct {
	LotID       string
	AccountID   int64
	Symbol      string
	LotType     string // "buy" or "sell"
	Transactions []TransactionRef
	OpenDate    time.Time
	Quantity    decimal.Decimal
	CostBasis   decimal.Decimal // sum of net_cash for buys (negative)
	SellProceeds decimal.Decimal // sum of net_cash for sells (positive)
}

// MatchResult records how a sell lot was matched against one or more buy lots,
// including the consumptions created and total realized P&L.
type MatchResult struct {
	SellLot       LotGroup
	Consumptions  []LotConsumption
	RealizedPnL   decimal.Decimal
}

// CalculateResult holds the complete output of the position calculator:
// open positions, closed positions, lots, and lot consumptions.
type CalculateResult struct {
	OpenPositions    []Position
	ClosedPositions  []Position
	Lots             []Lot
	Consumptions     []LotConsumption
	CashPositions    []Position
}
