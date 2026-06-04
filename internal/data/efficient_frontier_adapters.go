package data

import (
	"context"
	"fmt"

	"codeberg.org/eddiectc/portfoliolab/internal/data/queries"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/efficientfrontier"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketservice"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
)

// --- SymbolLister ---

// SymbolListerImpl returns the list of all known internal symbols.
type SymbolListerImpl struct {
	q  *queries.Queries
	db queries.DBTX
}

// NewSymbolLister creates a new symbol lister from the symbol mapping repository.
func NewSymbolLister(repo *SymbolMappingRepository) *SymbolListerImpl {
	return &SymbolListerImpl{q: repo.q, db: repo.db}
}

// ListAllSymbols returns all internal symbols. Implements efficientfrontier.SymbolLister.
func (r *SymbolListerImpl) ListAllSymbols(ctx context.Context) ([]string, error) {
	mappings, err := r.q.ListAllSymbolMappings(ctx, r.db)
	if err != nil {
		return nil, fmt.Errorf("list all symbols: %w", err)
	}
	symbols := make([]string, 0, len(mappings))
	for _, m := range mappings {
		symbols = append(symbols, m.InternalSymbol)
	}
	return symbols, nil
}

// --- PortfolioSymbolSource ---

// PortfolioSymbolSourceImpl returns the distinct symbols held in a real portfolio.
// It resolves accounts → positions → distinct symbols.
type PortfolioSymbolSourceImpl struct {
	accountLister *account.Service
	positionSvc   *position.Service
}

// NewPortfolioSymbolSource creates a new portfolio symbol source.
func NewPortfolioSymbolSource(
	accountLister *account.Service,
	positionSvc *position.Service,
) *PortfolioSymbolSourceImpl {
	return &PortfolioSymbolSourceImpl{
		accountLister: accountLister,
		positionSvc:   positionSvc,
	}
}

// GetSymbolsByPortfolio returns the distinct symbols held in a portfolio.
// Implements efficientfrontier.PortfolioSymbolSource.
func (s *PortfolioSymbolSourceImpl) GetSymbolsByPortfolio(ctx context.Context, portfolioID int64) ([]string, error) {
	accounts, err := s.accountLister.ListByPortfolio(ctx, portfolioID, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("list accounts for portfolio %d: %w", portfolioID, err)
	}
	if len(accounts) == 0 {
		return []string{}, nil
	}

	accountIDs := make([]int64, 0, len(accounts))
	for _, a := range accounts {
		accountIDs = append(accountIDs, a.ID)
	}

	positions, err := s.positionSvc.GetOpenPositions(ctx, accountIDs, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("get positions for portfolio %d: %w", portfolioID, err)
	}

	// Collect distinct symbols.
	seen := make(map[string]struct{})
	var symbols []string
	for _, p := range positions {
		if _, ok := seen[p.Symbol]; !ok {
			seen[p.Symbol] = struct{}{}
			symbols = append(symbols, p.Symbol)
		}
	}
	if symbols == nil {
		symbols = []string{}
	}
	return symbols, nil
}

// --- ModelPortfolioSource ---

// ModelPortfolioSourceImpl wraps the model portfolio service to return
// the lightweight ModelPortfolioRef expected by the efficient frontier service.
type ModelPortfolioSourceImpl struct {
	svc *modelportfolio.Service
}

// NewModelPortfolioSource creates a new model portfolio source adapter.
func NewModelPortfolioSource(svc *modelportfolio.Service) *ModelPortfolioSourceImpl {
	return &ModelPortfolioSourceImpl{svc: svc}
}

// Get retrieves a model portfolio and returns a lightweight reference.
// Implements efficientfrontier.ModelPortfolioSource.
func (s *ModelPortfolioSourceImpl) Get(ctx context.Context, id int64) (efficientfrontier.ModelPortfolioRef, error) {
	mp, err := s.svc.Get(ctx, id)
	if err != nil {
		return efficientfrontier.ModelPortfolioRef{}, err
	}
	symbols := make([]string, 0, len(mp.Entries))
	for _, e := range mp.Entries {
		symbols = append(symbols, e.Symbol)
	}
	return efficientfrontier.ModelPortfolioRef{Symbols: symbols}, nil
}

// --- FxRateSource ---

// FxRateSourceImpl wraps the market service FX rate to return the
// lightweight FxRate expected by the efficient frontier service.
type FxRateSourceImpl struct {
	svc *marketservice.Service
}

// NewFxRateSource creates a new FX rate source adapter.
func NewFxRateSource(svc *marketservice.Service) *FxRateSourceImpl {
	return &FxRateSourceImpl{svc: svc}
}

// GetCurrentFxRate returns the current FX rate as a lightweight struct.
// Returns nil (not error) when no rate is available.
// Implements efficientfrontier.FxRateSource.
func (s *FxRateSourceImpl) GetCurrentFxRate(ctx context.Context, baseCurrency, quoteCurrency string) (*efficientfrontier.FxRate, error) {
	fx, err := s.svc.GetCurrentFxRate(ctx, baseCurrency, quoteCurrency)
	if err != nil {
		return nil, err
	}
	if fx == nil {
		return nil, nil
	}
	rate, ok := fx.Rate.Float64()
	if !ok {
		return nil, fmt.Errorf("convert FX rate %s/%s to float64", fx.BaseCurrency, fx.QuoteCurrency)
	}
	return &efficientfrontier.FxRate{
		BaseCurrency:  fx.BaseCurrency,
		QuoteCurrency: fx.QuoteCurrency,
		Rate:          rate,
	}, nil
}
