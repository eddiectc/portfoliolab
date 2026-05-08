package handlers

import (
	"context"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
)

// SymbolService defines the symbol operations needed by the HTTP handler.
type SymbolService interface {
	Create(ctx context.Context, req symbolmapping.CreateRequest) (*symbolmapping.SymbolMapping, error)
}
