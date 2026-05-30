package imgp

import (
	"errors"
	"testing"
	"time"
)

func TestClient_FetchPage(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		mockResp   string
		mockErr    error
		wantErr    bool
	}{
		{
			name:     "successful fetch",
			url:      "https://www.imgp.com/fund/LU2951555585",
			mockResp: `<html><body>fund page</body></html>`,
			mockErr:  nil,
			wantErr:  false,
		},
		{
			name:     "network error",
			url:      "https://www.imgp.com/fund/LU2951555585",
			mockResp: "",
			mockErr:  errors.New("network error"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient()

			var capturedURL string
			client.SetFetchFunc(func(url string) (string, error) {
				capturedURL = url
				return tt.mockResp, tt.mockErr
			})

			got, err := client.FetchPage(tt.url)

			if (err != nil) != tt.wantErr {
				t.Errorf("Client.FetchPage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.mockResp {
				t.Errorf("Client.FetchPage() got = %v, want %v", got, tt.mockResp)
			}
			if capturedURL != tt.url {
				t.Errorf("Client.FetchPage() capturedURL = %v, want %v", capturedURL, tt.url)
			}
		})
	}
}

func TestClient_FetchPDF(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		mockResp   []byte
		mockErr    error
		wantErr    bool
	}{
		{
			name:     "successful fetch",
			url:      "https://www.imgp.com/wp-content/uploads/factsheet.pdf",
			mockResp: []byte("%PDF-1.4 test content"),
			mockErr:  nil,
			wantErr:  false,
		},
		{
			name:     "network error",
			url:      "https://www.imgp.com/wp-content/uploads/factsheet.pdf",
			mockResp: nil,
			mockErr:  errors.New("network error"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient()

			var capturedURL string
			client.SetFetchPDFFunc(func(url string) ([]byte, error) {
				capturedURL = url
				return tt.mockResp, tt.mockErr
			})

			got, err := client.FetchPDF(tt.url)

			if (err != nil) != tt.wantErr {
				t.Errorf("Client.FetchPDF() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if string(got) != string(tt.mockResp) {
				t.Errorf("Client.FetchPDF() got = %v, want %v", got, tt.mockResp)
			}
			if capturedURL != tt.url {
				t.Errorf("Client.FetchPDF() capturedURL = %v, want %v", capturedURL, tt.url)
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
		return "ok", nil
	})

	start := time.Now()
	for i := 0; i < 3; i++ {
		_, err := client.FetchPage("https://www.imgp.com/fund/test")
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

func TestExtractPDFURL(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		pageURL string
		wantURL string
		wantErr bool
	}{
		{
			name: "html contains factsheet link",
			html: `<p class="title">Factsheets</p>
<a href="https://www.imgp.com/uploads/factsheets/LU2951555585_FACTSHEETS_EN.pdf" target="_blank">`,
			pageURL: "https://www.imgp.com/fund/LU2951555585",
			wantURL: "https://www.imgp.com/uploads/factsheets/LU2951555585_FACTSHEETS_EN.pdf",
			wantErr: false,
		},
		{
			name: "html contains factsheet link with single quotes",
			html: `<a href='https://www.imgp.com/uploads/factsheets/IE00B_FACTSHEETS_EN.pdf'>`,
			pageURL: "https://www.imgp.com/fund/IE00B",
			wantURL: "https://www.imgp.com/uploads/factsheets/IE00B_FACTSHEETS_EN.pdf",
			wantErr: false,
		},
		{
			name: "case insensitive factsheet match",
			html: `<a href="https://www.imgp.com/uploads/factsheets/LU2951555585_factsheets_en.pdf">`,
			pageURL: "https://www.imgp.com/fund/LU2951555585",
			wantURL: "https://www.imgp.com/uploads/factsheets/LU2951555585_factsheets_en.pdf",
			wantErr: false,
		},
		{
			name:    "no factsheet link in html",
			html:    `<html><body>no factsheet link here</body></html>`,
			pageURL: "https://www.imgp.com/fund/LU2951555585",
			wantURL: "",
			wantErr: true,
		},
		{
			name:    "empty html",
			html:    "",
			pageURL: "https://www.imgp.com/fund/LU2951555585",
			wantURL: "",
			wantErr: true,
		},
		{
			name:    "only Italian factsheet, no English",
			html:    `<a href="https://www.imgp.com/uploads/factsheets/LU2951555585_FACTSHEETS_IT.pdf">`,
			pageURL: "https://www.imgp.com/fund/LU2951555585",
			wantURL: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractPDFURL(tt.html, tt.pageURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractPDFURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantURL {
				t.Errorf("extractPDFURL() = %q, want %q", got, tt.wantURL)
			}
		})
	}
}
