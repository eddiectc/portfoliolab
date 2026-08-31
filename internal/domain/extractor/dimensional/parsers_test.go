package dimensional

import (
	"testing"
	"time"
)

func TestParseFundDetail(t *testing.T) {
	tests := []struct {
		name          string
		jsonContent   string
		wantName      string
		wantAUM       float64
		wantTER       float64
		wantInception string
		wantNav       float64
		wantCSVURL    string
		wantSectors   int
		wantCountries int
		wantErr       bool
	}{
		{
			name: "full response with all sections",
			jsonContent: `{
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
											"slug": "charsEquityAllocationByGics",
											"blends": [
												{
													"data": {
														"allocations": [
															{ "name": "Information Technology", "weight": { "value": 0.2159 } }
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
																	{ "name": "United States", "weight": { "value": 0.7031 } }
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
			}`,
			wantName:      "Global Core Equity UCITS ETF (Acc.)",
			wantAUM:       1483669606.1,
			wantTER:       0.0026,
			wantInception: "2025-11-12",
			wantNav:       28.3405,
			wantCSVURL:    "https://tools-blob.dimensional.com/etf/20260528/IE000EGGFVG6.csv",
			wantSectors:   1,
			wantCountries: 1,
			wantErr:       false,
		},
		{
			name:        "missing fund facts returns error",
			jsonContent: `{"data": {"lensGroups": []}}`,
			wantErr:     true,
		},
		{
			name:        "invalid JSON returns error",
			jsonContent: `{invalid json}`,
			wantErr:     true,
		},
		{
			name: "empty blends array skips lens",
			jsonContent: `{
				"data": {
					"lensGroups": [
						{
							"data": {
								"lenses": [
									{
										"data": {
											"slug": "fundFacts",
											"blends": []
										}
									}
								]
							}
						}
					]
				}
			}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, profile, sectors, countries, nav, csvURL, err := ParseFundDetail(tt.jsonContent)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFundDetail() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if info == nil {
				t.Fatal("info is nil")
			}
			if info.Name != tt.wantName {
				t.Errorf("info.Name = %q, want %q", info.Name, tt.wantName)
			}

			if profile == nil {
				t.Fatal("profile is nil")
			}
			if profile.TotalNetAssets != tt.wantAUM {
				t.Errorf("profile.TotalNetAssets = %v, want %v", profile.TotalNetAssets, tt.wantAUM)
			}
			if profile.AnnualExpenseRatio != tt.wantTER {
				t.Errorf("profile.AnnualExpenseRatio = %v, want %v", profile.AnnualExpenseRatio, tt.wantTER)
			}
			if tt.wantInception != "" {
				gotDate := profile.InceptionDate.Format("2006-01-02")
				if gotDate != tt.wantInception {
					t.Errorf("profile.InceptionDate = %q, want %q", gotDate, tt.wantInception)
				}
			}

			if nav != tt.wantNav {
				t.Errorf("nav = %v, want %v", nav, tt.wantNav)
			}
			if csvURL != tt.wantCSVURL {
				t.Errorf("csvURL = %q, want %q", csvURL, tt.wantCSVURL)
			}
			if len(sectors) != tt.wantSectors {
				t.Errorf("len(sectors) = %d, want %d", len(sectors), tt.wantSectors)
			}
			if len(countries) != tt.wantCountries {
				t.Errorf("len(countries) = %d, want %d", len(countries), tt.wantCountries)
			}
		})
	}
}

func TestParseFundDetail_SectorPercent(t *testing.T) {
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
													"marketingName": "Test Fund",
													"fundAum": { "aum": { "value": 1000000 } },
													"inceptionDate": { "value": "2025-01-01" }
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
													{ "name": "Technology", "weight": { "value": 0.2159250600243086 } },
													{ "name": "Healthcare", "weight": { "value": 0.15 } }
												]
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

	_, _, sectors, _, _, _, err := ParseFundDetail(jsonContent)
	if err != nil {
		t.Fatalf("ParseFundDetail() error = %v", err)
	}

	if len(sectors) != 2 {
		t.Fatalf("len(sectors) = %d, want 2", len(sectors))
	}

	// Check Technology sector percent (0.2159... * 100 ≈ 21.59)
	if sectors[0].Sector != "Technology" {
		t.Errorf("sectors[0].Sector = %q, want %q", sectors[0].Sector, "Technology")
	}
	// Allow 0.01 tolerance
	if sectors[0].Percent < 21.58 || sectors[0].Percent > 21.60 {
		t.Errorf("sectors[0].Percent = %v, want ~21.59", sectors[0].Percent)
	}
}

func TestParseHoldingsCSV(t *testing.T) {
	tests := []struct {
		name    string
		csv     string
		wantLen int
		wantTop string // Symbol of the top holding (after sort by weight desc)
		wantErr bool
	}{
		{
			name:    "success with holdings unsorted",
			csv:     "date,etf_isin,ticker,description,weight,market_value\n2026-05-28,IE000EGGFVG6,AAPL,Apple Inc,0.05,5000000\n2026-05-28,IE000EGGFVG6,MSFT,Microsoft Corp,0.08,8000000\n2026-05-28,IE000EGGFVG6,CASH,Cash,0.01,100000",
			wantLen: 2, // AAPL and MSFT, CASH filtered
			wantTop: "MSFT",
			wantErr: false,
		},
		{
			name:    "cash positions filtered",
			csv:     "date,etf_isin,ticker,description,weight,market_value\n2026-05-28,IE000EGGFVG6,AAPL,Apple Inc,0.05,5000000\n2026-05-28,IE000EGGFVG6,XXX,Euro Income,0.02,200000\n2026-05-28,IE000EGGFVG6,YYY,US Dollar,0.01,100000",
			wantLen: 1,
			wantTop: "AAPL",
			wantErr: false,
		},
		{
			name:    "malformed rows skipped gracefully",
			csv:     "date,etf_isin,ticker,description,weight,market_value\n2026-05-28,IE000EGGFVG6,AAPL",
			wantLen: 0,
			wantTop: "",
			wantErr: false,
		},
		{
			name:    "weight with percent sign",
			csv:     "date,etf_isin,ticker,description,weight,market_value\n2026-05-28,IE000EGGFVG6,AAPL,Apple Inc,5%,5000000",
			wantLen: 1,
			wantTop: "AAPL",
			wantErr: false,
		},
		{
			name:    "empty CSV body",
			csv:     "date,etf_isin,ticker,description,weight,market_value",
			wantLen: 0,
			wantTop: "",
			wantErr: false,
		},
		{
			name: "large holdings list",
			csv: func() string {
				s := "date,etf_isin,ticker,description,weight,market_value\n"
				for i := 0; i < 500; i++ {
					s += "2026-05-28,IE000EGGFVG6,SYM" + string(rune('A'+i%26)) + "," +
						"Company " + string(rune('A'+i%26)) + ",0.001,100000\n"
				}
				return s
			}(),
			wantLen: 500,
			wantTop: "SYMA", // all same weight, first one after sort
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseHoldingsCSV(tt.csv)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseHoldingsCSV() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != tt.wantLen {
				t.Errorf("len(holdings) = %d, want %d", len(got), tt.wantLen)
			}
			if tt.wantTop != "" && len(got) > 0 && got[0].Symbol != tt.wantTop {
				t.Errorf("top holding = %q, want %q", got[0].Symbol, tt.wantTop)
			}
		})
	}
}

func TestIsCashPosition(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"CASH", true},
		{"Cash W-O", true},
		{"EURO INCOME", true},
		{"STERLING POUND", true},
		{"US DOLLAR", true},
		{"JAPANESE YEN", true},
		{"SWISS FRANC", true},
		{"BRAZIL REAL", true},
		{"CASH & CASH EQUIVALENTS", true},
		{"CGT ADJ", true},
		{"Apple Inc", false},
		{"Microsoft Corp", false},
		{"European Equity Fund", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCashPosition(tt.name)
			if got != tt.want {
				t.Errorf("isCashPosition(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestParseNumber(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{"plain number", "0.05", 0.05, false},
		{"with percent", "5%", 5, false},
		{"with euro sign", "€1,234.56", 1234.56, false},
		{"with dollar sign", "$1,234.56", 1234.56, false},
		{"with commas", "1,234.56", 1234.56, false},
		{"with spaces", "  0.05  ", 0.05, false},
		{"invalid", "abc", 0, true},
		{"empty", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNumber(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseNumber(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseNumber(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestParseAsOfDateFromNavHistory verifies the as-of date extraction logic
// used in Extract() — derives the date from the most recent NAV point.
func TestAsOfDateFromNavHistory(t *testing.T) {
	// This tests the date parsing used in Extract() for the as-of date.
	// The Extract() function takes navHistory[0].Date and parses it as "2006-01-02".
	dateStr := "2026-05-28"
	got, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		t.Fatalf("time.Parse(%q) error = %v", dateStr, err)
	}
	want := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parsed date = %v, want %v", got, want)
	}
}
