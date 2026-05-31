package vanguard

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
	"github.com/govalues/decimal"
)

// --- Phase 1 REST API response types ---

// restFundResponse is the top-level structure from /api/funds/{slug}.
type restFundResponse struct {
	Name                     string        `json:"name"`
	Ticker                   string        `json:"ticker"`
	Sedol                    string        `json:"sedol"`
	PortId                   string        `json:"portId"`
	InceptionDate            string        `json:"inceptionDate"`
	ISIN                     string        `json:"isin"`
	CurrencyCode             string        `json:"currencyCode"`
	OCF                      string        `json:"OCF"`
	Benchmark                string        `json:"benchmark"`
	ManagementType           string        `json:"managementType"`
	AssetClass               string        `json:"assetClass"`
	FundType                 string        `json:"fundType"`
	DistributionStrategyType string        `json:"distributionStrategyType"`
	Region                   string        `json:"region"`
	FundData                 restFundData  `json:"fundData"`
	Siblings                 []restSibling `json:"siblings"`
}

type restFundData struct {
	DistributionHistory interface{} `json:"distributionHistory"`
	AnnualNAVReturns    interface{} `json:"annualNAVReturns"`
}

type restSibling struct {
	PortId string `json:"portId"`
	Ticker string `json:"ticker"`
}

// --- Phase 2 GraphQL response types ---

// graphqlRoot is the top-level GraphQL response wrapper.
type graphqlRoot struct {
	Data json.RawMessage `json:"data"`
}

// holdingsResponse is the HoldingDetailsQuery response.
type holdingsResponse struct {
	BorHoldings struct {
		Holdings struct {
			TotalHoldings int     `json:"totalHoldings"`
			LastItemKey   *string `json:"lastItemKey"`
			Items         []struct {
				EffectiveDate      string   `json:"effectiveDate"`
				MarketValuePercent float64  `json:"marketValuePercentage"`
				IssuerName         string   `json:"issuerName"`
				SecurityLongDesc   string   `json:"securityLongDescription"`
				CouponRate         *float64 `json:"couponRate"`
				SecurityType       string   `json:"securityType"`
				FinalMaturity      *string  `json:"finalMaturity"`
			} `json:"items"`
		} `json:"holdings"`
	} `json:"borHoldings"`
}

// sectorResponse is the getSectorDiversification response.
type sectorResponse struct {
	Funds []struct {
		Profile struct {
			PrimarySectorEquityClassification string `json:"primarySectorEquityClassification"`
		} `json:"profile"`
		SectorDiversification []struct {
			SectorCode   string  `json:"sectorCode"`
			Date         string  `json:"date"`
			SectorName   string  `json:"sectorName"`
			FundPercent  float64 `json:"fundPercent"`
			BenchmarkPct float64 `json:"benchmarkPercent"`
		} `json:"sectorDiversification"`
	} `json:"funds"`
}

// countryResponse is the MarketAllocationGqlQuery response.
type countryResponse struct {
	Funds []struct {
		Profile struct {
			FundFullName             string `json:"fundFullName"`
			PrimaryMarketEquityClass string `json:"primaryMarketEquityClassification"`
			PolarisPdtTypeIndicator  string `json:"polarisPdtTypeIndicator"`
			MarketOfDomicile         string `json:"marketOfDomicile"`
		} `json:"profile"`
		MarketAllocation []struct {
			PortID          string  `json:"portId"`
			Date            string  `json:"date"`
			CountryCode     string  `json:"countryCode"`
			CountryName     string  `json:"countryName"`
			FundMktPct      float64 `json:"fundMktPercent"`
			HoldingStatCode string  `json:"holdingStatCode"`
			BenchmarkMktPct float64 `json:"benchmarkMktPercent"`
			RegionCode      string  `json:"regionCode"`
			RegionName      string  `json:"regionName"`
		} `json:"marketAllocation"`
	} `json:"funds"`
}

// characteristicsResponse is the FundCharacteristicsQuery response.
type characteristicsResponse struct {
	PolarisAnalyticsHistory []struct {
		PortID  string `json:"portId"`
		Monthly struct {
			Analytics struct {
				Fund struct {
					Items []struct {
						Codes struct {
							PERatio    *float64 `json:"PERATIO"`
							PBRatio    *float64 `json:"PBRATIO"`
							MktCapMedn *float64 `json:"MKTCAPMEDN"`
							FRCS5YROE  *float64 `json:"FRC5YRROE"`
							EPSFRC5YR  *float64 `json:"EPSFRC5YR"`
							TRNVRRPTR  *float64 `json:"TRNVRRPTR"`
							AVGCpn     *float64 `json:"AVGCPN"`
							AVGWTDMTY  *float64 `json:"AVGWTDMTY"`
							AVGQLYTFTO *float64 `json:"AVGQLYTFTO"`
							AVGDURADJ  *float64 `json:"AVGDURADJ"`
						} `json:"codes"`
					} `json:"items"`
				} `json:"fund"`
			} `json:"analytics"`
		} `json:"monthly"`
	} `json:"polarisAnalyticsHistory"`
}

// navResponse is the PriceDetailsQuery response.
type navResponse struct {
	Funds []struct {
		PricingDetails struct {
			NavPrices struct {
				Items []struct {
					Price        float64 `json:"price"`
					AsOfDate     string  `json:"asOfDate"`
					CurrencyCode string  `json:"currencyCode"`
				} `json:"items"`
			} `json:"navPrices"`
		} `json:"pricingDetails"`
	} `json:"funds"`
}

// --- GraphQL query strings ---

const holdingsQuery = `query HoldingDetailsQuery($portIds: [String!], $securityTypes: [String!], $lastItemKey: String) {
  borHoldings(portIds: $portIds) {
    holdings(limit: 1500, securityTypes: $securityTypes, lastItemKey: $lastItemKey) {
      totalHoldings
      lastItemKey
      items {
        effectiveDate
        marketValuePercentage
        issuerName
        securityLongDescription
        couponRate
        securityType
        finalMaturity
      }
    }
  }
}`

const sectorQuery = `query getSectorDiversification($portIds: [String!]!) {
  funds(portIds: $portIds) {
    profile { primarySectorEquityClassification }
    sectorDiversification { sectorCode date sectorName fundPercent benchmarkPercent }
  }
}`

const countryQuery = `query MarketAllocationGqlQuery($portIds: [String!]!) {
  funds(portIds: $portIds) {
    profile { fundFullName primaryMarketEquityClassification polarisPdtTypeIndicator marketOfDomicile }
    marketAllocation { portId date countryCode countryName fundMktPercent holdingStatCode benchmarkMktPercent regionCode regionName }
  }
}`

const characteristicsQuery = `query FundCharacteristicsQuery($portIds: [String!]!) {
  polarisAnalyticsHistory(portIds: $portIds) {
    portId
    monthly {
      analytics {
        fund(getLatest: true) {
          items {
            codes {
              PBRATIO
              PERATIO
              AVGCPN
              MKTCAPMEDN
              FRC5YRROE
              EPSFRC5YR
              TRNVRRPTR
              AVGWTDMTY
              AVGQLYTFTO
              AVGDURADJ
            }
          }
        }
      }
    }
  }
}`

const navQuery = `query PriceDetailsQuery($portIds: [String!]!, $startDate: String!, $endDate: String!, $limit: Float) {
  funds(portIds: $portIds) {
    pricingDetails {
      navPrices(startDate: $startDate, endDate: $endDate, limit: $limit) {
        items { price asOfDate currencyCode }
      }
    }
  }
}`

// All security types for holdings query.
var allSecurityTypes = []string{
	"MF.MF", "FI.ABS", "FI.CONV", "FI.CORP", "FI.IP", "FI.LOAN", "FI.MBS",
	"FI.MUNI", "FI.NONUS_GOV", "FI.US_GOV", "MM.AGC", "MM.BACC", "MM.CD",
	"MM.CP", "MM.MCP", "MM.RE", "MM.TBILL", "MM.TD", "MM.TFN", "EQ.DRCPT",
	"EQ.ETF", "EQ.FSH", "EQ.PREF", "EQ.PSH", "EQ.REIT", "EQ.STOCK",
	"EQ.RIGHT", "EQ.WRT",
}

// --- Phase 1: REST API parsers ---

// ParseFundIdentity extracts fund identity from the REST API response.
func ParseFundIdentity(data []byte) (*extractor.FundInfo, string, error) {
	var resp restFundResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, "", fmt.Errorf("unmarshal fund identity: %w", err)
	}

	if resp.Ticker == "" {
		return nil, "", fmt.Errorf("fund identity missing ticker")
	}
	if resp.Name == "" {
		return nil, "", fmt.Errorf("fund identity missing name")
	}
	if resp.PortId == "" {
		return nil, "", fmt.Errorf("fund identity missing portId")
	}

	return &extractor.FundInfo{
		Symbol: resp.Ticker,
		Name:   resp.Name,
	}, resp.PortId, nil
}

// ParseFundProfile extracts fund profile from the REST API response.
func ParseFundProfile(data []byte) (*extractor.FundProfile, error) {
	var resp restFundResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal fund profile: %w", err)
	}

	// Validate: at least one profile field must be populated.
	// A completely empty profile indicates a degraded API response.
	if resp.ManagementType == "" && resp.FundType == "" && resp.OCF == "" &&
		resp.InceptionDate == "" && resp.ISIN == "" && resp.Benchmark == "" &&
		resp.AssetClass == "" && resp.DistributionStrategyType == "" && resp.Region == "" {
		return nil, fmt.Errorf("fund profile: all fields empty")
	}

	profile := &extractor.FundProfile{
		Family:               resp.ManagementType,
		LegalType:            resp.FundType,
		AnnualExpenseRatio:   parseExpenseRatio(resp.OCF),
		InceptionDate:        parseDate(resp.InceptionDate),
		Isin:                 resp.ISIN,
		Benchmark:            resp.Benchmark,
		AssetClassification:  resp.AssetClass,
		DistributionStrategy: resp.DistributionStrategyType,
		MarketRegionFocus:    resp.Region,
	}

	return profile, nil
}

// --- Phase 2: GraphQL parsers ---

// ParseHoldings extracts holdings from the GraphQL response.
// Returns holdings slice and the effectiveDate.
func ParseHoldings(data []byte) ([]extractor.Holding, string, error) {
	var root graphqlRoot
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, "", fmt.Errorf("unmarshal GraphQL root: %w", err)
	}

	var resp holdingsResponse
	if err := json.Unmarshal(root.Data, &resp); err != nil {
		return nil, "", fmt.Errorf("unmarshal holdings: %w", err)
	}

	items := resp.BorHoldings.Holdings.Items
	if len(items) == 0 {
		// Empty holdings — check if effectiveDate is present
		// If no items at all, we can't determine effectiveDate
		return []extractor.Holding{}, "", nil
	}

	effectiveDate := items[0].EffectiveDate
	if effectiveDate == "" {
		return nil, "", fmt.Errorf("holdings missing effectiveDate")
	}

	var holdings []extractor.Holding
	for _, item := range items {
		holding := extractor.Holding{
			Name:          item.IssuerName,
			Percent:       item.MarketValuePercent,
			SecurityType:  item.SecurityType,
			CouponRate:    item.CouponRate,
			FinalMaturity: item.FinalMaturity,
			AsOfDate:      effectiveDate,
		}
		// Also store the long description if issuer name is empty
		if holding.Name == "" {
			holding.Name = item.SecurityLongDesc
		}

		holdings = append(holdings, holding)
	}

	return holdings, effectiveDate, nil
}

// ParseSectorAllocation extracts sector allocation from the GraphQL response.
// Returns sectors slice and the date.
func ParseSectorAllocation(data []byte) ([]extractor.SectorWeighting, string, error) {
	var root graphqlRoot
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, "", fmt.Errorf("unmarshal GraphQL root: %w", err)
	}

	var resp sectorResponse
	if err := json.Unmarshal(root.Data, &resp); err != nil {
		return nil, "", fmt.Errorf("unmarshal sector allocation: %w", err)
	}

	if len(resp.Funds) == 0 {
		return nil, "", fmt.Errorf("sector allocation: no funds data")
	}

	sectors := resp.Funds[0].SectorDiversification
	if len(sectors) == 0 {
		return []extractor.SectorWeighting{}, "", nil
	}

	date := sectors[0].Date

	var result []extractor.SectorWeighting
	for _, s := range sectors {
		result = append(result, extractor.SectorWeighting{
			Sector:  s.SectorName,
			Percent: s.FundPercent,
			Date:    s.Date,
		})
	}

	return result, date, nil
}

// ParseCountryAllocation extracts country allocation from the GraphQL response.
// Returns countries slice and the date.
func ParseCountryAllocation(data []byte) ([]extractor.CountryAllocation, string, error) {
	var root graphqlRoot
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, "", fmt.Errorf("unmarshal GraphQL root: %w", err)
	}

	var resp countryResponse
	if err := json.Unmarshal(root.Data, &resp); err != nil {
		return nil, "", fmt.Errorf("unmarshal country allocation: %w", err)
	}

	if len(resp.Funds) == 0 {
		return nil, "", fmt.Errorf("country allocation: no funds data")
	}

	allocations := resp.Funds[0].MarketAllocation
	if len(allocations) == 0 {
		return []extractor.CountryAllocation{}, "", nil
	}

	date := allocations[0].Date

	var result []extractor.CountryAllocation
	for _, a := range allocations {
		result = append(result, extractor.CountryAllocation{
			Country:    a.CountryName,
			Percent:    a.FundMktPct,
			RegionName: a.RegionName,
			RegionCode: a.RegionCode,
			Date:       a.Date,
		})
	}

	return result, date, nil
}

// ParseFundCharacteristics extracts fund characteristics from the GraphQL response.
func ParseFundCharacteristics(data []byte) (*extractor.FundCharacteristics, error) {
	var root graphqlRoot
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("unmarshal GraphQL root: %w", err)
	}

	var resp characteristicsResponse
	if err := json.Unmarshal(root.Data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal characteristics: %w", err)
	}

	if len(resp.PolarisAnalyticsHistory) == 0 {
		return nil, fmt.Errorf("characteristics: no analytics data")
	}

	analytics := resp.PolarisAnalyticsHistory[0].Monthly.Analytics
	if len(analytics.Fund.Items) == 0 {
		return nil, fmt.Errorf("characteristics: no fund items")
	}

	codes := analytics.Fund.Items[0].Codes
	characteristics := &extractor.FundCharacteristics{}

	if codes.PERatio != nil {
		characteristics.PriceToEarnings = *codes.PERatio
		characteristics.FieldsPresent |= extractor.CharacteristicPriceToEarnings
	}
	if codes.PBRatio != nil {
		characteristics.PriceToBook = *codes.PBRatio
		characteristics.FieldsPresent |= extractor.CharacteristicPriceToBook
	}
	if codes.MktCapMedn != nil {
		characteristics.MedianMarketCap = *codes.MktCapMedn
		characteristics.FieldsPresent |= extractor.CharacteristicMedianMarketCap
	}
	if codes.FRCS5YROE != nil {
		characteristics.ForwardROE = *codes.FRCS5YROE
		characteristics.FieldsPresent |= extractor.CharacteristicForwardROE
	}
	if codes.EPSFRC5YR != nil {
		characteristics.ForwardEPSGrowth = *codes.EPSFRC5YR
		characteristics.FieldsPresent |= extractor.CharacteristicForwardEPSGrowth
	}
	if codes.TRNVRRPTR != nil {
		characteristics.RevenueRatio = *codes.TRNVRRPTR
		characteristics.FieldsPresent |= extractor.CharacteristicRevenueRatio
	}
	if codes.AVGCpn != nil {
		characteristics.AverageCoupon = *codes.AVGCpn
		characteristics.FieldsPresent |= extractor.CharacteristicAverageCoupon
	}
	if codes.AVGWTDMTY != nil {
		characteristics.AverageMaturity = *codes.AVGWTDMTY
		characteristics.FieldsPresent |= extractor.CharacteristicAverageMaturity
	}
	if codes.AVGQLYTFTO != nil {
		characteristics.AverageQuality = *codes.AVGQLYTFTO
		characteristics.FieldsPresent |= extractor.CharacteristicAverageQuality
	}
	if codes.AVGDURADJ != nil {
		characteristics.AverageDuration = *codes.AVGDURADJ
		characteristics.FieldsPresent |= extractor.CharacteristicAverageDuration
	}

	return characteristics, nil
}

// ParseNavHistory extracts NAV price history from the GraphQL response.
func ParseNavHistory(data []byte) ([]extractor.NavPoint, error) {
	var root graphqlRoot
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("unmarshal GraphQL root: %w", err)
	}

	var resp navResponse
	if err := json.Unmarshal(root.Data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal NAV history: %w", err)
	}

	if len(resp.Funds) == 0 {
		return nil, fmt.Errorf("NAV history: no funds data")
	}

	items := resp.Funds[0].PricingDetails.NavPrices.Items
	if len(items) == 0 {
		return []extractor.NavPoint{}, nil
	}

	var points []extractor.NavPoint
	for _, item := range items {
		if item.Price == 0 || item.AsOfDate == "" {
			continue
		}
		points = append(points, extractor.NavPoint{
			Date: item.AsOfDate,
			NAV:  decimal.MustParse(fmt.Sprintf("%.4f", item.Price)),
		})
	}

	return points, nil
}

// --- Helpers ---

// parseExpenseRatio parses a percentage string like "0.40%" to a fraction (0.004).
func parseExpenseRatio(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return val / 100
}

// parseDate parses an ISO date string or returns zero time.
func parseDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t
}
