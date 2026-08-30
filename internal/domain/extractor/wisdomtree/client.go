package wisdomtree

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Danny-Dasilva/CycleTLS/cycletls"
)

// ja3Chrome129 is the JA3 TLS fingerprint for Chrome 129 on Windows.
// Matches the configuration confirmed working against WisdomTree's Cloudflare (2026-05-26).
const ja3Chrome129 = "771,4865-4866-4867-49195-49199-49196-49200-52393-52392-49171-49172-156-157-47-53,0-23-65281-10-11-35-16-5-13-18-51-45-43-27-21,29-23-24,0"

// userAgentChrome129 is the User-Agent string matching the JA3 fingerprint.
const userAgentChrome129 = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36"

// fetchFunc is the function signature for fetching page content.
// Allows injection of mock fetchers in tests.
type fetchFunc func(url string) (string, error)

// apiBaseURL is the base URL of WisdomTree's undocumented JSON API
// (see features/f021_wisdomtree-scraper/RESEARCH.md §5).
const apiBaseURL = "https://www.wisdomtree.com/api"

// Client fetches WisdomTree pages with browser-grade TLS fingerprinting to bypass Cloudflare.
type Client struct {
	cycleTLS cycletls.CycleTLS
	mu       sync.Mutex
	lastReq  time.Time
	minDelay time.Duration
	timeout  int
	fetch    fetchFunc // overridden in tests
	baseURL  string
}

// NewClient creates a new WisdomTree HTTP client with CycleTLS and rate limiting.
func NewClient() *Client {
	return &Client{
		cycleTLS: cycletls.Init(),
		minDelay: 1 * time.Second,
		timeout:  30,
		baseURL:  apiBaseURL,
	}
}

// Fetch retrieves the HTML content of a WisdomTree page.
// Uses CycleTLS with browser-grade TLS fingerprinting to bypass Cloudflare bot protection.
// Enforces a minimum delay between requests to avoid rate limiting.
func (c *Client) Fetch(url string) (string, error) {
	return c.fetchURL(url)
}

// fetchURL performs a single rate-limited GET. If a fetch function has been
// injected (SetFetchFunc), it is used instead of CycleTLS.
func (c *Client) fetchURL(url string) (string, error) {
	c.mu.Lock()
	now := time.Now()
	wait := c.minDelay - now.Sub(c.lastReq)
	c.mu.Unlock()

	if wait > 0 {
		time.Sleep(wait)
	}

	c.mu.Lock()
	c.lastReq = time.Now()
	c.mu.Unlock()

	if c.fetch != nil {
		return c.fetch(url)
	}

	resp, err := c.cycleTLS.Do(url, cycletls.Options{
		Ja3:        ja3Chrome129,
		UserAgent:  userAgentChrome129,
		Timeout:    c.timeout,
		ForceHTTP1: true,
		Headers: map[string]string{
			"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
			"Accept-Language":           "en-GB,en-US;q=0.9,en;q=0.8",
			"Accept-Encoding":           "gzip, deflate, br",
			"Sec-Ch-Ua":                 "\"Not_A Brand\";v=\"8\", \"Chromium\";v=\"129\", \"Google Chrome\";v=\"129\"",
			"Sec-Ch-Ua-Mobile":          "?0",
			"Sec-Ch-Ua-Platform":        "\"Windows\"",
			"Sec-Fetch-Dest":            "document",
			"Sec-Fetch-Mode":            "navigate",
			"Sec-Fetch-Site":            "none",
			"Sec-Fetch-User":            "?1",
			"Upgrade-Insecure-Requests": "1",
		},
	}, "GET")

	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", url, err)
	}

	if resp.Status != 200 {
		return "", fmt.Errorf("fetch %s: HTTP %d", url, resp.Status)
	}

	return resp.Body, nil
}

// holdingRecord is one row of GET /api/fund-holdings/{wtClassID}.
// The response is a JSON array; all rows share a single as-of date (DT).
// Wgt is a fraction (0.1476 = 14.76%) and rows sum to 1.0.
// See RESEARCH.md §5.1. Nullable fields are pointers (e.g. cash-like rows
// have no sector/ticker/FIGI).
type holdingRecord struct {
	DT                    string  `json:"dt"`
	WtClassID             int     `json:"wtClassID"`
	FundTicker            string  `json:"fundTicker"`
	SectorName            *string `json:"sectorName"`
	AssetGroup            string  `json:"assetGroup"`
	SecurityTicker        *string `json:"securityTicker"`
	SecurityName          string  `json:"securityName"`
	Shares                float64 `json:"shares"`
	MarketValueBase       float64 `json:"marketValueBase"`
	Wgt                   float64 `json:"wgt"`
	Figi                  *string `json:"figi"`
	CheckSumWgtAssetGroup float64 `json:"checkSumWgtAssetGroup"`
	CheckSumWgtEntity     float64 `json:"checkSumWgtEntity"`
	ExtraDataJSON         *string `json:"extraDataJSON"`
	DescriptorA           *string `json:"descriptorA"`
}

// historyPoint is one record of GET /api/fund-history/{wtClassID} (default
// view: full history since inception, ascending by date). AUM is in millions
// of the fund's base currency (47442.9648 = $47,442,965). NavDelta, NavDeltaPCT
// and NavPrevious are null on the first record. See RESEARCH.md §5.2.
type historyPoint struct {
	AUM               float64  `json:"aum"`
	DT                string   `json:"dt"`
	Name              string   `json:"name"`
	NAV               float64  `json:"nav"`
	NavDelta          *float64 `json:"navDelta"`
	NavDeltaPCT       *float64 `json:"navDeltaPCT"`
	NavPrevious       *float64 `json:"navPrevious"`
	RelatedTicker     *string  `json:"relatedTicker"`
	SharesOutstanding int      `json:"sharesOutstanding"`
	Ticker            string   `json:"ticker"`
}

// FundHoldings fetches the fund's current holdings from the JSON API.
// Returns the full holdings list as of the latest reporting date.
func (c *Client) FundHoldings(ctx context.Context, wtClassID int) ([]holdingRecord, error) {
	url := fmt.Sprintf("%s/fund-holdings/%d", c.baseURL, wtClassID)
	body, err := c.fetchURL(url)
	if err != nil {
		return nil, err
	}
	var records []holdingRecord
	if err := json.Unmarshal([]byte(body), &records); err != nil {
		return nil, fmt.Errorf("unmarshal fund-holdings %s: %w", url, err)
	}
	return records, nil
}

// FundHistory fetches the fund's NAV + AUM + shares-outstanding history from
// the JSON API (default view — full history since inception, ascending by date).
func (c *Client) FundHistory(ctx context.Context, wtClassID int) ([]historyPoint, error) {
	url := fmt.Sprintf("%s/fund-history/%d", c.baseURL, wtClassID)
	body, err := c.fetchURL(url)
	if err != nil {
		return nil, err
	}
	var points []historyPoint
	if err := json.Unmarshal([]byte(body), &points); err != nil {
		return nil, fmt.Errorf("unmarshal fund-history %s: %w", url, err)
	}
	return points, nil
}

// SetFetchFunc sets a custom fetch function (for testing).
func (c *Client) SetFetchFunc(fn fetchFunc) {
	c.fetch = fn
}
