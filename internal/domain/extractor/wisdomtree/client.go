package wisdomtree

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Client fetches WisdomTree pages with rate limiting.
type Client struct {
	httpClient http.Client
	mu         sync.Mutex
	lastReq    time.Time
	minDelay   time.Duration
}

// NewClient creates a new WisdomTree HTTP client with rate limiting.
func NewClient() *Client {
	return &Client{
		httpClient: http.Client{
			Timeout: 30 * time.Second,
		},
		minDelay: 1 * time.Second,
	}
}

// Fetch retrieves the HTML content of a WisdomTree page.
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

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body %s: %w", url, err)
	}

	return string(body), nil
}
