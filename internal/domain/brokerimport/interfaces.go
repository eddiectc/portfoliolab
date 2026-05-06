package brokerimport

import (
	"context"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/transaction"
)

// SymbolResolver resolves broker symbols to internal symbols and
// checks symbol existence.
type SymbolResolver interface {
	// ResolveBrokerSymbol looks up a broker symbol for a given broker name
	// and returns the associated internal symbol. Returns empty string if not found.
	ResolveBrokerSymbol(ctx context.Context, brokerName, brokerSymbol string) string
	// SymbolExists checks if an internal symbol exists in the symbol map.
	SymbolExists(ctx context.Context, symbol string) bool
}

// DuplicateChecker checks whether an external reference already exists.
type DuplicateChecker interface {
	ExternalReferenceExists(ctx context.Context, externalSystem, externalReference string) bool
}

// TransactionCreator creates transactions in bulk within a single
// database transaction (all-or-nothing).
type TransactionCreator interface {
	BatchCreate(ctx context.Context, txns []*transaction.Transaction) error
}

// AccountChecker checks account existence.
type AccountChecker interface {
	AccountExists(ctx context.Context, id int64) bool
}

// SymbolCreator creates new symbol mappings.
type SymbolCreator interface {
	CreateSymbol(ctx context.Context, internalSymbol, marketDataSymbol string) error
}

// BrokerSymbolAdder adds broker symbol mappings to existing symbols.
type BrokerSymbolAdder interface {
	AddBrokerSymbolMapping(ctx context.Context, brokerName, brokerSymbol, internalSymbol string) error
}
