package wisdomtree

import (
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

// Client fetches WisdomTree pages with browser-grade TLS fingerprinting to bypass Cloudflare.
type Client struct {
	cycleTLS   cycletls.CycleTLS
	mu         sync.Mutex
	lastReq    time.Time
	minDelay   time.Duration
	timeout    int
	fetch      fetchFunc // overridden in tests
}

// NewClient creates a new WisdomTree HTTP client with CycleTLS and rate limiting.
func NewClient() *Client {
	return &Client{
		cycleTLS: cycletls.Init(),
		minDelay: 1 * time.Second,
		timeout:  30,
	}
}

// Fetch retrieves the HTML content of a WisdomTree page.
// Uses CycleTLS with browser-grade TLS fingerprinting to bypass Cloudflare bot protection.
// Enforces a minimum delay between requests to avoid rate limiting.
func (c *Client) Fetch(url string) (string, error) {
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
			"Accept":              "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
			"Accept-Language":     "en-GB,en-US;q=0.9,en;q=0.8",
			"Accept-Encoding":     "gzip, deflate, br",
			"Sec-Ch-Ua":           "\"Not_A Brand\";v=\"8\", \"Chromium\";v=\"129\", \"Google Chrome\";v=\"129\"",
			"Sec-Ch-Ua-Mobile":    "?0",
			"Sec-Ch-Ua-Platform":  "\"Windows\"",
			"Sec-Fetch-Dest":      "document",
			"Sec-Fetch-Mode":      "navigate",
			"Sec-Fetch-Site":      "none",
			"Sec-Fetch-User":      "?1",
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

// SetFetchFunc sets a custom fetch function (for testing).
func (c *Client) SetFetchFunc(fn fetchFunc) {
	c.fetch = fn
}
