package seed

import (
	"context"
	"errors"
	"log/slog"

	"github.com/eddiectc/portfoliolab/internal/domain/transaction"
)

// ErrPortfolioExists indicates that the deployment already has portfolios, so
// the sample data seed must not run. The service treats this as a normal
// skip, not a failure.
var ErrPortfolioExists = errors.New("portfolios already exist; sample data seed skipped")

// Store writes the whole sample dataset in a single database transaction
// (all-or-nothing). Callers must treat ErrPortfolioExists as a skip.
type Store interface {
	// Seed persists the dataset atomically: one transaction, any error
	// rolls everything back. It returns the resulting counts for logging.
	Seed(ctx context.Context, d *Dataset) (*Result, error)
}

// PositionRecalculator is the narrow position-service dependency: the exact
// method behind POST /api/positions/recalculate and the automatic
// post-transaction hook. *position.Service satisfies it structurally.
type PositionRecalculator interface {
	RecalculatePortfolio(ctx context.Context, portfolioID int64) error
}

// Result reports what a successful seed wrote, for the startup log line.
type Result struct {
	PortfolioID    int64
	Accounts       int
	Transactions   int
	Mappings       int
	MappingsReused int
	ModelReused    bool
}

// Service orchestrates the startup sample-data seed.
type Service struct {
	store  Store
	recalc PositionRecalculator
	logger *slog.Logger
}

// NewService wires the seed service. logger may be nil (slog.Default is
// used instead).
func NewService(store Store, recalc PositionRecalculator, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, recalc: recalc, logger: logger}
}

// SeedDemo seeds the fixed sample dataset when no portfolio exists and then
// derives positions through the existing recalculation path.
//
// Guarantees:
//   - A deployment that already has portfolios is left untouched (Info log,
//     nil error).
//   - A position-recalculation failure after a successful data write is
//     logged at Error but returns nil: the data is intact and positions are
//     re-derived automatically on the next transaction mutation (or via the
//     manual recalculate endpoint) — the same failure posture as normal
//     transaction writes.
//   - Any other store error is returned to the caller, which logs and
//     continues (the server must start with an empty deployment).
func (s *Service) SeedDemo(ctx context.Context) error {
	d := NewDataset()

	// Assign lot IDs to every buy/sell transaction; deposits keep LotID nil.
	// NewDataset() builds fresh values, so mutating in place is safe.
	for i := range d.Accounts {
		for j := range d.Accounts[i].Transactions {
			tx := &d.Accounts[i].Transactions[j]
			if tx.Type == "buy" || tx.Type == "sell" {
				lotID := transaction.GenerateLotID()
				tx.LotID = &lotID
			}
		}
	}

	res, err := s.store.Seed(ctx, d)
	if err != nil {
		if errors.Is(err, ErrPortfolioExists) {
			s.logger.Info("skipping sample data seed: portfolios already exist")
			return nil
		}
		return err
	}

	if err := s.recalc.RecalculatePortfolio(ctx, res.PortfolioID); err != nil {
		s.logger.Error("sample data seeded but position recalculation failed; positions will be re-derived on the next transaction write",
			"portfolio_id", res.PortfolioID, "error", err)
		return nil
	}

	s.logger.Info("sample data seeded",
		"portfolio_id", res.PortfolioID,
		"accounts", res.Accounts,
		"transactions", res.Transactions,
		"mappings", res.Mappings,
		"mappings_reused", res.MappingsReused,
		"model_reused", res.ModelReused,
	)
	return nil
}
