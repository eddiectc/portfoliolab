package data

import (
	"context"
	"fmt"
)

// SymbolMappingDataSourceURLAdapter wraps SymbolMappingRepository to provide
// the DataSourceURLSource interface used by the symbols service.
type SymbolMappingDataSourceURLAdapter struct {
	repo *SymbolMappingRepository
}

// NewSymbolMappingDataSourceURLAdapter creates an adapter that implements
// the DataSourceURLSource interface from the symbols package.
func NewSymbolMappingDataSourceURLAdapter(repo *SymbolMappingRepository) *SymbolMappingDataSourceURLAdapter {
	return &SymbolMappingDataSourceURLAdapter{repo: repo}
}

// GetDataSourceURLByInternalSymbol returns the data_source_url and ID for a
// symbol mapping. Returns empty string and 0 ID if no URL is configured.
func (a *SymbolMappingDataSourceURLAdapter) GetDataSourceURLByInternalSymbol(ctx context.Context, internalSymbol string) (string, int64, error) {
	sm, err := a.repo.GetByInternalSymbol(ctx, internalSymbol)
	if err != nil {
		return "", 0, fmt.Errorf("get symbol mapping for %s: %w", internalSymbol, err)
	}
	if sm == nil {
		return "", 0, fmt.Errorf("symbol mapping not found for %s", internalSymbol)
	}
	return sm.DataSourceURL, sm.ID, nil
}

// UpdateDataSourceURL sets the data_source_url for a symbol mapping by ID.
func (a *SymbolMappingDataSourceURLAdapter) UpdateDataSourceURL(ctx context.Context, id int64, url string) error {
	return a.repo.UpdateDataSourceURL(ctx, id, url)
}
