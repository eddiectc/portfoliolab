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
				Value     string  `json:"value"`
				SortValue interface{} `json:"sortValue"`
			} `json:"header"`
			Column0 struct {
				Value     string  `json:"value"`
				SortValue interface{} `json:"sortValue"`
			} `json:"column_0"`
			Column1 struct {
				Value     string  `json:"value"`
				SortValue float64 `json:"sortValue"`
			} `json:"column_1"`
			Column2 struct {
				Value     string  `json:"value"`
				SortValue float64 `json:"sortValue"`
			} `json:"column_2"`
			Column3 struct {
				Value     string  `json:"value"`
				SortValue interface{} `json:"sortValue"`
			} `json:"column_3"`
			Column4 struct {
				Value     string  `json:"value"`
				SortValue interface{} `json:"sortValue"`
			} `json:"column_4"`
			Column5 struct {
				Value     string  `json:"value"`
				SortValue interface{} `json:"sortValue"`
			} `json:"column_5"`
		} `json:"values"`
	} `json:"tables"`
}

type performanceChartResp struct {
	AsOfDate string `json:"asOfDate"`
	Values   [][]interface{} `json:"values"`
}

// ParseFundInfo extracts basic identity data.
func ParseFundInfo(data string, slug string) (*extractor.FundInfo, error) {
	var resp pdpSettingsResp
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal pdpSettings: %w", err)
	}

	name := resp.InternalId
	if name == "" {
		// Fallback: extract name from slug (e.g. "IE00BMFKG444-nasdaq-100-ucits-etf-1c" -> "Nasdaq 100 Ucits Etf 1c")
		parts := strings.Split(slug, "-")
		if len(parts) > 1 {
			name = strings.Join(parts[1:], " ")
			// Simple title case
			name = strings.Title(strings.ToLower(name))
		}
	}

	return &extractor.FundInfo{
		Symbol: slug,
		Name:   name,
	}, nil
}

// ParseFundProfile extracts metadata and TER.
func ParseFundProfile(data string, aum float64) (*extractor.FundProfile, error) {
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
		TotalNetAssets:     aum,
		AnnualExpenseRatio: ter,
	}, nil
}

// ParseHoldings extracts all security holdings and performs aggregations.
func ParseHoldings(data string) ([]extractor.Holding, []extractor.CountryAllocation, []extractor.SectorWeighting, float64, error) {
	var resp holdingsResp
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		return nil, nil, nil, 0, fmt.Errorf("unmarshal holdings: %w", err)
	}

	if len(resp.Tables) == 0 || len(resp.Tables[0].Values) == 0 {
		return nil, nil, nil, 0, fmt.Errorf("holdings list is empty")
	}

	var holdings []extractor.Holding
	countryMap := make(map[string]float64)
	sectorMap := make(map[string]float64)
	var totalAUM float64

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
		totalAUM += v.Column2.SortValue
	}

	var countries []extractor.CountryAllocation
	for c, w := range countryMap {
		countries = append(countries, extractor.CountryAllocation{Country: c, Percent: w})
	}

	var sectors []extractor.SectorWeighting
	for s, w := range sectorMap {
		sectors = append(sectors, extractor.SectorWeighting{Sector: s, Percent: w})
	}

	return holdings, countries, sectors, totalAUM, nil
}

// ParseNavHistory extracts time series data.
func ParseNavHistory(data string) ([]extractor.NavPoint, error) {
	var resp performanceChartResp
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal performanceChart: %w", err)
	}

	var navs []extractor.NavPoint
	for _, row := range resp.Values {
		if len(row) < 2 {
			continue
		}

		// row[0] is the timestamp (float64)
		tsFloat, ok := row[0].(float64)
		if !ok {
			continue
		}
		timestamp := time.UnixMilli(int64(tsFloat))

		// row[1] is the data array [ [nav, nav, 0], [index, index, 0] ]
		series, ok := row[1].([]interface{})
		if !ok || len(series) < 1 {
			continue
		}

		// The first series is typically the NAV
		navData, ok := series[0].([]interface{})
		if !ok || len(navData) < 1 {
			continue
		}

		navVal, ok := navData[0].(float64)
		if !ok {
			continue
		}

		navs = append(navs, extractor.NavPoint{
			Date: timestamp.Format(time.RFC3339),
			NAV:  decimal.MustNew(int64(navVal*1000000), 6),
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

