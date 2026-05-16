package market

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
)

// Yahoo Finance endpoint URLs. Package-level vars (not consts) so they can be
// overridden in tests with a mock server.
var (
	yahooCookieURL    = "https://fc.yahoo.com"
	yahooCrumbURL     = "https://query2.finance.yahoo.com/v1/test/getcrumb"
	yahooQuoteSummary = "https://query2.finance.yahoo.com/v10/finance/quoteSummary"
)

// SymbolDetailsFetcher fetches rich symbol metadata (holdings, sectors, fund
// profile) from a market data provider.
type SymbolDetailsFetcher interface {
	FetchSymbolDetails(ctx context.Context, marketDataSymbol string) (*symbol.SymbolDetails, error)
}

// quoteSummaryResponse is the top-level response from Yahoo's quoteSummary API.
type quoteSummaryResponse struct {
	QuoteSummary struct {
		Result []quoteSummaryResult `json:"result"`
		Error  *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"quoteSummary"`
}

// quoteSummaryResult contains one or more modules of data for a symbol.
type quoteSummaryResult struct {
	TopHoldings  *topHoldingsModule  `json:"topHoldings,omitempty"`
	FundProfile  *fundProfileModule  `json:"fundProfile,omitempty"`
	AssetProfile *assetProfileModule `json:"assetProfile,omitempty"`
}

// topHoldingsModule contains ETF holdings data.
type topHoldingsModule struct {
	Holdings          []topHoldingItem    `json:"holdings"`
	StockPosition     float64             `json:"stockPosition"`
	BondPosition      float64             `json:"bondPosition"`
	CashPosition      float64             `json:"cashPosition"`
	ConvertiblePos    float64             `json:"convertiblePosition"`
	PreferredPosition float64             `json:"preferredPosition"`
	OtherPosition     float64             `json:"otherPosition"`
	EquityHoldings    *equityHoldingsData `json:"equityHoldings"`
	SectorWeightings  []sectorWeightItem  `json:"sectorWeightings"`
	MaxAge            int                 `json:"maxAge"`
}

type topHoldingItem struct {
	Symbol         string  `json:"symbol"`
	HoldingName    string  `json:"holdingName"`
	HoldingPercent float64 `json:"holdingPercent"`
}

type equityHoldingsData struct {
	PriceToEarnings float64 `json:"priceToEarnings"`
	PriceToBook     float64 `json:"priceToBook"`
	PriceToCashflow float64 `json:"priceToCashflow"`
	PriceToSales    float64 `json:"priceToSales"`
}

type sectorWeightItem map[string]float64

// fundProfileModule contains fund-level metadata.
type fundProfileModule struct {
	Family     string `json:"family"`
	LegalType  string `json:"legalType"`
	FeesExpenses struct {
		TotalNetAssets           float64 `json:"totalNetAssets"`
		AnnualReportExpenseRatio float64 `json:"annualReportExpenseRatio"`
		AnnualHoldingsTurnover   float64 `json:"annualHoldingsTurnover"`
	} `json:"feesExpensesInvestment"`
}

// assetProfileModule contains generic symbol info.
type assetProfileModule struct {
	ShortName string `json:"shortName"`
	LongName  string `json:"longName"`
	Exchange  string `json:"exchange"`
	Currency  string `json:"currency"`
	QuoteType string `json:"quoteType"`
	MaxAge    int    `json:"maxAge"`
}

// FetchSymbolDetails fetches rich metadata for a symbol from Yahoo Finance.
// It performs the crumb/cookie auth flow, calls the quoteSummary endpoint with
// topHoldings, fundProfile, and assetProfile modules, and returns a populated
// SymbolDetails struct. Partial data is returned gracefully — if some modules
// are missing, the available fields are still populated.
func (f *YahooFinanceFetcher) FetchSymbolDetails(ctx context.Context, marketDataSymbol string) (*symbol.SymbolDetails, error) {
	client := f.httpClient()

	// Step 1: Get cookie
	cookie, err := f.getCookie(ctx, client)
	if err != nil {
		f.logger.Warn("failed to get Yahoo cookie", "error", err)
		return nil, fmt.Errorf("failed to get Yahoo cookie: %w", err)
	}

	// Step 2: Get crumb
	crumb, err := f.getCrumb(ctx, client, cookie)
	if err != nil {
		f.logger.Warn("failed to get Yahoo crumb", "error", err)
		return nil, fmt.Errorf("failed to get Yahoo crumb: %w", err)
	}

	// Step 3: Fetch quoteSummary
	modules := "topHoldings,fundProfile,assetProfile"
	url := fmt.Sprintf("%s/%s?modules=%s&corsDomain=finance.yahoo.com&formatted=false&crumb=%s",
		yahooQuoteSummary, marketDataSymbol, modules, crumb)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch quoteSummary for %s: %w", marketDataSymbol, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized for %s (crumb may be expired)", marketDataSymbol)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("symbol %s not found on Yahoo Finance", marketDataSymbol)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, marketDataSymbol)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body for %s: %w", marketDataSymbol, err)
	}

	var quoteResp quoteSummaryResponse
	if err := json.Unmarshal(body, &quoteResp); err != nil {
		return nil, fmt.Errorf("parse quoteSummary JSON for %s: %w", marketDataSymbol, err)
	}

	if quoteResp.QuoteSummary.Error != nil {
		return nil, fmt.Errorf("Yahoo Finance error for %s: %s — %s",
			marketDataSymbol, quoteResp.QuoteSummary.Error.Code,
			quoteResp.QuoteSummary.Error.Description)
	}

	if len(quoteResp.QuoteSummary.Result) == 0 {
		return nil, fmt.Errorf("no results for %s", marketDataSymbol)
	}

	result := quoteResp.QuoteSummary.Result[0]

	// Step 4: Build SymbolDetails from available modules
	details := &symbol.SymbolDetails{
		FetchedAt: time.Now(),
	}

	if result.AssetProfile != nil {
		details.ShortName = result.AssetProfile.ShortName
		details.LongName = result.AssetProfile.LongName
		details.Exchange = result.AssetProfile.Exchange
		details.Currency = result.AssetProfile.Currency
		details.QuoteType = result.AssetProfile.QuoteType
	}

	if result.TopHoldings != nil {
		details.TopHoldings = parseTopHoldings(result.TopHoldings.Holdings)
		details.SectorWeightings = parseSectorWeightings(result.TopHoldings.SectorWeightings)
		details.AggregatePositions = &symbol.AggregatePositions{
			Stock:       result.TopHoldings.StockPosition,
			Bond:        result.TopHoldings.BondPosition,
			Cash:        result.TopHoldings.CashPosition,
			Convertible: result.TopHoldings.ConvertiblePos,
			Preferred:   result.TopHoldings.PreferredPosition,
			Other:       result.TopHoldings.OtherPosition,
		}
		if result.TopHoldings.EquityHoldings != nil {
			details.EquityValuation = &symbol.EquityValuation{
				PriceToEarnings: result.TopHoldings.EquityHoldings.PriceToEarnings,
				PriceToBook:     result.TopHoldings.EquityHoldings.PriceToBook,
				PriceToCashflow: result.TopHoldings.EquityHoldings.PriceToCashflow,
				PriceToSales:    result.TopHoldings.EquityHoldings.PriceToSales,
			}
		}
	}

	if result.FundProfile != nil {
		details.FundProfile = &symbol.FundProfile{
			Family:                 result.FundProfile.Family,
			LegalType:              result.FundProfile.LegalType,
			TotalNetAssets:         result.FundProfile.FeesExpenses.TotalNetAssets,
			AnnualExpenseRatio:     result.FundProfile.FeesExpenses.AnnualReportExpenseRatio,
			AnnualHoldingsTurnover: result.FundProfile.FeesExpenses.AnnualHoldingsTurnover,
		}
	}

	return details, nil
}

// getCookie fetches the Yahoo session cookie used for authentication.
func (f *YahooFinanceFetcher) getCookie(ctx context.Context, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, yahooCookieURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()

	cookies := resp.Header.Values("Set-Cookie")
	for _, c := range cookies {
		// Extract just the cookie name=value part (before the first ';')
		parts := strings.SplitN(c, ";", 2)
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0]), nil
		}
	}

	return "", fmt.Errorf("no cookie returned from %s", yahooCookieURL)
}

// getCrumb fetches the Yahoo crumb token using the session cookie.
func (f *YahooFinanceFetcher) getCrumb(ctx context.Context, client *http.Client, cookie string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, yahooCrumbURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Cookie", cookie)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get crumb: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(body)), nil
}

func parseTopHoldings(items []topHoldingItem) []symbol.TopHolding {
	if len(items) == 0 {
		return nil
	}
	holdings := make([]symbol.TopHolding, len(items))
	for i, item := range items {
		holdings[i] = symbol.TopHolding{
			Symbol:  item.Symbol,
			Name:    item.HoldingName,
			Percent: item.HoldingPercent,
		}
	}
	return holdings
}

func parseSectorWeightings(items []sectorWeightItem) []symbol.SectorWeighting {
	if len(items) == 0 {
		return nil
	}
	weightings := make([]symbol.SectorWeighting, 0, len(items))
	for _, item := range items {
		for sector, pct := range item {
			weightings = append(weightings, symbol.SectorWeighting{
				Sector:  sector,
				Percent: pct,
			})
		}
	}
	return weightings
}
