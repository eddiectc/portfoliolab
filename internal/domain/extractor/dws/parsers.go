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

type pdpMetaTagsResp struct {
	PdpResult struct {
		PageFrame struct {
			ProductHeader struct {
				Texts struct {
					Title string `json:"title"`
				} `json:"texts"`
				TableValues []struct {
					Key   string      `json:"key"`
					Value interface{} `json:"value"`
				} `json:"tableValues"`
			} `json:"productHeader"`
		} `json:"pageFrame"`
	} `json:"pdpResult"`
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
func ParseFundInfo(settingsData, metaData string, slug string) (*extractor.FundInfo, error) {
	var sResp pdpSettingsResp
	if err := json.Unmarshal([]byte(settingsData), &sResp); err != nil {
		return nil, fmt.Errorf("unmarshal pdpSettings: %w", err)
	}

	var mResp pdpMetaTagsResp
	if err := json.Unmarshal([]byte(metaData), &mResp); err != nil {
		return nil, fmt.Errorf("unmarshal pdpMetaTags: %w", err)
	}

	name := mResp.PdpResult.PageFrame.ProductHeader.Texts.Title
	if name == "" {
		name = sResp.InternalId
	}

	return &extractor.FundInfo{
		Symbol: slug,
		Name:   name,
	}, nil
}

// ParseFundProfile extracts metadata and TER.
func ParseFundProfile(settingsData, metaData string) (*extractor.FundProfile, error) {
	var sResp pdpSettingsResp
	if err := json.Unmarshal([]byte(settingsData), &sResp); err != nil {
		return nil, fmt.Errorf("unmarshal pdpSettings: %w", err)
	}

	var mResp pdpMetaTagsResp
	if err := json.Unmarshal([]byte(metaData), &mResp); err != nil {
		return nil, fmt.Errorf("unmarshal pdpMetaTags: %w", err)
	}

	var aum float64
	var ter float64

	for _, tv := range mResp.PdpResult.PageFrame.ProductHeader.TableValues {
		valStr, ok := tv.Value.(string)
		if !ok {
			continue
		}

		switch tv.Key {
		case "Total AUM of fund":
			aum = parseAUMValue(valStr)
		case "All-in-fee (TER)":
			if v, err := parsePercent(valStr); err == nil {
				ter = v
			}
		}
	}

	// Fallback for TER from settings if not found in meta tags
	if ter == 0 && sResp.CostsAndFees.TotalOngoingCosts != "" {
		if v, err := parsePercent(sResp.CostsAndFees.TotalOngoingCosts); err == nil {
			ter = v
		}
	}

	return &extractor.FundProfile{
		Family:             sResp.FundFamily,
		LegalType:          sResp.LegalType,
		TotalNetAssets:     aum,
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

func parseAUMValue(s string) float64 {
	s = strings.ToUpper(strings.TrimSpace(s))
	
	// Extract numeric part and multiplier
	// Example: "1.77 B GBP" -> 1.77, "B"
	var val float64
	var multiplier float64 = 1

	// Simple parser for B (Billion), M (Million), K (Thousand)
	if strings.Contains(s, " B") {
		multiplier = 1_000_000_000
	} else if strings.Contains(s, " M") {
		multiplier = 1_000_000
	} else if strings.Contains(s, " K") {
		multiplier = 1_000
	}

	// Find the first part that looks like a number
	fields := strings.Fields(s)
	if len(fields) > 0 {
		f, err := strconv.ParseFloat(fields[0], 64)
		if err == nil {
			val = f
		}
	}

	return val * multiplier
}

func parsePercent(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return val / 100, nil
}

