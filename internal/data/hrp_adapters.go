package data

import (
	"context"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/efficientfrontier"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/hierarchicalriskparity"
)

// --- HRP SymbolLister ---

// HrpSymbolListerImpl returns the list of all known internal symbols.
// Delegates to SymbolListerImpl (efficient frontier adapter).
type HrpSymbolListerImpl struct {
	source hierarchicalriskparity.SymbolLister
}

// NewHrpSymbolLister creates a new HRP symbol lister from an existing adapter.
func NewHrpSymbolLister(source *SymbolListerImpl) *HrpSymbolListerImpl {
	return &HrpSymbolListerImpl{source: source}
}

// ListAllSymbols returns all internal symbols.
// Implements hierarchicalriskparity.SymbolLister.
func (r *HrpSymbolListerImpl) ListAllSymbols(ctx context.Context) ([]string, error) {
	return r.source.ListAllSymbols(ctx)
}

// --- HRP PortfolioSymbolSource ---

// HrpPortfolioSymbolSourceImpl returns the distinct symbols held in a real portfolio.
// Delegates to PortfolioSymbolSourceImpl (efficient frontier adapter).
type HrpPortfolioSymbolSourceImpl struct {
	source hierarchicalriskparity.PortfolioSymbolSource
}

// NewHrpPortfolioSymbolSource creates a new HRP portfolio symbol source.
func NewHrpPortfolioSymbolSource(source *PortfolioSymbolSourceImpl) *HrpPortfolioSymbolSourceImpl {
	return &HrpPortfolioSymbolSourceImpl{source: source}
}

// GetSymbolsByPortfolio returns the distinct symbols held in a portfolio.
// Implements hierarchicalriskparity.PortfolioSymbolSource.
func (s *HrpPortfolioSymbolSourceImpl) GetSymbolsByPortfolio(ctx context.Context, portfolioID int64) ([]string, error) {
	return s.source.GetSymbolsByPortfolio(ctx, portfolioID)
}

// --- HRP ModelPortfolioSource ---

// HrpModelPortfolioSourceImpl wraps the efficient frontier model portfolio source
// to return the HRP-specific ModelPortfolioRef type.
type HrpModelPortfolioSourceImpl struct {
	source *ModelPortfolioSourceImpl
}

// NewHrpModelPortfolioSource creates a new HRP model portfolio source adapter.
func NewHrpModelPortfolioSource(source *ModelPortfolioSourceImpl) *HrpModelPortfolioSourceImpl {
	return &HrpModelPortfolioSourceImpl{source: source}
}

// Get retrieves a model portfolio and returns an HRP ModelPortfolioRef.
// Implements hierarchicalriskparity.ModelPortfolioSource.
func (s *HrpModelPortfolioSourceImpl) Get(ctx context.Context, id int64) (hierarchicalriskparity.ModelPortfolioRef, error) {
	ref, err := s.source.Get(ctx, id)
	if err != nil {
		return hierarchicalriskparity.ModelPortfolioRef{}, err
	}
	return hierarchicalriskparity.ModelPortfolioRef{Symbols: ref.Symbols}, nil
}

// --- HRP FxRateSource ---

// HrpFxRateSourceImpl wraps the efficient frontier FX rate source
// to return the HRP-specific FxRate type.
type HrpFxRateSourceImpl struct {
	source *FxRateSourceImpl
}

// NewHrpFxRateSource creates a new HRP FX rate source adapter.
func NewHrpFxRateSource(source *FxRateSourceImpl) *HrpFxRateSourceImpl {
	return &HrpFxRateSourceImpl{source: source}
}

// GetCurrentFxRate returns the current FX rate as an HRP FxRate.
// Returns nil (not error) when no rate is available.
// Implements hierarchicalriskparity.FxRateSource.
func (s *HrpFxRateSourceImpl) GetCurrentFxRate(ctx context.Context, baseCurrency, quoteCurrency string) (*hierarchicalriskparity.FxRate, error) {
	fx, err := s.source.GetCurrentFxRate(ctx, baseCurrency, quoteCurrency)
	if err != nil {
		return nil, err
	}
	if fx == nil {
		return nil, nil
	}
	return &hierarchicalriskparity.FxRate{
		BaseCurrency:  fx.BaseCurrency,
		QuoteCurrency: fx.QuoteCurrency,
		Rate:          fx.Rate,
	}, nil
}

// ensureAdapterTypes are compile-time checks that the HRP adapters
// satisfy the corresponding domain interfaces.
var (
	_ hierarchicalriskparity.SymbolLister         = (*HrpSymbolListerImpl)(nil)
	_ hierarchicalriskparity.PortfolioSymbolSource = (*HrpPortfolioSymbolSourceImpl)(nil)
	_ hierarchicalriskparity.ModelPortfolioSource  = (*HrpModelPortfolioSourceImpl)(nil)
	_ hierarchicalriskparity.FxRateSource          = (*HrpFxRateSourceImpl)(nil)
)

// unused import guard — efficientfrontier is referenced in the delegation
// type assertions above (indirectly through the source fields).
// This variable suppresses the "imported and not used" linter warning.
var _ efficientfrontier.ModelPortfolioRef
