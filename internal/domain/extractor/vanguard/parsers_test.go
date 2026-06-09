package vanguard

import (
	"encoding/json"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

// --- ParseFundIdentity tests ---

func TestParseFundIdentity(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		wantSymbol  string
		wantName    string
		wantPortId  string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid fund identity",
			json: `{
				"name": "Vanguard FTSE All-World UCITS ETF",
				"ticker": "VWRL",
				"sedol": "BYVTR12",
				"portId": "9505",
				"inceptionDate": "2019-09-24",
				"isin": "IE00BK5BQT80",
				"currencyCode": "USD"
			}`,
			wantSymbol: "VWRL",
			wantName:   "Vanguard FTSE All-World UCITS ETF",
			wantPortId: "9505",
		},
		{
			name: "missing ticker",
			json: `{
				"name": "Test Fund",
				"portId": "9505"
			}`,
			wantErr:     true,
			errContains: "missing ticker",
		},
		{
			name: "missing name",
			json: `{
				"ticker": "VWRL",
				"portId": "9505"
			}`,
			wantErr:     true,
			errContains: "missing name",
		},
		{
			name: "missing portId",
			json: `{
				"name": "Test Fund",
				"ticker": "VWRL"
			}`,
			wantErr:     true,
			errContains: "missing portId",
		},
		{
			name:        "invalid JSON",
			json:        `{invalid}`,
			wantErr:     true,
			errContains: "unmarshal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, portId, err := ParseFundIdentity([]byte(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if info.Symbol != tt.wantSymbol {
				t.Errorf("Symbol: got %q, want %q", info.Symbol, tt.wantSymbol)
			}
			if info.Name != tt.wantName {
				t.Errorf("Name: got %q, want %q", info.Name, tt.wantName)
			}
			if portId != tt.wantPortId {
				t.Errorf("PortId: got %q, want %q", portId, tt.wantPortId)
			}
		})
	}
}

// --- ParseFundProfile tests ---

func TestParseFundProfile(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		check   func(t *testing.T, profile *extractor.FundProfile)
	}{
		{
			name: "valid profile with OCF percentage",
			json: `{
				"OCF": "0.40%",
				"benchmark": "FTSE All-World Index",
				"managementType": "Index",
				"assetClass": "Equity",
				"fundType": "etf",
				"distributionStrategyType": "INCM",
				"region": "Global",
				"inceptionDate": "2019-09-24",
				"isin": "IE00BK5BQT80"
			}`,
			check: func(t *testing.T, profile *extractor.FundProfile) {
				if profile.AnnualExpenseRatio != 0.004 {
					t.Errorf("AnnualExpenseRatio: got %f, want 0.004", profile.AnnualExpenseRatio)
				}
				if profile.Family != "Index" {
					t.Errorf("Family: got %q, want %q", profile.Family, "Index")
				}
				if profile.LegalType != "etf" {
					t.Errorf("LegalType: got %q, want %q", profile.LegalType, "etf")
				}
				if profile.InceptionDate.IsZero() {
					t.Error("InceptionDate is zero")
				}
				if profile.Isin != "IE00BK5BQT80" {
					t.Errorf("Isin: got %q, want %q", profile.Isin, "IE00BK5BQT80")
				}
			},
		},
		{
			name: "OCF without percent sign",
			json: `{
				"OCF": "0.75",
				"managementType": "Active",
				"fundType": "fund"
			}`,
			check: func(t *testing.T, profile *extractor.FundProfile) {
				if profile.AnnualExpenseRatio != 0.0075 {
					t.Errorf("AnnualExpenseRatio: got %f, want 0.0075", profile.AnnualExpenseRatio)
				}
			},
		},
		{
			name: "empty OCF",
			json: `{
				"OCF": "",
				"managementType": "Index"
			}`,
			wantErr: false,
			check: func(t *testing.T, profile *extractor.FundProfile) {
				if profile.AnnualExpenseRatio != 0 {
					t.Errorf("AnnualExpenseRatio: got %f, want 0", profile.AnnualExpenseRatio)
				}
			},
		},
		{
			name:    "all fields empty",
			json:    `{"name": "Test Fund", "ticker": "TEST"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := ParseFundProfile([]byte(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, profile)
			}
		})
	}
}

// --- ParseHoldings tests ---

func TestParseHoldings(t *testing.T) {
	equityHolding := map[string]interface{}{
		"effectiveDate":           "2026-04-30",
		"marketValuePercentage":   1.399,
		"issuerName":              "Apple Inc.",
		"securityLongDescription": "Apple Inc. Common Stock",
		"securityType":            "EQ.STOCK",
		"ticker":                  "AAPL",
		"isin":                    "US0378331005",
		"sedol1":                  "2046251",
	}

	bondHolding := map[string]interface{}{
		"effectiveDate":           "2026-04-30",
		"marketValuePercentage":   0.5,
		"issuerName":              "US Treasury",
		"securityLongDescription": "US Treasury Note 2.5%",
		"couponRate":              2.5,
		"securityType":            "FI.US_GOV",
		"finalMaturity":           "2032-06-15",
		"ticker":                  "",
		"isin":                    "US912810TM82",
		"sedol1":                  "2401265",
	}

	tests := []struct {
		name        string
		items       []interface{}
		wantCount   int
		wantErr     bool
		errContains string
		checkFirst  func(t *testing.T, h extractor.Holding)
	}{
		{
			name:      "equity holding",
			items:     []interface{}{equityHolding},
			wantCount: 1,
			checkFirst: func(t *testing.T, h extractor.Holding) {
				if h.Name != "Apple Inc." {
					t.Errorf("Name: got %q, want %q", h.Name, "Apple Inc.")
				}
				if h.Symbol != "AAPL" {
					t.Errorf("Symbol: got %q, want %q", h.Symbol, "AAPL")
				}
				if h.Percent != 1.399 {
					t.Errorf("Percent: got %f, want 1.399", h.Percent)
				}
				if h.SecurityType != "EQ.STOCK" {
					t.Errorf("SecurityType: got %q, want %q", h.SecurityType, "EQ.STOCK")
				}
				if h.ISIN != "US0378331005" {
					t.Errorf("ISIN: got %q, want %q", h.ISIN, "US0378331005")
				}
				if h.CouponRate != nil {
					t.Errorf("CouponRate: got %v, want nil", h.CouponRate)
				}
				if h.FinalMaturity != nil {
					t.Errorf("FinalMaturity: got %v, want nil", h.FinalMaturity)
				}
				if h.AsOfDate != "2026-04-30" {
					t.Errorf("AsOfDate: got %q, want %q", h.AsOfDate, "2026-04-30")
				}
			},
		},
		{
			name:      "bond holding with coupon and maturity",
			items:     []interface{}{bondHolding},
			wantCount: 1,
			checkFirst: func(t *testing.T, h extractor.Holding) {
				if h.CouponRate == nil {
					t.Fatal("CouponRate is nil")
				} else if *h.CouponRate != 2.5 {
					t.Errorf("CouponRate: got %f, want 2.5", *h.CouponRate)
				}
				if h.FinalMaturity == nil {
					t.Fatal("FinalMaturity is nil")
				} else if *h.FinalMaturity != "2032-06-15" {
					t.Errorf("FinalMaturity: got %q, want %q", *h.FinalMaturity, "2032-06-15")
				}
			},
		},
		{
			name:      "empty holdings",
			items:     []interface{}{},
			wantCount: 0,
		},
		{
			name:        "missing effectiveDate",
			items:       []interface{}{map[string]interface{}{"marketValuePercentage": 1.0}},
			wantErr:     true,
			errContains: "missing effectiveDate",
		},
		{
			name: "multiple holdings",
			items: []interface{}{
				equityHolding,
				bondHolding,
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := map[string]interface{}{
				"borHoldings": []map[string]interface{}{
					{
						"holdings": map[string]interface{}{
							"totalHoldings": len(tt.items),
							"items":         tt.items,
						},
					},
				},
			}
			data, _ := buildGraphQLResponse(resp)

			holdings, date, err := ParseHoldings([]byte(data))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(holdings) != tt.wantCount {
				t.Errorf("got %d holdings, want %d", len(holdings), tt.wantCount)
			}
			if tt.checkFirst != nil && len(holdings) > 0 {
				tt.checkFirst(t, holdings[0])
			}
			if tt.wantCount > 0 && date == "" {
				t.Error("effectiveDate is empty")
			}
		})
	}
}

// --- ParseSectorAllocation tests ---

func TestParseSectorAllocation(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		wantCount   int
		wantErr     bool
		errContains string
		checkFirst  func(t *testing.T, s extractor.SectorWeighting)
	}{
		{
			name: "valid sector allocation",
			json: buildJSONResponse(map[string]interface{}{
				"funds": []interface{}{
					map[string]interface{}{
						"sectorDiversification": []interface{}{
							map[string]interface{}{
								"sectorCode":       "10",
								"date":             "2026-04-30",
								"sectorName":       "Technology",
								"fundPercent":      25.5,
								"benchmarkPercent": 28.0,
							},
							map[string]interface{}{
								"sectorCode":       "20",
								"date":             "2026-04-30",
								"sectorName":       "Financials",
								"fundPercent":      15.0,
								"benchmarkPercent": 14.5,
							},
						},
					},
				},
			}),
			wantCount: 2,
			checkFirst: func(t *testing.T, s extractor.SectorWeighting) {
				if s.Sector != "Technology" {
					t.Errorf("Sector: got %q, want %q", s.Sector, "Technology")
				}
				if s.Percent != 25.5 {
					t.Errorf("Percent: got %f, want 25.5", s.Percent)
				}
				if s.Date != "2026-04-30" {
					t.Errorf("Date: got %q, want %q", s.Date, "2026-04-30")
				}
			},
		},
		{
			name: "empty sectors",
			json: buildJSONResponse(map[string]interface{}{
				"funds": []interface{}{
					map[string]interface{}{
						"sectorDiversification": []interface{}{},
					},
				},
			}),
			wantCount: 0,
		},
		{
			name:        "no funds data",
			json:        buildJSONResponse(map[string]interface{}{"funds": []interface{}{}}),
			wantErr:     true,
			errContains: "no funds data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sectors, date, err := ParseSectorAllocation([]byte(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(sectors) != tt.wantCount {
				t.Errorf("got %d sectors, want %d", len(sectors), tt.wantCount)
			}
			if tt.checkFirst != nil && len(sectors) > 0 {
				tt.checkFirst(t, sectors[0])
			}
			if tt.wantCount > 0 && date == "" {
				t.Error("date is empty")
			}
		})
	}
}

// --- ParseCountryAllocation tests ---

func TestParseCountryAllocation(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		wantCount   int
		wantErr     bool
		errContains string
		checkFirst  func(t *testing.T, c extractor.CountryAllocation)
	}{
		{
			name: "valid country allocation with region",
			json: buildJSONResponse(map[string]interface{}{
				"funds": []interface{}{
					map[string]interface{}{
						"marketAllocation": []interface{}{
							map[string]interface{}{
								"portId":              "9505",
								"date":                "2026-04-30",
								"countryCode":         "US",
								"countryName":         "United States",
								"fundMktPercent":      60.5,
								"benchmarkMktPercent": 58.0,
								"regionCode":          "NA",
								"regionName":          "North America",
								"holdingStatCode":     "FTCTYATPCS",
							},
							map[string]interface{}{
								"portId":              "9505",
								"date":                "2026-04-30",
								"countryCode":         "GB",
								"countryName":         "United Kingdom",
								"fundMktPercent":      5.2,
								"benchmarkMktPercent": 5.0,
								"regionCode":          "EU",
								"regionName":          "Europe",
								"holdingStatCode":     "FTCTYATPCS",
							},
						},
					},
				},
			}),
			wantCount: 2,
			checkFirst: func(t *testing.T, c extractor.CountryAllocation) {
				if c.Country != "United States" {
					t.Errorf("Country: got %q, want %q", c.Country, "United States")
				}
				if c.Percent != 60.5 {
					t.Errorf("Percent: got %f, want 60.5", c.Percent)
				}
				if c.RegionName != "North America" {
					t.Errorf("RegionName: got %q, want %q", c.RegionName, "North America")
				}
				if c.RegionCode != "NA" {
					t.Errorf("RegionCode: got %q, want %q", c.RegionCode, "NA")
				}
				if c.Date != "2026-04-30" {
					t.Errorf("Date: got %q, want %q", c.Date, "2026-04-30")
				}
			},
		},
		{
			name: "empty countries",
			json: buildJSONResponse(map[string]interface{}{
				"funds": []interface{}{
					map[string]interface{}{
						"marketAllocation": []interface{}{},
					},
				},
			}),
			wantCount: 0,
		},
		{
			name: "filters out MSCTYATPCS and region subtotals",
			json: buildJSONResponse(map[string]interface{}{
				"funds": []interface{}{
					map[string]interface{}{
						"marketAllocation": []interface{}{
							// FTCTYATPCS — kept
							map[string]interface{}{
								"portId":          "9505",
								"date":            "2026-04-30",
								"countryCode":     "US",
								"countryName":     "United States",
								"fundMktPercent":  61.57,
								"regionCode":      "NA",
								"regionName":      "North America",
								"holdingStatCode": "FTCTYATPCS",
							},
							// MSCTYATPCS (Market of Domicile) — filtered out
							map[string]interface{}{
								"portId":          "9505",
								"date":            "2026-04-30",
								"countryCode":     "US",
								"countryName":     "United States",
								"fundMktPercent":  61.50,
								"regionCode":      "NA",
								"regionName":      "North America",
								"holdingStatCode": "MSCTYATPCS",
							},
							// SASTTYPPC (region subtotal) — filtered out
							map[string]interface{}{
								"portId":          "9505",
								"date":            "2026-04-30",
								"countryCode":     nil,
								"countryName":     nil,
								"fundMktPercent":  64.64,
								"regionCode":      "NA",
								"regionName":      "North America",
								"holdingStatCode": "SASTTYPPC",
							},
							// Another FTCTYATPCS — kept
							map[string]interface{}{
								"portId":          "9505",
								"date":            "2026-04-30",
								"countryCode":     "DE",
								"countryName":     "Germany",
								"fundMktPercent":  1.93,
								"regionCode":      "EU",
								"regionName":      "Europe",
								"holdingStatCode": "FTCTYATPCS",
							},
						},
					},
				},
			}),
			wantCount: 2,
			checkFirst: func(t *testing.T, c extractor.CountryAllocation) {
				if c.Country != "United States" {
					t.Errorf("Country: got %q, want %q", c.Country, "United States")
				}
				if c.Percent != 61.57 {
					t.Errorf("Percent: got %f, want 61.57", c.Percent)
				}
			},
		},
		{
			name:        "no funds data",
			json:        buildJSONResponse(map[string]interface{}{"funds": []interface{}{}}),
			wantErr:     true,
			errContains: "no funds data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			countries, date, err := ParseCountryAllocation([]byte(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(countries) != tt.wantCount {
				t.Errorf("got %d countries, want %d", len(countries), tt.wantCount)
			}
			if tt.checkFirst != nil && len(countries) > 0 {
				tt.checkFirst(t, countries[0])
			}
			if tt.wantCount > 0 && date == "" {
				t.Error("date is empty")
			}
		})
	}
}

// --- ParseFundCharacteristics tests ---

func TestParseFundCharacteristics(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		check   func(t *testing.T, c *extractor.FundCharacteristics)
	}{
		{
			name: "equity fund characteristics",
			json: buildJSONResponse(map[string]interface{}{
				"polarisAnalyticsHistory": []interface{}{
					map[string]interface{}{
						"portId": "9505",
						"monthly": map[string]interface{}{
							"analytics": map[string]interface{}{
								"fund": map[string]interface{}{
									"items": []interface{}{
										map[string]interface{}{
											"codes": map[string]interface{}{
												"PERATIO":    map[string]interface{}{"analyticValue": "18.5"},
												"PBRATIO":    map[string]interface{}{"analyticValue": "3.2"},
												"MKTCAPMEDN": map[string]interface{}{"analyticValue": "500.0"},
												"FRC5YRROE":  map[string]interface{}{"analyticValue": "15.3"},
												"EPSFRC5YR":  map[string]interface{}{"analyticValue": "8.7"},
												"TRNVRRPTR":  map[string]interface{}{"analyticValue": "1.05"},
											},
										},
									},
								},
							},
						},
					},
				},
			}),
			check: func(t *testing.T, c *extractor.FundCharacteristics) {
				if c.PriceToEarnings != 18.5 {
					t.Errorf("PriceToEarnings: got %f, want 18.5", c.PriceToEarnings)
				}
				if c.PriceToBook != 3.2 {
					t.Errorf("PriceToBook: got %f, want 3.2", c.PriceToBook)
				}
				if c.MedianMarketCap != 500.0 {
					t.Errorf("MedianMarketCap: got %f, want 500.0", c.MedianMarketCap)
				}
				if c.ForwardROE != 15.3 {
					t.Errorf("ForwardROE: got %f, want 15.3", c.ForwardROE)
				}
				if c.ForwardEPSGrowth != 8.7 {
					t.Errorf("ForwardEPSGrowth: got %f, want 8.7", c.ForwardEPSGrowth)
				}
				if c.RevenueRatio != 1.05 {
					t.Errorf("RevenueRatio: got %f, want 1.05", c.RevenueRatio)
				}
				if c.AverageCoupon != 0 {
					t.Errorf("AverageCoupon: got %f, want 0", c.AverageCoupon)
				}
			},
		},
		{
			name: "bond fund characteristics",
			json: buildJSONResponse(map[string]interface{}{
				"polarisAnalyticsHistory": []interface{}{
					map[string]interface{}{
						"portId": "9505",
						"monthly": map[string]interface{}{
							"analytics": map[string]interface{}{
								"fund": map[string]interface{}{
									"items": []interface{}{
										map[string]interface{}{
											"codes": map[string]interface{}{
												"AVGCPN":     map[string]interface{}{"analyticValue": "3.25"},
												"AVGWTDMTY":  map[string]interface{}{"analyticValue": "7.5"},
												"AVGQLYTFTO": map[string]interface{}{"analyticValue": "7.8"},
												"AVGDURADJ":  map[string]interface{}{"analyticValue": "6.2"},
											},
										},
									},
								},
							},
						},
					},
				},
			}),
			check: func(t *testing.T, c *extractor.FundCharacteristics) {
				if c.AverageCoupon != 3.25 {
					t.Errorf("AverageCoupon: got %f, want 3.25", c.AverageCoupon)
				}
				if c.AverageMaturity != 7.5 {
					t.Errorf("AverageMaturity: got %f, want 7.5", c.AverageMaturity)
				}
				if c.AverageQuality != 7.8 {
					t.Errorf("AverageQuality: got %f, want 7.8", c.AverageQuality)
				}
				if c.AverageDuration != 6.2 {
					t.Errorf("AverageDuration: got %f, want 6.2", c.AverageDuration)
				}
				if c.PriceToEarnings != 0 {
					t.Errorf("PriceToEarnings: got %f, want 0", c.PriceToEarnings)
				}
			},
		},
		{
			name:    "no analytics data",
			json:    buildJSONResponse(map[string]interface{}{"polarisAnalyticsHistory": []interface{}{}}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			char, err := ParseFundCharacteristics([]byte(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, char)
			}
		})
	}
}

// --- ParseNavHistory tests ---

func TestParseNavHistory(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		wantCount int
		wantErr   bool
		check     func(t *testing.T, points []extractor.NavPoint)
	}{
		{
			name: "valid NAV history",
			json: buildJSONResponse(map[string]interface{}{
				"funds": []interface{}{
					map[string]interface{}{
						"pricingDetails": map[string]interface{}{
							"navPrices": map[string]interface{}{
								"items": []interface{}{
									map[string]interface{}{
										"price":        10.5,
										"asOfDate":     "2026-05-30",
										"currencyCode": "USD",
									},
									map[string]interface{}{
										"price":        10.45,
										"asOfDate":     "2026-05-29",
										"currencyCode": "USD",
									},
								},
							},
						},
					},
				},
			}),
			wantCount: 2,
			check: func(t *testing.T, points []extractor.NavPoint) {
				want0 := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
				if !points[0].Date.Equal(want0) {
					t.Errorf("Date: got %q, want %q", points[0].Date, want0)
				}
				want1 := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)
				if !points[1].Date.Equal(want1) {
					t.Errorf("Date: got %q, want %q", points[1].Date, want1)
				}
				if points[0].Currency != "USD" {
					t.Errorf("Currency: got %q, want %q", points[0].Currency, "USD")
				}
				if points[1].Currency != "USD" {
					t.Errorf("Currency: got %q, want %q", points[1].Currency, "USD")
				}
			},
		},
		{
			name: "empty NAV history",
			json: buildJSONResponse(map[string]interface{}{
				"funds": []interface{}{
					map[string]interface{}{
						"pricingDetails": map[string]interface{}{
							"navPrices": map[string]interface{}{
								"items": []interface{}{},
							},
						},
					},
				},
			}),
			wantCount: 0,
		},
		{
			name: "skips zero price",
			json: buildJSONResponse(map[string]interface{}{
				"funds": []interface{}{
					map[string]interface{}{
						"pricingDetails": map[string]interface{}{
							"navPrices": map[string]interface{}{
								"items": []interface{}{
									map[string]interface{}{
										"price":    0,
										"asOfDate": "2026-05-30",
									},
									map[string]interface{}{
										"price":    10.5,
										"asOfDate": "2026-05-29",
									},
								},
							},
						},
					},
				},
			}),
			wantCount: 1,
		},
		{
			name:    "no funds data",
			json:    buildJSONResponse(map[string]interface{}{"funds": []interface{}{}}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			points, err := ParseNavHistory([]byte(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(points) != tt.wantCount {
				t.Errorf("got %d points, want %d", len(points), tt.wantCount)
			}
			if tt.check != nil && len(points) > 0 {
				tt.check(t, points)
			}
		})
	}
}

// --- extractSlug tests ---

func TestExtractSlug(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantSlug string
		wantErr  bool
	}{
		{"standard URL", "https://www.vanguardinvestor.co.uk/investments/vanguard-ftse-all-world-ucits-etf-usd-distributing", "vanguard-ftse-all-world-ucits-etf-usd-distributing", false},
		{"no www", "https://vanguardinvestor.co.uk/investments/some-fund", "some-fund", false},
		{"deep path", "https://www.vanguardinvestor.co.uk/en-gb/investments/some-fund", "some-fund", false},
		{"trailing slash", "https://www.vanguardinvestor.co.uk/investments/some-fund/", "some-fund", false},
		{"with overview suffix", "https://www.vanguardinvestor.co.uk/investments/vanguard-ftse-all-world-ucits-etf-usd-accumulating/overview", "vanguard-ftse-all-world-ucits-etf-usd-accumulating", false},
		{"with price-performance suffix", "https://www.vanguardinvestor.co.uk/investments/some-fund/price-performance", "some-fund", false},
		{"no path", "https://www.vanguardinvestor.co.uk", "", true},
		{"malformed", "://not-valid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slug, err := extractSlug(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if slug != tt.wantSlug {
				t.Errorf("got %q, want %q", slug, tt.wantSlug)
			}
		})
	}
}

// --- Helpers ---

func buildJSONResponse(data map[string]interface{}) string {
	result, _ := buildGraphQLResponse(data)
	return result
}

func buildGraphQLResponse(data map[string]interface{}) (string, error) {
	root := map[string]interface{}{
		"data": data,
	}
	bytes, err := marshal(root)
	return string(bytes), err
}

func marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
