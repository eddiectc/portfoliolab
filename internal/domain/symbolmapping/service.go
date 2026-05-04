package symbolmapping

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/arch-portfolio-lab/portfoliolab/internal/market"
)

const (
	maxSymbolLength = 20
	defaultLimit    = 50
)

// Repository defines the data access interface for symbol mappings.
type Repository interface {
	Create(ctx context.Context, sm *SymbolMapping) error
	GetByID(ctx context.Context, id int64) (*SymbolMapping, error)
	GetByInternalSymbol(ctx context.Context, internalSymbol string) (*SymbolMapping, error)
	GetAll(ctx context.Context, limit, offset int) ([]SymbolMapping, error)
	Update(ctx context.Context, sm *SymbolMapping) error
	Delete(ctx context.Context, id int64) error
	AddBrokerSymbol(ctx context.Context, symbolMappingID int64, brokerName, brokerSymbol string) error
	GetBrokerSymbolByBroker(ctx context.Context, brokerName, brokerSymbol string) (*BrokerSymbol, error)
	HasReferencingTransactions(ctx context.Context, id int64) (bool, error)
}

// ErrNotFound indicates the requested symbol mapping does not exist.
var ErrNotFound = fmt.Errorf("symbol mapping not found")

// ErrInternalSymbolExists indicates a mapping with the same internal symbol already exists.
var ErrInternalSymbolExists = fmt.Errorf("internal symbol already exists")

// ErrBrokerSymbolExists indicates the broker symbol is already mapped to a different internal symbol.
var ErrBrokerSymbolExists = fmt.Errorf("broker symbol already exists")

// ErrInvalidSymbol indicates the symbol is empty or exceeds the maximum length.
var ErrInvalidSymbol = fmt.Errorf("invalid symbol")

// ErrInUse indicates the symbol mapping is referenced by transactions and cannot be deleted.
var ErrInUse = fmt.Errorf("symbol mapping is in use")

// ErrPreviewFailed indicates the market data preview could not be fetched.
var ErrPreviewFailed = fmt.Errorf("could not fetch market data preview")

// Service handles symbol mapping business logic.
type Service struct {
	repo    Repository
	fetcher market.QuoteFetcher
}

// ServiceOption configures the Service.
type ServiceOption func(*Service)

// WithQuoteFetcher sets an optional quote fetcher for market data preview.
func WithQuoteFetcher(fetcher market.QuoteFetcher) ServiceOption {
	return func(s *Service) {
		s.fetcher = fetcher
	}
}

// NewService creates a new symbol mapping service.
func NewService(repo Repository, opts ...ServiceOption) *Service {
	s := &Service{repo: repo}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Create creates a new symbol mapping with optional broker symbols.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*SymbolMapping, error) {
	internalSymbol := strings.TrimSpace(req.InternalSymbol)
	marketDataSymbol := strings.TrimSpace(req.MarketDataSymbol)

	if err := validateSymbol(internalSymbol, "internal_symbol"); err != nil {
		return nil, ErrInvalidSymbol
	}
	if err := validateSymbol(marketDataSymbol, "market_data_symbol"); err != nil {
		return nil, ErrInvalidSymbol
	}

	// Check for duplicate internal symbol
	if _, err := s.repo.GetByInternalSymbol(ctx, internalSymbol); err == nil {
		return nil, ErrInternalSymbolExists
	}

	now := time.Now()
	sm := &SymbolMapping{
		InternalSymbol:   internalSymbol,
		MarketDataSymbol: marketDataSymbol,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := s.repo.Create(ctx, sm); err != nil {
		return nil, fmt.Errorf("create symbol mapping: %w", err)
	}

	// Add broker symbols
	for _, bs := range req.BrokerSymbols {
		brokerName := strings.TrimSpace(bs.BrokerName)
		brokerSymbol := strings.TrimSpace(bs.BrokerSymbol)
		if brokerName == "" || brokerSymbol == "" {
			continue // skip empty broker symbol entries
		}
		if err := s.addBrokerSymbol(ctx, sm.ID, brokerName, brokerSymbol, internalSymbol); err != nil {
			return nil, err
		}
	}

	// Reload with broker symbols
	return s.repo.GetByID(ctx, sm.ID)
}

// Get retrieves a symbol mapping by ID.
func (s *Service) Get(ctx context.Context, id int64) (*SymbolMapping, error) {
	sm, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get symbol mapping: %w", err)
	}
	return sm, nil
}

// List retrieves all symbol mappings with pagination.
// A limit of 0 (or negative) defaults to defaultLimit (50).
func (s *Service) List(ctx context.Context, limit, offset int) ([]SymbolMapping, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if offset < 0 {
		offset = 0
	}

	mappings, err := s.repo.GetAll(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list symbol mappings: %w", err)
	}
	return mappings, nil
}

// Update updates an existing symbol mapping.
func (s *Service) Update(ctx context.Context, id int64, req UpdateRequest) (*SymbolMapping, error) {
	sm, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get symbol mapping for update: %w", err)
	}

	changed := false

	if req.InternalSymbol != nil {
		newSymbol := strings.TrimSpace(*req.InternalSymbol)
		if err := validateSymbol(newSymbol, "internal_symbol"); err != nil {
			return nil, ErrInvalidSymbol
		}
		// Check for duplicate internal symbol (excluding current mapping)
		existing, err := s.repo.GetByInternalSymbol(ctx, newSymbol)
		if err == nil && existing.ID != id {
			return nil, ErrInternalSymbolExists
		}
		sm.InternalSymbol = newSymbol
		changed = true
	}

	if req.MarketDataSymbol != nil {
		newSymbol := strings.TrimSpace(*req.MarketDataSymbol)
		if err := validateSymbol(newSymbol, "market_data_symbol"); err != nil {
			return nil, ErrInvalidSymbol
		}
		sm.MarketDataSymbol = newSymbol
		changed = true
	}

	if changed {
		sm.UpdatedAt = time.Now()
	}

	if err := s.repo.Update(ctx, sm); err != nil {
		return nil, fmt.Errorf("update symbol mapping: %w", err)
	}

	return sm, nil
}

// Delete removes a symbol mapping by ID.
// Returns ErrInUse if the mapping is referenced by transactions.
func (s *Service) Delete(ctx context.Context, id int64) error {
	inUse, err := s.repo.HasReferencingTransactions(ctx, id)
	if err != nil {
		return fmt.Errorf("check symbol mapping usage: %w", err)
	}
	if inUse {
		return ErrInUse
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete symbol mapping: %w", err)
	}
	return nil
}

// AddBrokerSymbol adds a broker symbol to an existing symbol mapping.
func (s *Service) AddBrokerSymbol(ctx context.Context, id int64, req BrokerSymbolRequest) error {
	// Verify the mapping exists
	sm, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get symbol mapping: %w", err)
	}

	brokerName := strings.TrimSpace(req.BrokerName)
	brokerSymbol := strings.TrimSpace(req.BrokerSymbol)

	if brokerName == "" {
		return ErrInvalidSymbol
	}
	if brokerSymbol == "" {
		return ErrInvalidSymbol
	}

	return s.addBrokerSymbol(ctx, id, brokerName, brokerSymbol, sm.InternalSymbol)
}

// addBrokerSymbol adds a broker symbol after validating uniqueness.
func (s *Service) addBrokerSymbol(ctx context.Context, symbolMappingID int64, brokerName, brokerSymbol, currentInternalSymbol string) error {
	// Check if broker symbol is already mapped to a different internal symbol
	existing, err := s.repo.GetBrokerSymbolByBroker(ctx, brokerName, brokerSymbol)
	if err == nil {
		// Broker symbol exists — check if it maps to the same symbol mapping
		if existing.SymbolID != symbolMappingID {
			return ErrBrokerSymbolExists
		}
		// Already mapped to this same mapping — no-op
		return nil
	}
	// If not found, that's fine — proceed with adding

	if err := s.repo.AddBrokerSymbol(ctx, symbolMappingID, brokerName, brokerSymbol); err != nil {
		return fmt.Errorf("add broker symbol: %w", err)
	}
	return nil
}

// PreviewSymbol fetches a market data quote for the given symbol.
// Returns ErrPreviewFailed if no fetcher is configured or the fetch fails.
func (s *Service) PreviewSymbol(ctx context.Context, symbol string) (*market.Quote, error) {
	if s.fetcher == nil {
		return nil, ErrPreviewFailed
	}
	quote, err := s.fetcher.FetchQuote(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrPreviewFailed, err.Error())
	}
	return quote, nil
}

func validateSymbol(symbol, field string) error {
	if symbol == "" {
		return fmt.Errorf("%s is required", field)
	}
	if len(symbol) > maxSymbolLength {
		return fmt.Errorf("%s must be at most %d characters", field, maxSymbolLength)
	}
	return nil
}
