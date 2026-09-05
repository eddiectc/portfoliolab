package ibkrimport

import "github.com/eddiectc/portfoliolab/internal/domain/brokerimport"

// Type aliases for backward compatibility within the package.
// The actual type definitions live in internal/domain/brokerimport/types.go
type (
	PreviewResponse    = brokerimport.PreviewResponse
	PreviewTransaction = brokerimport.PreviewTransaction
	SkippedTransaction = brokerimport.SkippedTransaction
	ErroredTransaction = brokerimport.ErroredTransaction
	ImportResult       = brokerimport.ImportResult
)
