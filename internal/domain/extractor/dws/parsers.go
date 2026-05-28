package dws

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/govalues/decimal"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

// JSON structs for DWS API responses

type pdpSettingsResp struct {
	ProductType    string `json:"productType"`
	InternalId     string `json:"internalId"`
	FundFamily     string `json:"fundFamily"`
	LegalType      string `json:"legalType"`
	CostsAndFees   struct {
		TotalOngoingCosts string `json:"totalOngoingCosts"`
	} `json:"costsAndFees"`
}

type holdingsResp struct {
	Tables []struct {
		Values []struct {
			Header struct {
				Value string `json:"value"`
			} `json:"header"`
			Column0 struct {
				Value string `json:"value"`
			} `json:"column_0"`
			Column1 struct {
				Value string `json:"value"`
			} `json:"column_1"`
			Column3 struct {
				Value string `json:"value"`
			} `json:"column_3"`
			Column4 struct {
				Value string `json:"value"`
			} `json:"column_4"`
		} `json:"values"`
	} `json:"tables"`
}

type performanceChartResp struct {
	AsOfDate string `json:"asOfDate"`
	ChartData []struct {
		Timestamp string          `json:"timestamp"`
		Value     decimal.Decimal `json:"value"`
	} `json:"chartData"`
}

// ParseFundInfo extracts basic identity data.
func ParseFundInfo(data string, symbol string) (*extractor.FundInfo, error) {
	var resp pdpSettingsResp
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal pdpSettings: %w", err)
	}

	return &extractor.FundInfo{
		Symbol: symbol,
		Name:   resp.InternalId, // Using internal ID as name if dedicated name field is missing
	}, nil
}

// ParseFundProfile extracts metadata and TER.
func ParseFundProfile(data string) (*extractor.FundProfile, error) {
	var resp pdpSettingsResp
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal pdpSettings: %w", err)
	}

	var ter float64
	if resp.CostsAndFees.TotalOngoingCosts != "" {
		val, err := parsePercent(resp.CostsAndFees.TotalOngoingCosts)
		if err != nil {
			// Log error but don't fail the entire extraction for optional TER
			fmt.Printf("warning: failed to parse TER %q: %v\n", resp.CostsAndFees.TotalOngoingCosts, err)
		} else {
			ter = val
		}
	}

	return &extractor.FundProfile{
		Family:             resp.FundFamily,
		LegalType:          resp.LegalType,
		AnnualExpenseRatio: ter,
	}, nil
}

// ParseHoldings extracts all security holdings and performs aggregations.
func ParseHoldings(data string) ([]extractor.Holding, []extractor.CountryAllocation, []extractor.SectorWeighting, error) {
	var resp holdingsResp
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		return nil, nil, nil, fmt.Errorf("unmarshal holdings: %w", err)
	}

	if len(resp.Tables) == 0 || len(resp.Tables[0].Values) == 0 {
		return nil, nil, nil, fmt.Errorf("holdings list is empty")
	}

	var holdings []extractor.Holding
	countryMap := make(map[string]float64)
	sectorMap := make(map[string]float64)

	for _, v := range resp.Tables[0].Values {
		weight, err := parsePercent(v.Column1.Value)
		if err != nil {
			// Log error but skip this holding if weight is unparseable
			fmt.Printf("warning: failed to parse weight %q for %s: %v\n", v.Column1.Value, v.Header.Value, err)
			continue
		}

		holdings = append(holdings, extractor.Holding{
			Symbol:  v.Header.Value,
			Name:    v.Column0.Value,
			Percent: weight * 100, // Convert back to percentage for domain model (e.g. 8.315)
		})

		countryMap[v.Column3.Value] += weight * 100
		sectorMap[v.Column4.Value] += weight * 100
	}

	var countries []extractor.CountryAllocation
	for c, w := range countryMap {
		countries = append(countries, extractor.CountryAllocation{Country: c, Percent: w})
	}

	var sectors []extractor.SectorWeighting
	for s, w := range sectorMap {
		sectors = append(sectors, extractor.SectorWeighting{Sector: s, Percent: w})
	}

	return holdings, countries, sectors, nil
}

// ParseNavHistory extracts time series data.
func ParseNavHistory(data string) ([]extractor.NavPoint, error) {
	var resp performanceChartResp
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal performanceChart: %w", err)
	}

	var navs []extractor.NavPoint
	for _, p := range resp.ChartData {
		navs = append(navs, extractor.NavPoint{
			Date: p.Timestamp,
			NAV:  p.Value,
		})
	}

	return navs, nil
}

// ParseAsOfDate extracts the reference date.
func ParseAsOfDate(data string) (time.Time, error) {
	var resp performanceChartResp
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		return time.Time{}, fmt.Errorf("unmarshal performanceChart: %w", err)
	}

	// Try RFC3339 first (standard API), then fall back to common date formats
	t, err := time.Parse(time.RFC3339, resp.AsOfDate)
	if err != nil {
		// Fallback to common date formats used in DWS API
		formats := []string{
			"02/01/2006",
			"02-01-2006",
			"2006-01-02",
		}
		var parsed bool
		for _, f := range formats {
			if t, err = time.Parse(f, resp.AsOfDate); err == nil {
				parsed = true
				break
			}
		}
		if !parsed {
			return time.Time{}, fmt.Errorf("parse asOfDate %q: %w", resp.AsOfDate, err)
		}
	}

	return t, nil
}

// Helpers

func parsePercent(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return val / 100, nil
}

