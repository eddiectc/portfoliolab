package dimensional

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/eddiectc/portfoliolab/internal/domain/extractor"
)

type mockClient struct {
	fetchFunc func(url string, headers map[string]string) (string, error)
	postFunc  func(url string, body interface{}, headers map[string]string) (string, error)
}

func (m *mockClient) Fetch(url string, headers map[string]string) (string, error) {
	if m.fetchFunc != nil {
		return m.fetchFunc(url, headers)
	}
	return "", fmt.Errorf("fetch not mocked")
}

func (m *mockClient) Post(url string, body interface{}, headers map[string]string) (string, error) {
	if m.postFunc != nil {
		return m.postFunc(url, body, headers)
	}
	return "", fmt.Errorf("post not mocked")
}

// buildRegistryJSON returns a fund center registry response for the given ISIN.
func buildRegistryJSON(isin string) string {
	return fmt.Sprintf(`{
		"data": {
			"portfolios": [
				{
					"portfolioNumber": 1600,
					"meta": {
						"marketingName": "Global Core Equity UCITS ETF (Acc.)",
						"isEtf": true,
						"isDfaUcitsEtf": true,
						"identifiers": [ { "slug": "isin", "value": "%s" } ]
					},
					"prices": [
						{ "date": { "value": "2026-05-28" }, "nav": { "value": 28.34 } },
						{ "date": { "value": "2026-05-27" }, "nav": { "value": 28.20 } }
					]
				}
			]
		}
	}`, isin)
}

// buildMutualFundRegistryJSON returns a registry containing a mutual fund
// (DDGT) and its corresponding UCITS ETF share class (GTV UCITS ETF).
func buildMutualFundRegistryJSON() string {
	return `{
		"data": {
			"portfolios": [
				{
					"portfolioNumber": 406,
					"meta": {
						"marketingName": "Global Targeted Value Fund (USD, Acc.)",
						"isEtf": false,
						"isDfaUcitsEtf": false,
						"identifiers": [ { "slug": "isin", "value": "IE00B2PC0609" } ]
					},
					"prices": [
						{ "date": { "value": "2026-08-31" }, "nav": { "value": 100.0 } }
					]
				},
				{
					"portfolioNumber": 1602,
					"meta": {
						"marketingName": "Global Targeted Value UCITS ETF (Acc.)",
						"isEtf": true,
						"isDfaUcitsEtf": true,
						"identifiers": [ { "slug": "isin", "value": "IE000S67ID55" } ]
					},
					"prices": [
						{ "date": { "value": "2026-08-31" }, "nav": { "value": 28.34 } }
					]
				}
			]
		}
	}`
}

// buildMutualFundDetailJSON returns a fund detail response for a mutual fund:
// fundFacts with isEtf/isDfaUcitsEtf false and no charsEtfTopHoldingsDaily lens
// (Dimensional only publishes full holdings for UCITS ETFs).
func buildMutualFundDetailJSON(marketingName string) string {
	return fmt.Sprintf(`{
		"data": {
			"lensGroups": [
				{
					"data": {
						"lenses": [
							{
								"data": {
									"slug": "fundFacts",
									"blends": [
										{
											"data": {
												"fundFacts": {
													"marketingName": %q,
													"isEtf": false,
													"isDfaUcitsEtf": false,
													"fundAum": { "aum": { "value": 99979314.0 } },
													"inceptionDate": { "value": "1994-12-30" }
												}
											}
										}
									]
								}
							}
						]
					}
				}
			]
		}
	}`, marketingName)
}

// buildDetailJSON returns a minimal fund detail response with all required sections.
func buildDetailJSON(csvURL string) string {
	return fmt.Sprintf(`{
		"data": {
			"lensGroups": [
				{
					"data": {
						"lenses": [
							{
								"data": {
									"slug": "fundFacts",
									"blends": [
										{
											"data": {
												"fundFacts": {
													"marketingName": "Global Core Equity UCITS ETF (Acc.)",
													"fundAum": { "aum": { "value": 1483669606.1 } },
													"inceptionDate": { "value": "2025-11-12" }
												}
											}
										}
									]
								}
							},
							{
								"data": {
									"slug": "fundPrices",
									"blends": [
										{
											"data": {
												"fundPrices": {
													"prices": [ { "nav": { "value": 28.3405 } } ]
												}
											}
										}
									]
								}
							},
							{
								"data": {
									"slug": "fees",
									"blends": [
										{
											"data": {
												"fees": {
													"fees": [ { "slug": "net-exp-ratio", "value": { "value": 0.0026 } } ]
												}
											}
										}
									]
								}
							},
							{
								"data": {
									"slug": "charsEtfTopHoldingsDaily",
									"blends": [
										{
											"data": {
												"fullHoldingsCsvUrl": "%s"
											}
										}
									]
								}
							}
						]
					}
				}
			]
		}
	}`, csvURL)
}

func TestExtractor_Extract(t *testing.T) {
	isin := "IE000EGGFVG6"
	sourceURL := fmt.Sprintf("https://www.dimensional.com/gb-en/funds/%s/global-core-equity-ucits-etf-acc", isin)
	csvURL := "https://tools-blob.dimensional.com/etf/20260528/IE000EGGFVG6.csv"

	tests := []struct {
		name       string
		fetchFunc  func(url string, headers map[string]string) (string, error)
		postFunc   func(url string, body interface{}, headers map[string]string) (string, error)
		ctx        context.Context
		sourceURL  string
		wantErr    bool
		wantErrSub string // substring expected in error message
		validate   func(t *testing.T, result *extractor.ExtractResult)
	}{
		{
			name: "success — full extraction",
			fetchFunc: func(url string, headers map[string]string) (string, error) {
				if url == "https://etf.dimensional.com/public/v2/fundcenter?allowMorningstarFixedIncome=true" {
					return buildRegistryJSON(isin), nil
				}
				if url == csvURL {
					return "date,etf_isin,ticker,description,weight,market_value\n2026-05-28,IE000EGGFVG6,AAPL,Apple Inc,0.05,5000000\n", nil
				}
				return "", fmt.Errorf("unexpected fetch URL: %s", url)
			},
			postFunc: func(url string, body interface{}, headers map[string]string) (string, error) {
				if url == "https://etf.dimensional.com/public/v2/fundcenter/funddetail" {
					return buildDetailJSON(csvURL), nil
				}
				return "", fmt.Errorf("unexpected post URL: %s", url)
			},
			sourceURL: sourceURL,
			wantErr:   false,
			validate: func(t *testing.T, result *extractor.ExtractResult) {
				if result == nil {
					t.Fatal("result is nil")
				}
				if result.AsOfDate.IsZero() {
					t.Error("AsOfDate is zero")
				}
				switch {
				case result.FundInfo == nil:
					t.Error("FundInfo is nil")
				case result.FundInfo.Symbol != isin:
					t.Errorf("FundInfo.Symbol = %q, want %q", result.FundInfo.Symbol, isin)
				case result.FundInfo.Name != "Global Core Equity UCITS ETF (Acc.)":
					t.Errorf("FundInfo.Name = %q", result.FundInfo.Name)
				}
				if len(result.NavHistory) != 2 {
					t.Errorf("len(NavHistory) = %d, want 2", len(result.NavHistory))
				}
				if len(result.Holdings) != 1 {
					t.Errorf("len(Holdings) = %d, want 1", len(result.Holdings))
				} else if result.Holdings[0].Symbol != "AAPL" {
					t.Errorf("Holdings[0].Symbol = %q, want %q", result.Holdings[0].Symbol, "AAPL")
				}
			},
		},
		{
			name: "registry API failure",
			fetchFunc: func(url string, headers map[string]string) (string, error) {
				return "", fmt.Errorf("registry api failure")
			},
			sourceURL:  sourceURL,
			wantErr:    true,
			wantErrSub: "map ISIN to portfolio number",
		},
		{
			name: "ISIN not found in registry",
			fetchFunc: func(url string, headers map[string]string) (string, error) {
				return buildRegistryJSON("IE000XXXXXXXX"), nil
			},
			sourceURL:  sourceURL,
			wantErr:    true,
			wantErrSub: "not found in fund center",
		},
		{
			name: "fund detail API failure",
			fetchFunc: func(url string, headers map[string]string) (string, error) {
				return buildRegistryJSON(isin), nil
			},
			postFunc: func(url string, body interface{}, headers map[string]string) (string, error) {
				return "", fmt.Errorf("detail api failure")
			},
			sourceURL:  sourceURL,
			wantErr:    true,
			wantErrSub: "fetch fund detail",
		},
		{
			name: "holdings CSV fetch failure",
			fetchFunc: func(url string, headers map[string]string) (string, error) {
				if url == "https://etf.dimensional.com/public/v2/fundcenter?allowMorningstarFixedIncome=true" {
					return buildRegistryJSON(isin), nil
				}
				if url == csvURL {
					return "", fmt.Errorf("csv fetch failed")
				}
				return "", fmt.Errorf("unexpected fetch URL: %s", url)
			},
			postFunc: func(url string, body interface{}, headers map[string]string) (string, error) {
				return buildDetailJSON(csvURL), nil
			},
			sourceURL:  sourceURL,
			wantErr:    true,
			wantErrSub: "fetch holdings CSV",
		},
		{
			name: "empty holdings after parse — atomic failure",
			fetchFunc: func(url string, headers map[string]string) (string, error) {
				if url == "https://etf.dimensional.com/public/v2/fundcenter?allowMorningstarFixedIncome=true" {
					return buildRegistryJSON(isin), nil
				}
				if url == csvURL {
					// CSV with only cash positions — all filtered out
					return "date,etf_isin,ticker,description,weight,market_value\n2026-05-28,IE000EGGFVG6,CASH,Cash,0.01,100000\n", nil
				}
				return "", fmt.Errorf("unexpected fetch URL: %s", url)
			},
			postFunc: func(url string, body interface{}, headers map[string]string) (string, error) {
				return buildDetailJSON(csvURL), nil
			},
			sourceURL:  sourceURL,
			wantErr:    true,
			wantErrSub: "holdings list is empty",
		},
		{
			name: "missing CSV URL in detail response",
			fetchFunc: func(url string, headers map[string]string) (string, error) {
				return buildRegistryJSON(isin), nil
			},
			postFunc: func(url string, body interface{}, headers map[string]string) (string, error) {
				// ETF flags present but no charsEtfTopHoldingsDaily lens — generic error path
				return `{"data":{"lensGroups":[{"data":{"lenses":[{"data":{"slug":"fundFacts","blends":[{"data":{"fundFacts":{"marketingName":"Test","isEtf":true,"isDfaUcitsEtf":true,"fundAum":{"aum":{"value":1}},"inceptionDate":{"value":"2025-01-01"}}}}]}}]}}]}}`, nil
			},
			sourceURL:  sourceURL,
			wantErr:    true,
			wantErrSub: "full holdings CSV URL not found",
		},
		{
			name: "mutual fund — suggests corresponding UCITS ETF",
			fetchFunc: func(url string, headers map[string]string) (string, error) {
				return buildMutualFundRegistryJSON(), nil
			},
			postFunc: func(url string, body interface{}, headers map[string]string) (string, error) {
				return buildMutualFundDetailJSON("Global Targeted Value Fund (USD, Acc.)"), nil
			},
			sourceURL:  "https://www.dimensional.com/gb-en/funds/IE00B2PC0609/global-targeted-value-fund-usd-acc",
			wantErr:    true,
			wantErrSub: "use the UCITS ETF share class IE000S67ID55 instead",
		},
		{
			name: "mutual fund — no corresponding UCITS ETF in registry",
			fetchFunc: func(url string, headers map[string]string) (string, error) {
				return buildMutualFundRegistryJSON(), nil
			},
			postFunc: func(url string, body interface{}, headers map[string]string) (string, error) {
				return buildMutualFundDetailJSON("Some Oddly Named Fund (XYZ, Acc.)"), nil
			},
			sourceURL:  "https://www.dimensional.com/gb-en/funds/IE00B2PC0609/global-targeted-value-fund-usd-acc",
			wantErr:    true,
			wantErrSub: "full holdings are only published for UCITS ETFs",
		},
		{
			name: "context cancelled",
			fetchFunc: func(url string, headers map[string]string) (string, error) {
				return "", fmt.Errorf("should not be called")
			},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			sourceURL:  sourceURL,
			wantErr:    true,
			wantErrSub: "context canceled",
		},
		{
			name: "invalid ISIN in URL",
			fetchFunc: func(url string, headers map[string]string) (string, error) {
				return "", fmt.Errorf("should not be called")
			},
			sourceURL:  "https://www.dimensional.com/gb-en/funds/not-a-valid-isin/fund-name",
			wantErr:    true,
			wantErrSub: "ISIN not found in URL",
		},
		{
			name: "NAV value as int in registry",
			fetchFunc: func(url string, headers map[string]string) (string, error) {
				if url == "https://etf.dimensional.com/public/v2/fundcenter?allowMorningstarFixedIncome=true" {
					return fmt.Sprintf(`{
						"data": {
							"portfolios": [
								{
									"portfolioNumber": 1600,
									"meta": { "identifiers": [ { "slug": "isin", "value": "%s" } ] },
									"prices": [ { "date": { "value": "2026-05-28" }, "nav": { "value": 28 } } ]
								}
							]
						}
					}`, isin), nil
				}
				if url == csvURL {
					return "date,etf_isin,ticker,description,weight,market_value\n2026-05-28,IE000EGGFVG6,AAPL,Apple Inc,0.05,5000000\n", nil
				}
				return "", fmt.Errorf("unexpected fetch URL: %s", url)
			},
			postFunc: func(url string, body interface{}, headers map[string]string) (string, error) {
				return buildDetailJSON(csvURL), nil
			},
			sourceURL: sourceURL,
			wantErr:   false,
			validate: func(t *testing.T, result *extractor.ExtractResult) {
				if result == nil {
					t.Fatal("result is nil")
				}
				if len(result.NavHistory) != 1 {
					t.Errorf("len(NavHistory) = %d, want 1", len(result.NavHistory))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewExtractor()
			mock := &mockClient{
				fetchFunc: tt.fetchFunc,
				postFunc:  tt.postFunc,
			}
			extractor.client = mock

			ctx := tt.ctx
			if ctx == nil {
				ctx = context.Background()
			}

			result, err := extractor.Extract(ctx, tt.sourceURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("Extract() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.wantErrSub != "" {
				if err != nil && !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErrSub)
				}
				return
			}
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestSuggestUcitsEtf(t *testing.T) {
	var reg struct {
		Data struct {
			Portfolios []fundCenterEntry `json:"portfolios"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(buildMutualFundRegistryJSON()), &reg); err != nil {
		t.Fatalf("unmarshal registry: %v", err)
	}
	entries := reg.Data.Portfolios

	tests := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{name: "matches corresponding UCITS ETF", input: "Global Targeted Value Fund (USD, Acc.)", want: "IE000S67ID55", ok: true},
		{name: "name without currency suffix", input: "Global Targeted Value Fund", want: "IE000S67ID55", ok: true},
		{name: "no matching UCITS ETF", input: "Some Oddly Named Fund (XYZ, Acc.)", want: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := suggestUcitsEtf(entries, tt.input)
			if ok != tt.ok || got != tt.want {
				t.Errorf("suggestUcitsEtf(%q) = (%q, %v), want (%q, %v)", tt.input, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestExtractISIN(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "lowercase ISIN in URL",
			url:  "https://www.dimensional.com/gb-en/funds/ie000eggfvg6/global-core-equity-ucits-etf-acc",
			want: "ie000eggfvg6",
		},
		{
			name: "uppercase ISIN in URL (normalized to lowercase by regex)",
			url:  "https://www.dimensional.com/gb-en/funds/IE000EGGFVG6/global-core-equity-ucits-etf-acc",
			want: "ie000eggfvg6",
		},
		{
			name:    "no ISIN in URL",
			url:     "https://www.dimensional.com/gb-en/about",
			want:    "",
			wantErr: true,
		},
		{
			name:    "empty URL",
			url:     "",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractISIN(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractISIN(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("extractISIN(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestExtractor_Name(t *testing.T) {
	extractor := NewExtractor()
	if got := extractor.Name(); got != Name {
		t.Errorf("Name() = %q, want %q", got, Name)
	}
}

func TestExtractor_SetClient(t *testing.T) {
	extractor := NewExtractor()
	c := NewClient()
	extractor.SetClient(c)
	if extractor.client != c {
		t.Error("SetClient did not set the client")
	}
}
