package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/arch-portfolio-lab/portfoliolab/internal/data/queries"
	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/symbolmapping"
)

// SymbolMappingRepository provides data access for symbol mappings,
// delegating to sqlc-generated queries.
type SymbolMappingRepository struct {
	q    *queries.Queries
	db   queries.DBTX
}

// NewSymbolMappingRepository creates a new symbol mapping repository.
func NewSymbolMappingRepository(db *sql.DB) *SymbolMappingRepository {
	return &SymbolMappingRepository{
		q:  queries.New(),
		db: db,
	}
}

// toSymbolMapping converts a sqlc SymbolMapping to a domain SymbolMapping.
// SQLite stores timestamps as text, so sqlc generates string fields;
// this function parses them back to time.Time.
func toSymbolMapping(sm queries.SymbolMapping) (*symbolmapping.SymbolMapping, error) {
	createdAt, err := parseTime(sm.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	updatedAt, err := parseTime(sm.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}

	return &symbolmapping.SymbolMapping{
		ID:               sm.ID,
		InternalSymbol:   sm.InternalSymbol,
		MarketDataSymbol: sm.MarketDataSymbol,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
	}, nil
}

// toBrokerSymbol converts a sqlc BrokerSymbolMapping to a domain BrokerSymbol.
func toBrokerSymbol(bsm queries.BrokerSymbolMapping) (*symbolmapping.BrokerSymbol, error) {
	createdAt, err := parseTime(bsm.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse broker symbol created_at: %w", err)
	}

	return &symbolmapping.BrokerSymbol{
		ID:           bsm.ID,
		SymbolID:     bsm.SymbolMappingID,
		BrokerName:   bsm.BrokerName,
		BrokerSymbol: bsm.BrokerSymbol,
		CreatedAt:    createdAt,
	}, nil
}

// Create inserts a new symbol mapping and returns it with the generated ID.
func (r *SymbolMappingRepository) Create(ctx context.Context, sm *symbolmapping.SymbolMapping) error {
	result, err := r.q.CreateSymbolMapping(ctx, r.db, queries.CreateSymbolMappingParams{
		InternalSymbol:   sm.InternalSymbol,
		MarketDataSymbol: sm.MarketDataSymbol,
		CreatedAt:        sm.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        sm.UpdatedAt.Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("insert symbol mapping: %w", err)
	}
	sm.ID = result.ID
	return nil
}

// GetByID retrieves a symbol mapping by its ID, including broker symbols.
func (r *SymbolMappingRepository) GetByID(ctx context.Context, id int64) (*symbolmapping.SymbolMapping, error) {
	sm, err := r.q.GetSymbolMapping(ctx, r.db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get symbol mapping by id %d: %w", id, err)
	}

	mapping, err := toSymbolMapping(sm)
	if err != nil {
		return nil, err
	}

	// Load associated broker symbols
	brokerSymbols, err := r.q.GetBrokerSymbolsByMappingID(ctx, r.db, id)
	if err != nil {
		return nil, fmt.Errorf("get broker symbols for mapping %d: %w", id, err)
	}

	mapping.BrokerSymbols = make([]symbolmapping.BrokerSymbol, len(brokerSymbols))
	for i, bsm := range brokerSymbols {
		bs, err := toBrokerSymbol(bsm)
		if err != nil {
			return nil, fmt.Errorf("parse broker symbol %d: %w", bsm.ID, err)
		}
		mapping.BrokerSymbols[i] = *bs
	}

	return mapping, nil
}

// GetByInternalSymbol retrieves a symbol mapping by its internal symbol.
func (r *SymbolMappingRepository) GetByInternalSymbol(ctx context.Context, internalSymbol string) (*symbolmapping.SymbolMapping, error) {
	sm, err := r.q.GetSymbolMappingByInternalSymbol(ctx, r.db, internalSymbol)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get symbol mapping by internal symbol %q: %w", internalSymbol, err)
	}
	return toSymbolMapping(sm)
}

// GetAll retrieves all symbol mappings with pagination.
func (r *SymbolMappingRepository) GetAll(ctx context.Context, limit, offset int) ([]symbolmapping.SymbolMapping, error) {
	sms, err := r.q.ListSymbolMappings(ctx, r.db, queries.ListSymbolMappingsParams{
		Limit:  int64(limit),
		Offset: int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list symbol mappings: %w", err)
	}

	mappings := make([]symbolmapping.SymbolMapping, len(sms))
	for i, sm := range sms {
		d, err := toSymbolMapping(sm)
		if err != nil {
			return nil, fmt.Errorf("parse symbol mapping %d: %w", sm.ID, err)
		}
		mappings[i] = *d
	}
	return mappings, nil
}

// Update modifies an existing symbol mapping.
func (r *SymbolMappingRepository) Update(ctx context.Context, sm *symbolmapping.SymbolMapping) error {
	_, err := r.q.UpdateSymbolMapping(ctx, r.db, queries.UpdateSymbolMappingParams{
		InternalSymbol:   sm.InternalSymbol,
		MarketDataSymbol: sm.MarketDataSymbol,
		UpdatedAt:        sm.UpdatedAt.Format(time.RFC3339),
		ID:               sm.ID,
	})
	if err != nil {
		return fmt.Errorf("update symbol mapping %d: %w", sm.ID, err)
	}
	return nil
}

// Delete removes a symbol mapping by ID (broker symbols are cascade-deleted).
func (r *SymbolMappingRepository) Delete(ctx context.Context, id int64) error {
	rows, err := r.q.DeleteSymbolMapping(ctx, r.db, id)
	if err != nil {
		return fmt.Errorf("delete symbol mapping %d: %w", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// AddBrokerSymbol adds a broker symbol to a symbol mapping.
func (r *SymbolMappingRepository) AddBrokerSymbol(ctx context.Context, symbolMappingID int64, brokerName, brokerSymbol string) error {
	_, err := r.q.AddBrokerSymbol(ctx, r.db, queries.AddBrokerSymbolParams{
		SymbolMappingID: symbolMappingID,
		BrokerName:      brokerName,
		BrokerSymbol:    brokerSymbol,
		CreatedAt:       time.Now().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("add broker symbol to mapping %d: %w", symbolMappingID, err)
	}
	return nil
}

// GetBrokerSymbolByBroker retrieves a broker symbol by broker name and broker symbol.
func (r *SymbolMappingRepository) GetBrokerSymbolByBroker(ctx context.Context, brokerName, brokerSymbol string) (*symbolmapping.BrokerSymbol, error) {
	bsm, err := r.q.GetBrokerSymbolByBroker(ctx, r.db, queries.GetBrokerSymbolByBrokerParams{
		BrokerName:   brokerName,
		BrokerSymbol: brokerSymbol,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get broker symbol %q/%q: %w", brokerName, brokerSymbol, err)
	}
	return toBrokerSymbol(bsm)
}

// HasReferencingTransactions checks if any transactions reference this symbol mapping.
// Stub: returns false until the transaction feature (f004) is implemented.
// TODO(f004): Replace with actual check against transactions table.
func (r *SymbolMappingRepository) HasReferencingTransactions(ctx context.Context, id int64) (bool, error) {
	return false, nil
}
