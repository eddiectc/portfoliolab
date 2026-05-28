package dws

import (
	"fmt"
	"sync"
	"time"

	"github.com/Danny-Dasilva/CycleTLS/cycletls"
)

// ja3Chrome129 is the JA3 TLS fingerprint for Chrome 129 on Windows.
// This is used to bypass bot detection systems.
const ja3Chrome129 = "771,4865-4866-4867-49195-49199-49196-49200-52393-52392-49171-49172-156-157-47-53,0-23-65281-10-11-35-16-5-13-18-51-45-43-27-21,29-23-24,0"

// userAgentChrome129 is the User-Agent string matching the JA3 fingerprint.
const userAgentChrome129 = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36"

// fetchFunc is the function signature for fetching content.
// Allows injection of mock fetchers in tests.
type fetchFunc func(url string) (string, error)

// Client fetches data from the DWS JSON API.
type Client struct {
	cycleTLS   cycletls.CycleTLS
	mu         sync.Mutex
	lastReq    time.Time
	minDelay   time.Duration
	timeout    int
	fetch      fetchFunc // overridden in tests
	baseURL    string
}

// NewClient creates a new DWS HTTP client with CycleTLS and rate limiting.
func NewClient() *Client {
	return &Client{
		cycleTLS: cycletls.Init(),
		minDelay: 1 * time.Second,
		timeout:  30,
		baseURL:  "https://etf.dws.com/api/pdp/en-gb/etf",
	}
}

// Fetch retrieves JSON content from a specific DWS API endpoint for a given slug.
// Enforces a minimum delay between requests to avoid rate limiting.
func (c *Client) Fetch(slug, endpoint string) (string, error) {
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

	fullURL := fmt.Sprintf("%s/%s/%s", c.baseURL, slug, endpoint)
	// Log the URL being fetched for debugging purposes
	fmt.Printf("DWS API Request: %s\n", fullURL)

	if c.fetch != nil {
		return c.fetch(fullURL)
	}

	resp, err := c.cycleTLS.Do(fullURL, cycletls.Options{
		Ja3:       ja3Chrome129,
		UserAgent: userAgentChrome129,
		Timeout:   c.timeout,
	}, "GET")

	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", fullURL, err)
	}

	if resp.Status != 200 {
		return "", fmt.Errorf("fetch %s: HTTP %d", fullURL, resp.Status)
	}

	return resp.Body, nil
}

// SetFetchFunc sets a custom fetch function (for testing).
func (c *Client) SetFetchFunc(fn fetchFunc) {
	c.fetch = fn
}
