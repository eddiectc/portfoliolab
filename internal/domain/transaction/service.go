package transaction

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Repository defines the data access interface for transactions.
type Repository interface {
	Create(ctx context.Context, t *Transaction) error
	GetByID(ctx context.Context, id int64) (*Transaction, error)
	List(ctx context.Context, filters ListFilters, limit, offset int) ([]Transaction, error)
	ListWithAccount(ctx context.Context, filters ListFilters, limit, offset int) ([]TransactionWithAccount, error)
	Update(ctx context.Context, t *Transaction) error
	Delete(ctx context.Context, id int64) error
}

// AccountChecker defines the interface for checking account existence.
type AccountChecker interface {
	AccountExists(ctx context.Context, id int64) bool
}

// SymbolChecker defines the interface for checking symbol existence.
type SymbolChecker interface {
	SymbolExists(ctx context.Context, symbol string) bool
}

// SymbolCreator defines the interface for creating symbols (used for $CASH auto-creation).
type SymbolCreator interface {
	CreateSymbol(ctx context.Context, internalSymbol, marketDataSymbol string) error
}

// ExternalReferenceChecker defines the interface for checking whether
// an external system reference already exists (used for duplicate detection).
type ExternalReferenceChecker interface {
	ExternalReferenceExists(ctx context.Context, externalSystem, externalReference string) bool
}

// LotChecker defines the interface for checking lot existence and metadata
// during transaction create/update validation.
type LotChecker interface {
	GetLotInfo(ctx context.Context, lotID string) (*LotInfo, error)
}

const (
	// defaultLimit is the default pagination limit when not specified.
	defaultLimit = 50
)

// Service errors.
var (
	// ErrNotFound indicates the requested transaction does not exist.
	ErrNotFound = fmt.Errorf("transaction not found")

	// ErrAccountNotFound indicates the referenced account does not exist.
	ErrAccountNotFound = fmt.Errorf("account not found")

	// ErrSymbolNotFound indicates the referenced symbol does not exist in the symbol map.
	ErrSymbolNotFound = fmt.Errorf("symbol not found")

	// ErrInvalidSymbol indicates the symbol is empty or invalid.
	ErrInvalidSymbol = fmt.Errorf("invalid symbol")

	// ErrInvalidPrice indicates the price is zero or negative.
	ErrInvalidPrice = fmt.Errorf("invalid price")

	// ErrInvalidCurrency indicates the currency code is not a valid ISO 4217 code
	// or does not match the cash symbol.
	ErrInvalidCurrency = fmt.Errorf("invalid currency")

	// ErrInvalidType indicates the transaction type is not one of the allowed types.
	ErrInvalidType = fmt.Errorf("invalid type")

	// ErrInvalidQuantity indicates the quantity is zero.
	ErrInvalidQuantity = fmt.Errorf("invalid quantity")

	// ErrInvalidDate indicates the date string does not parse as YYYY-MM-DD.
	ErrInvalidDate = fmt.Errorf("invalid date")

	// ErrInvalidNetCash indicates the net cash value is missing, null, or zero.
	ErrInvalidNetCash = fmt.Errorf("invalid net_cash")

	// ErrInvalidExternalField indicates an external field exceeds the 100-char limit.
	ErrInvalidExternalField = fmt.Errorf("invalid external field")

	// ErrLotNotFound indicates the referenced lot does not exist.
	ErrLotNotFound = fmt.Errorf("lot not found")

	// ErrLotSymbolMismatch indicates the lot belongs to a different symbol.
	ErrLotSymbolMismatch = fmt.Errorf("lot symbol mismatch")

	// ErrLotAccountMismatch indicates the lot belongs to a different account.
	ErrLotAccountMismatch = fmt.Errorf("lot account mismatch")

	// ErrLotTypeMismatch indicates the lot type is incompatible with the transaction type.
	ErrLotTypeMismatch = fmt.Errorf("lot type mismatch")

	// ErrInvalidLotID indicates the lot ID format is invalid.
	ErrInvalidLotID = fmt.Errorf("invalid lot ID")

)

// Service handles transaction business logic.
type Service struct {
	repo       Repository
	accounts   AccountChecker
	symbols    SymbolChecker
	symCreate  SymbolCreator
	lotChecker LotChecker
}

// NewService creates a new transaction service.
func NewService(repo Repository, accounts AccountChecker, symbols SymbolChecker, symCreate SymbolCreator, lotChecker LotChecker) *Service {
	return &Service{
		repo:       repo,
		accounts:   accounts,
		symbols:    symbols,
		symCreate:  symCreate,
		lotChecker: lotChecker,
	}
}

// Create creates a new transaction from a CreateRequest.
// Validates all fields, checks account and symbol existence,
// auto-creates $CASH symbols, and persists the transaction.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Transaction, error) {
	if err := ValidateCreateRequest(req); err != nil {
		return nil, mapValidationError(err)
	}

	if !s.accounts.AccountExists(ctx, req.AccountID) {
		return nil, ErrAccountNotFound
	}

	symbol := strings.TrimSpace(req.Symbol)

	// Check symbol existence; auto-create $CASH symbols if missing.
	if !s.symbols.SymbolExists(ctx, symbol) {
		if strings.HasPrefix(symbol, cashSymbolPrefix) {
			if err := s.symCreate.CreateSymbol(ctx, symbol, symbol); err != nil {
				return nil, fmt.Errorf("auto-create cash symbol: %w", err)
			}
		} else {
			return nil, ErrSymbolNotFound
		}
	}

	date, err := parseDate(req.Date)
	if err != nil {
		return nil, ErrInvalidDate
	}

	// Validate and resolve lot_id.
	lotID, err := s.resolveLotID(ctx, req.LotID, req.AccountID, symbol, req.Type)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	t := &Transaction{
		AccountID:         req.AccountID,
		Date:              date,
		Type:              req.Type,
		Symbol:            symbol,
		Quantity:          req.Quantity,
		Price:             req.Price,
		Currency:          req.Currency,
		NetCash:           req.NetCash,
		LotID:             lotID,
		ExternalSystem:    req.ExternalSystem,
		ExternalReference: req.ExternalReference,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := s.repo.Create(ctx, t); err != nil {
		return nil, fmt.Errorf("create transaction: %w", err)
	}

	return t, nil
}

// Get retrieves a transaction by ID.
func (s *Service) Get(ctx context.Context, id int64) (*Transaction, error) {
	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get transaction: %w", err)
	}
	return t, nil
}

// List retrieves transactions with filtering and pagination.
// A limit of 0 (or negative) defaults to defaultLimit (50).
// offset<0 defaults to 0.
func (s *Service) List(ctx context.Context, filters ListFilters, limit, offset int) ([]Transaction, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if offset < 0 {
		offset = 0
	}

	// Handle single-bound date filters by synthesizing the missing bound.
	f := filters
	if f.DateFrom != nil && f.DateTo == nil {
		to := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
		f.DateTo = &to
	}
	if f.DateTo != nil && f.DateFrom == nil {
		from := time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
		f.DateFrom = &from
	}

	items, err := s.repo.List(ctx, f, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list transactions: %w", err)
	}
	return items, nil
}

// ListWithAccount retrieves transactions with account names resolved via a
// SQL JOIN, with filtering and pagination.
// A limit of 0 (or negative) defaults to defaultLimit (50).
// offset<0 defaults to 0.
func (s *Service) ListWithAccount(ctx context.Context, filters ListFilters, limit, offset int) ([]TransactionWithAccount, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if offset < 0 {
		offset = 0
	}

	// Handle single-bound date filters by synthesizing the missing bound.
	f := filters
	if f.DateFrom != nil && f.DateTo == nil {
		to := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
		f.DateTo = &to
	}
	if f.DateTo != nil && f.DateFrom == nil {
		from := time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
		f.DateFrom = &from
	}

	items, err := s.repo.ListWithAccount(ctx, f, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list transactions with account: %w", err)
	}
	return items, nil
}

// Update updates an existing transaction.
// Only non-nil fields in the UpdateRequest are applied.
// Returns the updated transaction.
// No-op (empty request) returns current state without touching updated_at.
func (s *Service) Update(ctx context.Context, id int64, req UpdateRequest) (*Transaction, error) {
	if err := ValidateUpdateRequest(req); err != nil {
		return nil, mapValidationError(err)
	}

	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get transaction for update: %w", err)
	}

	changed := false

	if req.Date != nil {
		date, err := parseDate(*req.Date)
		if err != nil {
			return nil, ErrInvalidDate
		}
		t.Date = date
		changed = true
	}

	if req.Type != nil {
		t.Type = *req.Type
		changed = true
	}

	if req.Symbol != nil {
		symbol := strings.TrimSpace(*req.Symbol)
		// Check symbol existence; $CASH symbols are auto-created.
		if !s.symbols.SymbolExists(ctx, symbol) {
			if strings.HasPrefix(symbol, cashSymbolPrefix) {
				if err := s.symCreate.CreateSymbol(ctx, symbol, symbol); err != nil {
					return nil, fmt.Errorf("auto-create cash symbol: %w", err)
				}
			} else {
				return nil, ErrSymbolNotFound
			}
		}
		t.Symbol = symbol
		changed = true
	}

	if req.Quantity != nil {
		t.Quantity = *req.Quantity
		changed = true
	}

	if req.Price != nil {
		t.Price = *req.Price
		changed = true
	}

	if req.Currency != nil {
		t.Currency = *req.Currency
		changed = true
	}

	if req.NetCash.IsSet {
		t.NetCash = req.NetCash.Dec
		changed = true
	}

	if req.ExternalSystem != nil {
		t.ExternalSystem = req.ExternalSystem
		changed = true
	}

	if req.ExternalReference != nil {
		t.ExternalReference = req.ExternalReference
		changed = true
	}

	// lot_id is immutable once set — reject changes.
	if req.LotID != nil {
		// Empty string treated as "no change" (same as nil in form submissions).
		if *req.LotID != "" {
			if t.LotID != nil {
				// Already has a lot_id — reject any change.
				if *t.LotID != *req.LotID {
					return nil, ErrInvalidLotID
				}
			} else {
				// No lot_id yet — allow setting it if valid.
				if s.lotChecker != nil {
					lotInfo, err := s.lotChecker.GetLotInfo(ctx, *req.LotID)
					if err != nil {
						// Lot not found — this is OK for new lots.
						// The lot will be created during position recalculation.
					} else {
						// Lot exists — validate cross-references.
						if lotInfo.AccountID != t.AccountID {
							return nil, ErrLotAccountMismatch
						}
						if lotInfo.Symbol != t.Symbol {
							return nil, ErrLotSymbolMismatch
						}
						if lotInfo.LotType != t.Type {
							return nil, ErrLotTypeMismatch
						}
					}
				}
				t.LotID = req.LotID
				changed = true
			}
		}
	}

	if changed {
		t.UpdatedAt = time.Now()
		if err := s.repo.Update(ctx, t); err != nil {
			return nil, fmt.Errorf("update transaction: %w", err)
		}
	}

	return t, nil
}

// Delete removes a transaction by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete transaction: %w", err)
	}
	return nil
}

// parseDate parses a YYYY-MM-DD date string into a time.Time at midnight UTC.
func parseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

// resolveLotID validates a provided lot_id or auto-generates one.
// Only buy/sell transactions get lot_ids; other types return nil.
// If lot_id is nil or empty, a new one is auto-generated.
// If lot_id is provided, it is validated against existing lots via LotChecker.
func (s *Service) resolveLotID(ctx context.Context, lotID *string, accountID int64, symbol, txType string) (*string, error) {
	// Only buy/sell transactions get lots.
	if txType != "buy" && txType != "sell" {
		return nil, nil
	}

	// No lot_id provided — auto-generate.
	if lotID == nil || *lotID == "" {
		generated := generateLotID()
		return &generated, nil
	}

	// Validate provided lot_id.
	if len(*lotID) > 100 {
		return nil, ErrInvalidLotID
	}

	// Check that the lot exists and belongs to this account/symbol/type.
	if s.lotChecker != nil {
		lotInfo, err := s.lotChecker.GetLotInfo(ctx, *lotID)
		if err != nil {
			// Lot not found — this is OK for new lots (first transaction with this lot_id).
			// The lot will be created during position recalculation.
			// We only validate if the lot already exists.
			return lotID, nil
		}
		if lotInfo.AccountID != accountID {
			return nil, ErrLotAccountMismatch
		}
		if lotInfo.Symbol != symbol {
			return nil, ErrLotSymbolMismatch
		}
		if lotInfo.LotType != txType {
			return nil, ErrLotTypeMismatch
		}
	}

	return lotID, nil
}

// generateLotID creates a new unique lot ID in the format LOT-<first 8 chars of UUID>.
func generateLotID() string {
	return "LOT-" + strings.ReplaceAll(uuid.New().String(), "-", "")[:8]
}

// mapValidationError maps a validator error to the appropriate service error.
func mapValidationError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	// Check for known error patterns from the validator.
	case strings.Contains(msg, "invalid transaction type"):
		return ErrInvalidType
	case strings.Contains(msg, "quantity must be non-zero"):
		return ErrInvalidQuantity
	case strings.Contains(msg, "price must be greater than zero"):
		return ErrInvalidPrice
	case strings.Contains(msg, "invalid currency"):
		return ErrInvalidCurrency
	case strings.Contains(msg, "invalid date"):
		return ErrInvalidDate
	case strings.Contains(msg, "symbol is required"):
		return ErrInvalidSymbol
	case strings.Contains(msg, "net_cash"):
		return ErrInvalidNetCash
	case strings.Contains(msg, "external"):
		return ErrInvalidExternalField
	case strings.Contains(msg, "lot_id"):
		return ErrInvalidLotID
	default:
		return err
	}
}
