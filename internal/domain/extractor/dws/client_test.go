package dws

import (
	"errors"
	"testing"
	"time"
)

func TestClient_Fetch(t *testing.T) {
	tests := []struct {
		name        string
		slug        string
		endpoint    string
		mockResp    string
		mockErr     error
		wantErr     bool
		expectedURL string
	}{
		{
			name:        "successful fetch",
			slug:        "test-slug",
			endpoint:    "pdpSettings",
			mockResp:    `{"status": "ok"}`,
			mockErr:     nil,
			wantErr:     false,
			expectedURL: "https://etf.dws.com/api/pdp/en-gb/etf/test-slug/pdpSettings",
		},
		{
			name:        "api error",
			slug:        "test-slug",
			endpoint:    "pdpSettings",
			mockResp:    "",
			mockErr:     errors.New("network error"),
			wantErr:     true,
			expectedURL: "https://etf.dws.com/api/pdp/en-gb/etf/test-slug/pdpSettings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient()

			// Use a captured URL to verify the endpoint construction
			var capturedURL string
			client.SetFetchFunc(func(url string) (string, error) {
				capturedURL = url
				return tt.mockResp, tt.mockErr
			})

			got, err := client.Fetch(tt.slug, tt.endpoint)

			if (err != nil) != tt.wantErr {
				t.Errorf("Client.Fetch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.mockResp {
				t.Errorf("Client.Fetch() got = %v, want %v", got, tt.mockResp)
			}
			if capturedURL != tt.expectedURL {
				t.Errorf("Client.Fetch() capturedURL = %v, want %v", capturedURL, tt.expectedURL)
			}
		})
	}
}

// TestClient_RateLimiting verifies the limiter waits ~minDelay before every
// request after the first. Waits are asserted via the throttle hook, so the
// test is deterministic and sleep-free, and runs against the production
// minDelay (1s).
func TestClient_RateLimiting(t *testing.T) {
	client := NewClient()
	var waits []time.Duration
	client.SetThrottle(func(d time.Duration) { waits = append(waits, d) })

	count := 0
	client.SetFetchFunc(func(url string) (string, error) {
		count++
		return "{}", nil
	})

	for i := 0; i < 3; i++ {
		_, err := client.Fetch("slug", "endpoint")
		if err != nil {
			t.Fatalf("fetch failed at iteration %d: %v", i, err)
		}
	}

	if count != 3 {
		t.Errorf("Expected 3 requests, got %d", count)
	}
	if len(waits) != 2 {
		t.Fatalf("throttle called %d times, want 2 (first request must not wait)", len(waits))
	}
	for i, w := range waits {
		// Mocked fetches are instantaneous, so each wait is ~minDelay
		// (1s minus the few microseconds between requests).
		if w < 900*time.Millisecond {
			t.Errorf("throttle wait[%d] = %v, want ~%v", i, w, client.minDelay)
		}
	}
}
