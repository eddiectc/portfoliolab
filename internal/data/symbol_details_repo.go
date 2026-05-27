package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/data/queries"
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
)

// SymbolDetailsRepository provides data access for cached symbol details,
// delegating to sqlc-generated queries.
type SymbolDetailsRepository struct {
	q  *queries.Queries
	db queries.DBTX
}

// NewSymbolDetailsRepository creates a new symbol details repository.
func NewSymbolDetailsRepository(db *sql.DB) *SymbolDetailsRepository {
	return &SymbolDetailsRepository{
		q:  queries.New(),
		db: db,
	}
}

// toSymbolDetail converts a sqlc SymbolDetail to a domain SymbolDetails.
func (r *SymbolDetailsRepository) toSymbolDetail(sd queries.SymbolDetail) (*symbol.SymbolDetails, error) {
	fetchedAt, err := parseTime(sd.FetchedAt)
	if err != nil {
		return nil, fmt.Errorf("parse fetched_at: %w", err)
	}

	details := &symbol.SymbolDetails{
		InternalSymbol: sd.InternalSymbol,
		ShortName:      nullString(sd.ShortName),
		LongName:       nullString(sd.LongName),
		Exchange:       nullString(sd.Exchange),
		Currency:       nullString(sd.Currency),
		QuoteType:      nullString(sd.QuoteType),
		Sector:         nullString(sd.Sector),
		FetchedAt:      fetchedAt,
	}

	// Deserialize JSON fields (may be null/empty)
	if sd.TopHoldings.Valid {
		if err := json.Unmarshal([]byte(sd.TopHoldings.String), &details.TopHoldings); err != nil {
			return nil, fmt.Errorf("parse top_holdings for %s: %w", sd.InternalSymbol, err)
		}
	}
	if sd.SectorWeightings.Valid {
		if err := json.Unmarshal([]byte(sd.SectorWeightings.String), &details.SectorWeightings); err != nil {
			return nil, fmt.Errorf("parse sector_weightings for %s: %w", sd.InternalSymbol, err)
		}
	}
	if sd.AggregatePositions.Valid {
		if err := json.Unmarshal([]byte(sd.AggregatePositions.String), &details.AggregatePositions); err != nil {
			return nil, fmt.Errorf("parse aggregate_positions for %s: %w", sd.InternalSymbol, err)
		}
	}
	if sd.FundProfile.Valid {
		if err := json.Unmarshal([]byte(sd.FundProfile.String), &details.FundProfile); err != nil {
			return nil, fmt.Errorf("parse fund_profile for %s: %w", sd.InternalSymbol, err)
		}
	}
	if sd.EquityValuation.Valid {
		if err := json.Unmarshal([]byte(sd.EquityValuation.String), &details.EquityValuation); err != nil {
			return nil, fmt.Errorf("parse equity_valuation for %s: %w", sd.InternalSymbol, err)
		}
	}
	if sd.GeographicAllocations.Valid {
		if err := json.Unmarshal([]byte(sd.GeographicAllocations.String), &details.GeographicAllocations); err != nil {
			return nil, fmt.Errorf("parse geographic_allocations for %s: %w", sd.InternalSymbol, err)
		}
	}
	if sd.MarketCapBreakdown.Valid {
		if err := json.Unmarshal([]byte(sd.MarketCapBreakdown.String), &details.MarketCapBreakdown); err != nil {
			return nil, fmt.Errorf("parse market_cap_breakdown for %s: %w", sd.InternalSymbol, err)
		}
	}
	if sd.Themes.Valid {
		if err := json.Unmarshal([]byte(sd.Themes.String), &details.Themes); err != nil {
			return nil, fmt.Errorf("parse themes for %s: %w", sd.InternalSymbol, err)
		}
	}
	if sd.ExtractorAsOfDate.Valid {
		asOf, err := parseTime(sd.ExtractorAsOfDate.String)
		if err != nil {
			return nil, fmt.Errorf("parse extractor_as_of_date for %s: %w", sd.InternalSymbol, err)
		}
		details.ExtractorAsOfDate = asOf
	}

	return details, nil
}

// nullString extracts the string value from sql.NullString, returning empty
// string if not valid.
func nullString(ns sql.NullString) string {
	if !ns.Valid {
		return ""
	}
	return ns.String
}

// toSQLNullString converts a possibly-empty string to sql.NullString.
// Empty strings are stored as NULL.
func toSQLNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// toSQLNullTime converts a time.Time to sql.NullString.
// Zero time returns empty (invalid) NullString.
func toSQLNullTime(t time.Time) sql.NullString {
	if t.IsZero() {
		return sql.NullString{}
	}
	return sql.NullString{String: t.Format(time.RFC3339), Valid: true}
}

// toSQLNullJSON marshals a value to JSON and returns sql.NullString.
// Returns empty (invalid) NullString if input is nil/empty.
func toSQLNullJSON(v interface{}) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return sql.NullString{}
	}
	if len(b) == 0 || string(b) == "null" {
		return sql.NullString{}
	}
	return sql.NullString{String: string(b), Valid: true}
}

// Upsert inserts or updates symbol details for a symbol.
func (r *SymbolDetailsRepository) Upsert(ctx context.Context, details *symbol.SymbolDetails) error {
	now := time.Now()
	_, err := r.q.InsertSymbolDetails(ctx, r.db, queries.InsertSymbolDetailsParams{
		InternalSymbol:        details.InternalSymbol,
		ShortName:             toSQLNullString(details.ShortName),
		LongName:              toSQLNullString(details.LongName),
		Exchange:              toSQLNullString(details.Exchange),
		Currency:              toSQLNullString(details.Currency),
		QuoteType:             toSQLNullString(details.QuoteType),
		Sector:                toSQLNullString(details.Sector),
		TopHoldings:           toSQLNullJSON(details.TopHoldings),
		SectorWeightings:      toSQLNullJSON(details.SectorWeightings),
		AggregatePositions:    toSQLNullJSON(details.AggregatePositions),
		FundProfile:           toSQLNullJSON(details.FundProfile),
		EquityValuation:       toSQLNullJSON(details.EquityValuation),
		GeographicAllocations: toSQLNullJSON(details.GeographicAllocations),
		MarketCapBreakdown:    toSQLNullJSON(details.MarketCapBreakdown),
		Themes:                toSQLNullJSON(details.Themes),
		ExtractorAsOfDate:     toSQLNullTime(details.ExtractorAsOfDate),
		FetchedAt:             details.FetchedAt.Format(time.RFC3339),
		UpdatedAt:             now.Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("upsert symbol details for %s: %w", details.InternalSymbol, err)
	}
	return nil
}

// GetByInternalSymbol retrieves cached symbol details by internal symbol.
// Returns ErrNotFound if no details exist for the symbol.
func (r *SymbolDetailsRepository) GetByInternalSymbol(ctx context.Context, internalSymbol string) (*symbol.SymbolDetails, error) {
	sd, err := r.q.GetSymbolDetailsByInternalSymbol(ctx, r.db, internalSymbol)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get symbol details for %s: %w", internalSymbol, err)
	}
	return r.toSymbolDetail(sd)
}

// ListStale retrieves symbols whose details are older than the given threshold.
// Returns internal_symbol and market_data_symbol pairs for refresh.
func (r *SymbolDetailsRepository) ListStale(ctx context.Context, olderThan time.Time) ([]symbol.StaleSymbol, error) {
	rows, err := r.q.ListStaleSymbolDetails(ctx, r.db, olderThan.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("list stale symbol details: %w", err)
	}

	stale := make([]symbol.StaleSymbol, len(rows))
	for i, row := range rows {
		var fetchedAt time.Time
		if row.FetchedAt.Valid {
			var err error
			fetchedAt, err = parseTime(row.FetchedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse fetched_at for %s: %w", row.InternalSymbol, err)
			}
		} else {
			// No details row exists — zero time indicates missing (not stale)
			fetchedAt = time.Time{}
		}
		stale[i] = symbol.StaleSymbol{
			InternalSymbol:   row.InternalSymbol,
			MarketDataSymbol: row.MarketDataSymbol,
			DataSourceURL:    nullString(row.DataSourceUrl),
			FetchedAt:        fetchedAt,
		}
	}
	return stale, nil
}
