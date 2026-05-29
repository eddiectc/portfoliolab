package dimensional

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Danny-Dasilva/CycleTLS/cycletls"
)

// ja3Chrome129 is the JA3 TLS fingerprint for Chrome 129 on Windows.
const ja3Chrome129 = "771,4865-4866-4867-49195-49199-49196-49200-52393-52392-49171-49172-156-157-47-53,0-23-65281-10-11-35-16-5-13-18-51-45-43-27-21,29-23-24,0"

// userAgentChrome129 is the User-Agent string matching the JA3 fingerprint.
const userAgentChrome129 = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36"

// fetchFunc is the function signature for fetching page content.
type fetchFunc func(url string) (string, error)
type bypassFunc func() error

// ClientAPI defines the interface for fetching Dimensional fund data.
type ClientAPI interface {
	Fetch(url string, headers map[string]string) (string, error)
	Post(url string, body interface{}, headers map[string]string) (string, error)
}

// Client fetches Dimensional fund data with browser-grade TLS fingerprinting.
type Client struct {

	cycleTLS cycletls.CycleTLS
	mu       sync.Mutex
	lastReq  time.Time
	minDelay time.Duration
	timeout  int
	fetch    fetchFunc // overridden in tests
}

// NewClient creates a new Dimensional HTTP client with CycleTLS and rate limiting.
func NewClient() *Client {
	return &Client{
		cycleTLS: cycletls.Init(),
		minDelay: 1 * time.Second,
		timeout:  30,
	}
}

// Fetch retrieves the content of a URL using a GET request.
func (c *Client) Fetch(url string, headers map[string]string) (string, error) {
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

	opts := cycletls.Options{
		Ja3:       ja3Chrome129,
		UserAgent: userAgentChrome129,
		Timeout:   c.timeout,
		Headers:   headers,
	}

	resp, err := c.cycleTLS.Do(url, opts, "GET")

	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", url, err)
	}

	if resp.Status != 200 {
		return "", fmt.Errorf("fetch %s: HTTP %d", url, resp.Status)
	}

	return resp.Body, nil
}

// Post sends a POST request with a JSON body.
func (c *Client) Post(url string, body interface{}, headers map[string]string) (string, error) {
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

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal post body: %w", err)
	}

	opts := cycletls.Options{
		Ja3:       ja3Chrome129,
		UserAgent: userAgentChrome129,
		Timeout:   c.timeout,
		Headers:   headers,
		Body:      string(jsonBody),
	}

	resp, err := c.cycleTLS.Do(url, opts, "POST")

	if err != nil {
		return "", fmt.Errorf("post %s: %w", url, err)
	}

	if resp.Status != 200 {
		return "", fmt.Errorf("post %s: HTTP %d", url, resp.Status)
	}

	return resp.Body, nil
}

// SetFetchFunc sets a custom fetch function (for testing).
func (c *Client) SetFetchFunc(fn fetchFunc) {
	c.fetch = fn
}
