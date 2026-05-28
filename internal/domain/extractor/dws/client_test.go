package dws

import (
	"errors"
	"testing"
	"time"
)

func TestClient_Fetch(t *testing.T) {
	tests := []struct {
		name       string
		slug       string
		endpoint   string
		mockResp   string
		mockErr    error
		wantErr    bool
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

func TestClient_RateLimiting(t *testing.T) {
	client := NewClient()
	client.minDelay = 100 * time.Millisecond
	
	count := 0
	client.SetFetchFunc(func(url string) (string, error) {
		count++
		return "{}", nil
	})

	start := time.Now()
	for i := 0; i < 3; i++ {
		_, err := client.Fetch("slug", "endpoint")
		if err != nil {
			t.Fatalf("fetch failed at iteration %d: %v", i, err)
		}
	}
	duration := time.Since(start)

	expectedMinDuration := 200 * time.Millisecond // 2 intervals of 100ms
	if duration < expectedMinDuration {
		t.Errorf("Rate limiting not applied, duration %v < %v", duration, expectedMinDuration)
	}
	if count != 3 {
		t.Errorf("Expected 3 requests, got %d", count)
	}
}
