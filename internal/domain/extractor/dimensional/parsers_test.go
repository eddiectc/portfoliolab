package dimensional

import (
	"testing"

	"github.com/govalues/decimal"
	"github.com/stretchr/testify/assert"
)

func TestParseFundDetail(t *testing.T) {
	jsonContent := `{
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
													"fundAum": {
														"aum": { "value": 1483669606.1 }
													},
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
									"slug": "charsEquityAllocationByGics",
									"blends": [
										{
											"data": {
												"allocations": [
													{ "name": "Information Technology", "weight": { "value": 0.2159250600243086 } }
												]
											}
										}
									]
								}
							},
							{
								"data": {
									"slug": "charsEquityAllocationByCountryByRegionDevelopedEmerging",
									"blends": [
										{
											"data": {
												"allocations": [
													{
														"name": "Developed",
														"subCategories": [
															{ "name": "United States", "weight": { "value": 0.7031153345359613 } }
														]
													}
												]
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
												"fullHoldingsCsvUrl": "https://tools-blob.dimensional.com/etf/20260528/IE000EGGFVG6.csv"
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
	}`

	info, profile, sectors, countries, nav, csvURL, err := ParseFundDetail(jsonContent)
	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.Equal(t, "Global Core Equity UCITS ETF (Acc.)", info.Name)
	assert.NotNil(t, profile)
	assert.Equal(t, 1483669606.1, profile.TotalNetAssets)
	assert.Equal(t, 0.0026, profile.AnnualExpenseRatio)
	assert.Equal(t, 28.3405, nav)
	assert.Equal(t, "https://tools-blob.dimensional.com/etf/20260528/IE000EGGFVG6.csv", csvURL)
	assert.Len(t, sectors, 1)
	assert.Equal(t, "Information Technology", sectors[0].Sector)
	assert.InDelta(t, 21.59, sectors[0].Percent, 0.01)
	assert.Len(t, countries, 1)
	assert.Equal(t, "United States", countries[0].Country)
	assert.InDelta(t, 70.31, countries[0].Percent, 0.01)
}

func TestParseHoldingsCSV(t *testing.T) {
	tests := []struct {
		name    string
		csv     string
		wantLen int
		wantTop string // Symbol of the top holding
		wantErr bool
	}{
		{
			name: "Success with holdings (unsorted)",
			csv: "date,etf_isin,ticker,description,weight,market_value\n2026-05-28,IE000EGGFVG6,AAPL,Apple Inc,0.05,5000000\n2026-05-28,IE000EGGFVG6,MSFT,Microsoft Corp,0.08,8000000\n2026-05-28,IE000EGGFVG6,CASH,Cash,0.01,100000",
			wantLen: 2, // AAPL and MSFT, CASH should be filtered
			wantTop: "MSFT",
			wantErr: false,
		},
		{
			name: "Fail invalid CSV",
			csv: "date,etf_isin,ticker,description,weight,market_value\n2026-05-28,IE000EGGFVG6,AAPL",
			wantLen: 0,
			wantErr: false, // skip malformed rows
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseHoldingsCSV(tt.csv)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseHoldingsCSV() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				assert.Equal(t, tt.wantLen, len(got))
				if tt.wantTop != "" && len(got) > 0 {
					assert.Equal(t, tt.wantTop, got[0].Symbol)
				}
			}
		})
	}
}
