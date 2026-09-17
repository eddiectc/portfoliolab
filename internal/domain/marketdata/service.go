package marketdata

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrInvalidSymbol indicates the symbol is empty.
	ErrInvalidSymbol = errors.New("symbol is required")

	// ErrInvalidDataType indicates the data type is not one of stock, fx, nav.
	ErrInvalidDataType = errors.New("data_type must be one of stock, fx, nav")

	// ErrInvalidPrice indicates the price is zero or negative.
	ErrInvalidPrice = errors.New("price must be greater than zero")

	// ErrInvalidDate indicates the date does not parse as YYYY-MM-DD or is
	// later than today (UTC).
	ErrInvalidDate = errors.New("invalid date")
)

// dateLayout is the canonical on-wire and on-disk date format.
const dateLayout = "2006-01-02"

// Store is the narrow persistence dependency of Service. The repository maps
// it onto the market_data_overrides table.
type Store interface {
	// Upsert inserts or replaces the override for (Symbol, Date, DataType).
	Upsert(ctx context.Context, o *Override) error
	// Delete removes the override for the key and reports whether a row
	// was actually deleted.
	Delete(ctx context.Context, symbol, date string, dt DataType) (bool, error)
	// List returns all overrides, or only the symbol's when symbol is non-nil.
	List(ctx context.Context, symbol *string) ([]Override, error)
}

// Service validates override requests and orchestrates their storage.
type Service struct {
	store Store
}

// NewService wires the override service.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Set validates req and upserts the override, returning the stored value.
// Validation failures return the matching sentinel error and never touch
// the store.
func (s *Service) Set(ctx context.Context, req SetRequest) (*Override, error) {
	if req.Symbol == "" {
		return nil, ErrInvalidSymbol
	}
	if !req.DataType.Valid() {
		return nil, ErrInvalidDataType
	}
	if !req.Price.IsPos() {
		return nil, ErrInvalidPrice
	}
	if err := validateDate(req.Date); err != nil {
		return nil, err
	}

	o := &Override{
		Symbol:   req.Symbol,
		Date:     req.Date,
		DataType: req.DataType,
		Price:    req.Price,
		Currency: req.Currency,
		Source:   Source,
	}

	if err := s.store.Upsert(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

// Remove validates the data type, deletes the override for the key, and
// returns ErrNotFound when no override matched.
func (s *Service) Remove(ctx context.Context, symbol, date string, dt DataType) error {
	if !dt.Valid() {
		return ErrInvalidDataType
	}
	deleted, err := s.store.Delete(ctx, symbol, date, dt)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrNotFound
	}
	return nil
}

// List passes through to the store; nil symbol lists all overrides.
func (s *Service) List(ctx context.Context, symbol *string) ([]Override, error) {
	return s.store.List(ctx, symbol)
}

// validateDate requires a YYYY-MM-DD date on or before today (UTC).
// Overrides only correct history, so future dates are rejected.
func validateDate(s string) error {
	d, err := time.Parse(dateLayout, s)
	if err != nil {
		return ErrInvalidDate
	}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if d.After(today) {
		return ErrInvalidDate
	}
	return nil
}
