package dimensional

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

// FundDetailResponse represents the structure of the /funddetail API response.
type FundDetailResponse struct {
	Data struct {
		AsOfDate struct {
			Value string `json:"value"`
		} `json:"asOfDate"`
		LensGroups []struct {
			Data struct {
				Slug   string `json:"slug"`
				Lenses []struct {
					Data struct {
						Name   string `json:"name"`
						Slug   string `json:"slug"`
						Blends []struct {
							Data struct {
								FundFacts struct {
									MarketingName string   `json:"marketingName"`
									Benchmarks    []string `json:"benchmarks"`
									IsEtf         bool     `json:"isEtf"`
									IsDfaUcitsEtf bool     `json:"isDfaUcitsEtf"`
									FundAum       struct {
										Aum struct {
											Value float64 `json:"value"`
										} `json:"aum"`
									} `json:"fundAum"`
									InceptionDate struct {
										Value string `json:"value"`
									} `json:"inceptionDate"`
								} `json:"fundFacts"`
								FundPrices struct {
									Prices []struct {
										Nav struct {
											Value float64 `json:"value"`
										} `json:"nav"`
									} `json:"prices"`
								} `json:"fundPrices"`
								Fees struct {
									Fees []struct {
										Slug  string `json:"slug"`
										Value struct {
											Value float64 `json:"value"`
										} `json:"value"`
									} `json:"fees"`
								} `json:"fees"`
								Allocations []struct {
									Name   string `json:"name"`
									Weight struct {
										Value float64 `json:"value"`
									} `json:"weight"`
									SubCategories []struct {
										Name   string `json:"name"`
										Weight struct {
											Value float64 `json:"value"`
										} `json:"weight"`
									} `json:"subCategories"`
								} `json:"allocations"`
								FullHoldingsCsvUrl string `json:"fullHoldingsCsvUrl"`
							} `json:"data"`
						} `json:"blends"`
					} `json:"data"`
				} `json:"lenses"`
			} `json:"data"`
		} `json:"lensGroups"`
	} `json:"data"`
}

// ParseFundDetail extracts structured data from the fund detail JSON response.
func ParseFundDetail(jsonContent string) (*extractor.FundInfo, *extractor.FundProfile, []extractor.SectorWeighting, []extractor.CountryAllocation, float64, string, error) {
	var data FundDetailResponse
	if err := json.Unmarshal([]byte(jsonContent), &data); err != nil {
		return nil, nil, nil, nil, 0, "", fmt.Errorf("decode fund detail: %w", err)
	}

	var info *extractor.FundInfo
	var profile *extractor.FundProfile
	var sectors []extractor.SectorWeighting
	var countries []extractor.CountryAllocation
	var nav float64
	var csvURL string

	for _, group := range data.Data.LensGroups {
		for _, lens := range group.Data.Lenses {
			slug := lens.Data.Slug
			if len(lens.Data.Blends) == 0 {
				continue
			}
			blendData := lens.Data.Blends[0].Data

			switch slug {
			case "fundFacts":
				ff := blendData.FundFacts
				info = &extractor.FundInfo{
					Name: ff.MarketingName,
				}
				legalType := "Mutual Fund"
				if ff.IsEtf || ff.IsDfaUcitsEtf {
					legalType = "ETF"
				}
				profile = &extractor.FundProfile{
					TotalNetAssets: ff.FundAum.Aum.Value,
					Family:         "Dimensional Fund Advisors",
					LegalType:      legalType,
				}
				if t, err := time.Parse("2006-01-02", ff.InceptionDate.Value); err == nil {
					profile.InceptionDate = t
				}
			case "fundPrices":
				if len(blendData.FundPrices.Prices) > 0 {
					nav = blendData.FundPrices.Prices[0].Nav.Value
				}
			case "fees":
				for _, f := range blendData.Fees.Fees {
					if f.Slug == "net-exp-ratio" {
						if profile != nil {
							profile.AnnualExpenseRatio = f.Value.Value
						}
					}
				}
			case "charsEquityAllocationByGics":
				for _, a := range blendData.Allocations {
					sectors = append(sectors, extractor.SectorWeighting{
						Sector:  a.Name,
						Percent: a.Weight.Value * 100,
					})
				}
			case "charsEquityAllocationByCountryByRegionDevelopedEmerging":
				for _, a := range blendData.Allocations {
					for _, sc := range a.SubCategories {
						countries = append(countries, extractor.CountryAllocation{
							Country: sc.Name,
							Percent: sc.Weight.Value * 100,
						})
					}
				}
			case "charsEtfTopHoldingsDaily":
				csvURL = blendData.FullHoldingsCsvUrl
			}
		}
	}

	if info == nil {
		return nil, nil, nil, nil, 0, "", fmt.Errorf("required fund facts not found in response")
	}

	return info, profile, sectors, countries, nav, csvURL, nil
}

// ParseHoldingsCSV extracts holdings from the Dimensional CSV format.
func ParseHoldingsCSV(csvContent string) ([]extractor.Holding, error) {
	reader := csv.NewReader(strings.NewReader(csvContent))
	reader.TrimLeadingSpace = true

	if _, err := reader.Read(); err != nil {
		return nil, fmt.Errorf("read holdings header: %w", err)
	}

	var holdings []extractor.Holding
	for {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if _, ok := err.(*csv.ParseError); ok {
				slog.Warn("skipping malformed CSV row in holdings", "error", err)
				continue
			}
			return nil, fmt.Errorf("read holdings record: %w", err)
		}

		if len(record) < 6 {
			slog.Warn("skipping short CSV row in holdings", "fields", len(record))
			continue
		}

		ticker := strings.TrimSpace(record[2])
		name := strings.TrimSpace(record[3])
		weightStr := strings.TrimSpace(record[4])

		if isCashPosition(name) {
			continue
		}

		weight, err := parseNumber(weightStr)
		if err != nil {
			slog.Warn("skipping CSV row with unparseable weight", "name", name, "weight", weightStr)
			continue
		}

		weight *= 100

		holdings = append(holdings, extractor.Holding{
			Symbol:  ticker,
			Name:    name,
			Percent: weight,
		})
	}

	sort.Slice(holdings, func(i, j int) bool {
		return holdings[i].Percent > holdings[j].Percent
	})

	return holdings, nil
}

func isCashPosition(name string) bool {
	name = strings.ToUpper(name)
	cashKeywords := []string{
		"CASH", "CASH W-O", "EURO INCOME", "STERLING POUND", "US DOLLAR",
		"JAPANESE YEN", "AUSTRALIAN DOLLAR", "CANADIAN DOLLAR", "SWISS FRANC",
		"SWEDISH KRONA", "NORWEGIAN KRONE", "DANISH KRONE", "BRAZIL REAL",
		"KOREAN WON", "SINGAPORE DOLLAR", "HONG KONG DOLLAR", "INDONESIAN RUPIAH",
		"THAILAND BAHT", "MALAYSIAN RINGIT", "POLISH ZLOTY", "MEXICAN PESO",
		"PHILIPPINE PESO", "TURKISH LIRA", "SOUTH AFRICAN RAND",
		"CHINESE RENMINBI", "CHINESE RENIMBI", "CZECHOSLOVAKIAN",
		"CGT ADJ", "CGT", "CASH & CASH EQUIVALENTS",
	}

	for _, keyword := range cashKeywords {
		if strings.Contains(name, keyword) {
			return true
		}
	}
	return false
}

func parseNumber(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "%", "")
	s = strings.ReplaceAll(s, "€", "")
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, ",", "")

	return strconv.ParseFloat(s, 64)
}
