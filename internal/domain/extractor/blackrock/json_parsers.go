package blackrock

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/extractor"
)

// --- Product Data JSON API (2026-08 site redesign) ---
//
// iShares product pages (2026-08 "onedes" build and later) load fund data
// from a Varnish-backed product data JSON API instead of embedding it in the
// page HTML. The API base URL and request parameters are embedded in the
// page as the "apiHost" / "productDataParams" config; the portfolio ID comes
// from the product URL.
//
// Response envelope:
//
//	{
//	  "fundName": "...",
//	  "componentsByNameMap": {
//	    "<component>": {
//	      "containersByNameMap": {
//	        "<container>": {
//	          "dataPointsByNameMap": { "<key>": { "name": ..., "formattedValue": ... } }
//	        }
//	      }
//	    }
//	  }
//	}
//
// Scalar data points have a string formattedValue (possibly null or an
// object with display/value). Column data points (holdings) have an array of
// strings, one per holding row.

// ProductDataConfig is the product data API configuration extracted from the page.
type ProductDataConfig struct {
	// APIHost is the product data API base URL.
	APIHost string
	// AppSubType, e.g. "ISHARES".
	AppSubType string
	// AppType, e.g. "PRODUCT_PAGE".
	AppType string
	// Locale with region, e.g. "en_GB" (region-less locales are rejected by the API).
	Locale string
	// TargetSite, e.g. "ishares-uk".
	TargetSite string
	// UserType, e.g. "individual".
	UserType string
}

// embeddedParams mirrors the "productDataParams" object embedded in the page.
type embeddedParams struct {
	AppSubType string `json:"appSubType"`
	AppType    string `json:"appType"`
	Locale     string `json:"locale"`
	TargetSite string `json:"targetSite"`
	UserType   string `json:"userType"`
}

// ParseProductDataConfig extracts the product data API configuration from the
// product page HTML. The config is embedded HTML-entity-escaped (&quot;), so
// the page is unescaped before extraction.
func ParseProductDataConfig(page string) (*ProductDataConfig, error) {
	unescaped := html.UnescapeString(page)

	apiHostRe := regexp.MustCompile(`"apiHost":"([^"]+)"`)
	m := apiHostRe.FindStringSubmatch(unescaped)
	if m == nil {
		return nil, fmt.Errorf("apiHost not found in page config")
	}
	apiHost := strings.TrimRight(m[1], "?")

	paramsRe := regexp.MustCompile(`"productDataParams":\s*(\{[^}]*\})`)
	m = paramsRe.FindStringSubmatch(unescaped)
	if m == nil {
		return nil, fmt.Errorf("productDataParams not found in page config")
	}
	var p embeddedParams
	if err := json.Unmarshal([]byte(m[1]), &p); err != nil {
		return nil, fmt.Errorf("parse productDataParams: %w", err)
	}
	if p.AppSubType == "" || p.Locale == "" || p.TargetSite == "" || p.UserType == "" {
		return nil, fmt.Errorf("incomplete productDataParams in page config")
	}

	return &ProductDataConfig{
		APIHost:    apiHost,
		AppSubType: p.AppSubType,
		AppType:    p.AppType,
		Locale:     p.Locale,
		TargetSite: p.TargetSite,
		UserType:   p.UserType,
	}, nil
}

// ParsePortfolioID extracts the numeric portfolio ID from the product URL.
// Pattern: /{market}/{userType}/{locale}/products/{portfolioId}/{slug}
func ParsePortfolioID(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse product URL: %w", err)
	}
	re := regexp.MustCompile(`/products/(\d+)/`)
	m := re.FindStringSubmatch(u.Path)
	if m == nil {
		return "", fmt.Errorf("portfolio ID not found in URL path %q", u.Path)
	}
	return m[1], nil
}

// BuildProductDataURL constructs a product data API URL for the given
// component (e.g. "keyFundFacts", "holdings") using the page config and the
// portfolio ID.
func BuildProductDataURL(cfg *ProductDataConfig, portfolioID, component string) (string, error) {
	if cfg == nil || cfg.APIHost == "" {
		return "", fmt.Errorf("product data API host not configured")
	}
	u, err := url.Parse(cfg.APIHost)
	if err != nil {
		return "", fmt.Errorf("parse API host: %w", err)
	}
	q := u.Query()
	q.Set("appSubType", cfg.AppSubType)
	q.Set("appType", cfg.AppType)
	q.Set("component", component)
	q.Set("locale", cfg.Locale)
	q.Set("portfolioId", portfolioID)
	q.Set("targetSite", cfg.TargetSite)
	q.Set("userType", cfg.UserType)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// productDataResponse mirrors the product data API response envelope.
type productDataResponse struct {
	FundName            string               `json:"fundName"`
	ComponentsByNameMap map[string]component `json:"componentsByNameMap"`
}

// component mirrors one named component in the response.
type component struct {
	ContainersByNameMap map[string]jsonDataContainer `json:"containersByNameMap"`
}

// jsonDataContainer mirrors one named container within a component.
type jsonDataContainer struct {
	DataPointsByNameMap map[string]jsonDataPoint `json:"dataPointsByNameMap"`
}

// jsonDataPoint mirrors a single data point. FormattedValue is a string for
// scalar points, an array of strings for per-row column data, and null or an
// object for some component types — it is kept raw and decoded per use case.
type jsonDataPoint struct {
	Name           string          `json:"name"`
	FormattedValue json.RawMessage `json:"formattedValue"`
}

// dataPointString returns a scalar data point's formatted value as a string.
// Handles null, plain strings, and object-wrapped values with a display or
// value field.
func dataPointString(raw json.RawMessage) string {
	if raw == nil || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var o struct {
		Display string `json:"display"`
		Value   string `json:"value"`
	}
	if err := json.Unmarshal(raw, &o); err == nil {
		if o.Display != "" {
			return o.Display
		}
		return o.Value
	}
	return ""
}

// dataPointStrings returns a column data point's formatted value as a
// row-aligned string slice.
func dataPointStrings(raw json.RawMessage) []string {
	if raw == nil || string(raw) == "null" {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	if s := dataPointString(raw); s != "" {
		return []string{s}
	}
	return nil
}

// componentDataPoints unmarshals a product data API response and returns the
// data points for the named component, preferring the named container when
// present, otherwise the first container that holds data points.
func componentDataPoints(body, component, preferredContainer string) (map[string]jsonDataPoint, error) {
	var resp productDataResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return nil, fmt.Errorf("parse product data response: %w", err)
	}
	comp, ok := resp.ComponentsByNameMap[component]
	if !ok {
		return nil, fmt.Errorf("%s component not found in response", component)
	}
	containers := comp.ContainersByNameMap
	if len(containers) == 0 {
		return nil, fmt.Errorf("%s component has no containers", component)
	}
	if c, ok := containers[preferredContainer]; ok && len(c.DataPointsByNameMap) > 0 {
		return c.DataPointsByNameMap, nil
	}
	for _, c := range containers {
		if len(c.DataPointsByNameMap) > 0 {
			return c.DataPointsByNameMap, nil
		}
	}
	return nil, fmt.Errorf("%s component has no container with data points", component)
}

// jsonStr returns the trimmed scalar value of a data point ("" when absent).
func jsonStr(dpm map[string]jsonDataPoint, key string) string {
	if dp, ok := dpm[key]; ok {
		return strings.TrimSpace(dataPointString(dp.FormattedValue))
	}
	return ""
}

// ParseFundProfileFromJSON extracts the fund profile from the keyFundFacts
// component of a product data API response.
//
// Note: the keyFundFacts payload does not include Total Expense Ratio —
// callers should fill it from the page HTML when needed.
func ParseFundProfileFromJSON(body string) (*extractor.FundProfile, error) {
	dpm, err := componentDataPoints(body, "keyFundFacts", "default")
	if err != nil {
		return nil, err
	}

	profile := &extractor.FundProfile{}
	if s := jsonStr(dpm, "isin"); s != "" {
		profile.Isin = s
	}
	if s := jsonStr(dpm, "totalNetAssets"); s != "" {
		profile.TotalNetAssets = extractAUM(s)
	}
	if s := jsonStr(dpm, "inceptionDate"); s != "" {
		if t, err := parseIShareDate(s); err == nil {
			profile.InceptionDate = t
		}
	}
	if s := jsonStr(dpm, "assetClass"); s != "" {
		profile.AssetClassification = s
	}
	if s := jsonStr(dpm, "sfdr"); s != "" {
		profile.SFDRClassification = s
	}
	if s := jsonStr(dpm, "useOfProfitsCode"); s != "" {
		profile.DistributionStrategy = s
	}
	if s := jsonStr(dpm, "domicile"); s != "" {
		profile.Domicile = s
	}
	if s := jsonStr(dpm, "rebalanceFrequency"); s != "" {
		profile.RebalanceFrequency = s
	}
	if s := jsonStr(dpm, "fundmanager"); s != "" {
		profile.FundManager = s
	}
	if s := jsonStr(dpm, "fundCustodian"); s != "" {
		profile.Custodian = s
	}
	if s := jsonStr(dpm, "bbeqtick"); s != "" {
		profile.BenchmarkTicker = s
	}
	if s := jsonStr(dpm, "indexSeriesName"); s != "" {
		profile.Benchmark = s
	}
	if s := jsonStr(dpm, "productStructure"); s != "" {
		profile.ProductStructure = s
	}
	if s := jsonStr(dpm, "fundMethodologyTypeCode"); s != "" {
		profile.Methodology = s
	}
	if s := jsonStr(dpm, "issuingCompany"); s != "" {
		profile.IssuingCompany = s
	}
	if s := jsonStr(dpm, "baseCurrencyCode"); s != "" {
		profile.BaseCurrency = s
	}

	// Validate: at least some key fields should be present
	if profile.Isin == "" && profile.TotalNetAssets == 0 {
		return nil, fmt.Errorf("fund profile data missing (no ISIN or AUM found)")
	}

	return profile, nil
}

// ParseHoldingsFromJSON extracts holdings from the holdings component of a
// product data API response. The data is column-oriented: each field is an
// array of strings aligned by row index. Cash rows keep a currency ticker
// with ISIN "-" and are kept.
//
// Returns the holdings list and the snapshot as-of date from the "asOfDate"
// data point (zero time when absent).
func ParseHoldingsFromJSON(body string) ([]extractor.Holding, time.Time, error) {
	dpm, err := componentDataPoints(body, "holdings", "all")
	if err != nil {
		return nil, time.Time{}, err
	}

	getCol := func(key string) []string {
		if dp, ok := dpm[key]; ok {
			return dataPointStrings(dp.FormattedValue)
		}
		return nil
	}

	cols := map[string][]string{
		"ticker":             getCol("ticker"),
		"issueName":          getCol("issueName"),
		"holdingPercent":     getCol("holdingPercent"),
		"sectorName":         getCol("sectorName"),
		"assetClass":         getCol("assetClass"),
		"marketValue":        getCol("marketValue"),
		"notionalValue":      getCol("notionalValue"),
		"unitsHeld":          getCol("unitsHeld"),
		"unitPrice":          getCol("unitPrice"),
		"countryOfRisk":      getCol("countryOfRisk"),
		"exchange":           getCol("exchange"),
		"marketCurrencyCode": getCol("marketCurrencyCode"),
		"isin":               getCol("isin"),
	}

	n := len(cols["ticker"])
	if n == 0 {
		return nil, holdingsAsOfDate(dpm), nil
	}

	cell := func(key string, i int) string {
		c := cols[key]
		if i < len(c) {
			return strings.TrimSpace(c[i])
		}
		return ""
	}

	var holdings []extractor.Holding
	for i := 0; i < n; i++ {
		symbol := cell("ticker", i)
		name := cell("issueName", i)
		if symbol == "" && name == "" {
			continue // skip blank rows
		}
		isin := cell("isin", i)
		if isin == "" {
			isin = "-"
		}
		holdings = append(holdings, extractor.Holding{
			Symbol:         symbol,
			Name:           name,
			Percent:        parseFloatValue(cell("holdingPercent", i)),
			Sector:         cell("sectorName", i),
			AssetClass:     cell("assetClass", i),
			MarketValue:    parseFloatValue(cell("marketValue", i)),
			NotionalValue:  parseFloatValue(cell("notionalValue", i)),
			Shares:         parseFloatValue(cell("unitsHeld", i)),
			Price:          parseFloatValue(cell("unitPrice", i)),
			ISIN:           isin,
			Location:       cell("countryOfRisk", i),
			Exchange:       cell("exchange", i),
			MarketCurrency: cell("marketCurrencyCode", i),
		})
	}

	return holdings, holdingsAsOfDate(dpm), nil
}

// holdingsAsOfDate extracts the holdings snapshot date from the "asOfDate"
// data point of the holdings component.
func holdingsAsOfDate(dpm map[string]jsonDataPoint) time.Time {
	if s := jsonStr(dpm, "asOfDate"); s != "" {
		if t, err := parseIShareDate(s); err == nil {
			return t
		}
	}
	return time.Time{}
}
