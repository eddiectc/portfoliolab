package trading212import

import "github.com/arch-portfolio-lab/portfoliolab/internal/domain/brokerimport"

// Type aliases for shared import types (defined in brokerimport).
type (
	PreviewResponse    = brokerimport.PreviewResponse
	PreviewTransaction = brokerimport.PreviewTransaction
	SkippedTransaction = brokerimport.SkippedTransaction
	ErroredTransaction = brokerimport.ErroredTransaction
	ImportResult       = brokerimport.ImportResult
)

// Type aliases for domain interfaces (defined in brokerimport).
type (
	SymbolResolver     = brokerimport.SymbolResolver
	DuplicateChecker   = brokerimport.DuplicateChecker
	TransactionCreator = brokerimport.TransactionCreator
	AccountChecker     = brokerimport.AccountChecker
	SymbolCreator      = brokerimport.SymbolCreator
	BrokerSymbolAdder  = brokerimport.BrokerSymbolAdder
)
