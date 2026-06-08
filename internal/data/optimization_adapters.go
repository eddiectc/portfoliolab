package data

import (
	"context"
	"fmt"

	"codeberg.org/eddiectc/portfoliolab/internal/data/queries"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/optimization"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketservice"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
)

// --- SymbolLister ---

// OptimizationSymbolLister returns the list of all known internal symbols.
type OptimizationSymbolLister struct {
	q  *queries.Queries
	db queries.DBTX
}

// NewOptimizationSymbolLister creates a new symbol lister from the symbol mapping repository.
func NewOptimizationSymbolLister(repo *SymbolMappingRepository) *OptimizationSymbolLister {
	return &OptimizationSymbolLister{q: repo.q, db: repo.db}
}

// ListAllSymbols returns all internal symbols. Implements optimization.SymbolLister.
func (r *OptimizationSymbolLister) ListAllSymbols(ctx context.Context) ([]string, error) {
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

// OptimizationPortfolioSymbolSource returns the distinct symbols held in a real portfolio.
// It resolves accounts → positions → distinct symbols.
type OptimizationPortfolioSymbolSource struct {
	accountLister *account.Service
	positionSvc   *position.Service
}

// NewOptimizationPortfolioSymbolSource creates a new portfolio symbol source.
func NewOptimizationPortfolioSymbolSource(
	accountLister *account.Service,
	positionSvc *position.Service,
) *OptimizationPortfolioSymbolSource {
	return &OptimizationPortfolioSymbolSource{
		accountLister: accountLister,
		positionSvc:   positionSvc,
	}
}

// GetSymbolsByPortfolio returns the distinct symbols held in a portfolio.
// Implements optimization.PortfolioSymbolSource.
func (s *OptimizationPortfolioSymbolSource) GetSymbolsByPortfolio(ctx context.Context, portfolioID int64) ([]string, error) {
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

// OptimizationModelPortfolioSource wraps the model portfolio service to return
// the lightweight ModelPortfolioRef expected by the optimization service.
type OptimizationModelPortfolioSource struct {
	svc *modelportfolio.Service
}

// NewOptimizationModelPortfolioSource creates a new model portfolio source adapter.
func NewOptimizationModelPortfolioSource(svc *modelportfolio.Service) *OptimizationModelPortfolioSource {
	return &OptimizationModelPortfolioSource{svc: svc}
}

// Get retrieves a model portfolio and returns a lightweight reference.
// Implements optimization.ModelPortfolioSource.
func (s *OptimizationModelPortfolioSource) Get(ctx context.Context, id int64) (optimization.ModelPortfolioRef, error) {
	mp, err := s.svc.Get(ctx, id)
	if err != nil {
		return optimization.ModelPortfolioRef{}, err
	}
	symbols := make([]string, 0, len(mp.Entries))
	for _, e := range mp.Entries {
		symbols = append(symbols, e.Symbol)
	}
	return optimization.ModelPortfolioRef{Symbols: symbols}, nil
}

// --- FxRateSource ---

// OptimizationFxRateSource wraps the market service FX rate to return the
// lightweight FxRate expected by the optimization service.
type OptimizationFxRateSource struct {
	svc *marketservice.Service
}

// NewOptimizationFxRateSource creates a new FX rate source adapter.
func NewOptimizationFxRateSource(svc *marketservice.Service) *OptimizationFxRateSource {
	return &OptimizationFxRateSource{svc: svc}
}

// GetCurrentFxRate returns the current FX rate as a lightweight struct.
// Returns nil (not error) when no rate is available.
// Implements optimization.FxRateSource.
func (s *OptimizationFxRateSource) GetCurrentFxRate(ctx context.Context, baseCurrency, quoteCurrency string) (*optimization.FxRate, error) {
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
	return &optimization.FxRate{
		BaseCurrency:  fx.BaseCurrency,
		QuoteCurrency: fx.QuoteCurrency,
		Rate:          rate,
	}, nil
}

// Compile-time checks that adapters satisfy the optimization interfaces.
var (
	_ optimization.SymbolLister          = (*OptimizationSymbolLister)(nil)
	_ optimization.PortfolioSymbolSource = (*OptimizationPortfolioSymbolSource)(nil)
	_ optimization.ModelPortfolioSource  = (*OptimizationModelPortfolioSource)(nil)
	_ optimization.FxRateSource          = (*OptimizationFxRateSource)(nil)
)
