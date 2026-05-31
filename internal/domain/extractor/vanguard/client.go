package vanguard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	baseURL        = "https://www.vanguardinvestor.co.uk"
	restPath       = "/api/funds/%s"
	graphqlPath    = "/gpx/graphql"
	defaultTimeout = 30 * time.Second
)

// fetchFunc is the function signature for fetching content.
// Allows injection of mock fetchers in tests.
type fetchFunc func(method, url string, body []byte) ([]byte, error)

// restFunc is the function signature for REST GET requests.
type restFunc func(slug string) ([]byte, error)

// graphqlFunc is the function signature for GraphQL POST requests.
type graphqlFunc func(operationName string, variables map[string]interface{}, query string) ([]byte, error)

// Client fetches data from Vanguard's REST and GraphQL APIs.
// Uses standard http.Client — Vanguard's API has no Cloudflare protection.
type Client struct {
	httpClient   *http.Client
	mu           sync.Mutex
	lastReq      time.Time
	minDelay     time.Duration
	pageDelay    time.Duration
	fetch        fetchFunc   // overridden in tests
	restFetch    restFunc    // overridden in tests
	graphqlFetch graphqlFunc // overridden in tests
}

// NewClient creates a new Vanguard HTTP client with rate limiting.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		minDelay:  1 * time.Second,        // between major queries
		pageDelay: 500 * time.Millisecond, // between pagination pages
	}
}

// FetchREST retrieves JSON from the REST API for a given fund slug.
func (c *Client) FetchREST(slug string) ([]byte, error) {
	c.enforceDelay(c.minDelay)

	if c.restFetch != nil {
		return c.restFetch(slug)
	}

	url := fmt.Sprintf(baseURL+restPath, slug)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch REST %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch REST %s: HTTP %d", url, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// FetchGraphQL sends a GraphQL POST request and returns the raw JSON response.
func (c *Client) FetchGraphQL(operationName string, variables map[string]interface{}, query string) ([]byte, error) {
	c.enforceDelay(c.minDelay)

	if c.graphqlFetch != nil {
		return c.graphqlFetch(operationName, variables, query)
	}

	body := map[string]interface{}{
		"operationName": operationName,
		"variables":     variables,
		"query":         query,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal GraphQL body: %w", err)
	}

	resp, err := c.httpClient.Post(baseURL+graphqlPath, "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("fetch GraphQL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch GraphQL: HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// FetchGraphQLPage sends a paginated GraphQL request with a delay for pagination.
func (c *Client) FetchGraphQLPage(operationName string, variables map[string]interface{}, query string) ([]byte, error) {
	c.enforceDelay(c.pageDelay)
	return c.FetchGraphQL(operationName, variables, query)
}

// enforceDelay ensures minimum time between requests.
func (c *Client) enforceDelay(minDelay time.Duration) {
	c.mu.Lock()
	now := time.Now()
	wait := minDelay - now.Sub(c.lastReq)
	c.mu.Unlock()

	if wait > 0 {
		time.Sleep(wait)
	}

	c.mu.Lock()
	c.lastReq = time.Now()
	c.mu.Unlock()
}

// SetFetchFunc sets a custom fetch function (for testing).
func (c *Client) SetFetchFunc(fn fetchFunc) {
	c.fetch = fn
}

// SetRestFetch sets a custom REST fetch function (for testing).
func (c *Client) SetRestFetch(fn restFunc) {
	c.restFetch = fn
}

// SetGraphqlFetch sets a custom GraphQL fetch function (for testing).
func (c *Client) SetGraphqlFetch(fn graphqlFunc) {
	c.graphqlFetch = fn
}
