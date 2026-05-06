package transaction

import (
	"context"
	"fmt"
	"strings"
	"time"
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

)

// Service handles transaction business logic.
type Service struct {
	repo     Repository
	accounts AccountChecker
	symbols  SymbolChecker
	symCreate SymbolCreator
}

// NewService creates a new transaction service.
func NewService(repo Repository, accounts AccountChecker, symbols SymbolChecker, symCreate SymbolCreator) *Service {
	return &Service{
		repo:      repo,
		accounts:  accounts,
		symbols:   symbols,
		symCreate: symCreate,
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
	default:
		return err
	}
}
