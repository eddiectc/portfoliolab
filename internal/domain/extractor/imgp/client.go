package imgp

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/Danny-Dasilva/CycleTLS/cycletls"
)

// ja3Chrome129 is the JA3 TLS fingerprint for Chrome 129 on Windows.
const ja3Chrome129 = "771,4865-4866-4867-49195-49199-49196-49200-52393-52392-49171-49172-156-157-47-53,0-23-65281-10-11-35-16-5-13-18-51-45-43-27-21,29-23-24,0"

// userAgentChrome129 is the User-Agent string matching the JA3 fingerprint.
const userAgentChrome129 = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36"

// fetchFunc is the function signature for fetching page content (HTML).
// Allows injection of mock fetchers in tests.
type fetchFunc func(url string) (string, error)

// fetchPDFFunc is the function signature for fetching PDF bytes.
// Allows injection of mock fetchers in tests.
type fetchPDFFunc func(url string) ([]byte, error)

// Client fetches iMGP pages and PDFs with browser-grade TLS fingerprinting.
type Client struct {
	cycleTLS cycletls.CycleTLS
	mu       sync.Mutex
	lastReq  time.Time
	minDelay time.Duration
	timeout  int
	fetch    fetchFunc           // overridden in tests
	fetchPDF fetchPDFFunc        // overridden in tests
	throttle func(time.Duration) // replaces time.Sleep for testability; default time.Sleep
}

// NewClient creates a new iMGP HTTP client with CycleTLS and rate limiting.
func NewClient() *Client {
	return &Client{
		cycleTLS: cycletls.Init(),
		minDelay: 1 * time.Second,
		timeout:  30,
		throttle: time.Sleep,
	}
}

// FetchPage retrieves the HTML content of an iMGP fund page.
// Uses CycleTLS with browser-grade TLS fingerprinting to bypass bot protection.
// Enforces a minimum delay between requests to avoid rate limiting.
func (c *Client) FetchPage(url string) (string, error) {
	c.wait()

	if c.fetch != nil {
		return c.fetch(url)
	}

	resp, err := c.cycleTLS.Do(url, cycletls.Options{
		Ja3:       ja3Chrome129,
		UserAgent: userAgentChrome129,
		Timeout:   c.timeout,
	}, "GET")

	if err != nil {
		return "", fmt.Errorf("fetch page %s: %w", url, err)
	}

	if resp.Status != 200 {
		return "", fmt.Errorf("fetch page %s: HTTP %d", url, resp.Status)
	}

	return resp.Body, nil
}

// FetchPDF retrieves the raw bytes of a factsheet PDF.
// Uses CycleTLS with browser-grade TLS fingerprinting to bypass bot protection.
// Enforces a minimum delay between requests to avoid rate limiting.
func (c *Client) FetchPDF(url string) ([]byte, error) {
	c.wait()

	if c.fetchPDF != nil {
		return c.fetchPDF(url)
	}

	resp, err := c.cycleTLS.Do(url, cycletls.Options{
		Ja3:       ja3Chrome129,
		UserAgent: userAgentChrome129,
		Timeout:   c.timeout,
	}, "GET")

	if err != nil {
		return nil, fmt.Errorf("fetch pdf %s: %w", url, err)
	}

	if resp.Status != 200 {
		return nil, fmt.Errorf("fetch pdf %s: HTTP %d", url, resp.Status)
	}

	// CycleTLS returns PDF bodies as base64-encoded strings (see DecompressBody
	// for application/pdf content type). Decode back to raw bytes.
	pdfBytes, err := base64.StdEncoding.DecodeString(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decode pdf base64: %w", err)
	}

	return pdfBytes, nil
}

// factsheetENPattern matches the English factsheet PDF link in the fund page HTML.
// e.g. href="https://www.imgp.com/uploads/factsheets/LU2951555585_FACTSHEETS_EN.pdf"
var factsheetENPattern = regexp.MustCompile(`(?i)href=["'](https://[^"']*FACTSHEETS_EN\.pdf)["']`)

// extractPDFURL finds the factsheet PDF download link in the fund page HTML.
// Returns the absolute PDF URL or an error if no factsheet link is found.
func extractPDFURL(html, pageURL string) (string, error) {
	matches := factsheetENPattern.FindStringSubmatch(html)
	if len(matches) == 2 {
		return matches[1], nil
	}

	return "", fmt.Errorf("no factsheet PDF link found in page %q", pageURL)
}

// SetFetchFunc sets a custom fetch function for HTML pages (for testing).
func (c *Client) SetFetchFunc(fn fetchFunc) {
	c.fetch = fn
}

// SetFetchPDFFunc sets a custom fetch function for PDF bytes (for testing).
func (c *Client) SetFetchPDFFunc(fn fetchPDFFunc) {
	c.fetchPDF = fn
}

// SetMinDelay sets the minimum delay between requests (for testing).
func (c *Client) SetMinDelay(d time.Duration) {
	c.minDelay = d
}

// SetThrottle sets the function used to wait between requests (for testing).
// Production uses time.Sleep; tests may pass a no-op or recorder to assert
// the wait sequence without burning wall-clock time.
func (c *Client) SetThrottle(fn func(time.Duration)) {
	c.throttle = fn
}

// wait enforces the minimum delay between requests.
func (c *Client) wait() {
	c.mu.Lock()
	now := time.Now()
	wait := c.minDelay - now.Sub(c.lastReq)
	c.mu.Unlock()

	if wait > 0 {
		c.throttle(wait)
	}

	c.mu.Lock()
	c.lastReq = time.Now()
	c.mu.Unlock()
}
