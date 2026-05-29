package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
)

func main() {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// 1. Bypass Wall
	fmt.Println("Bypassing wall...")
	resp, err := client.Post("https://www.dimensional.com/audience-selector-api/accept-professional-affirmation", "application/json", strings.NewReader("{}"))
	if err != nil {
		fmt.Printf("Error bypassing wall: %v\n", err)
		return
	}
	resp.Body.Close()

	// 2. Fetch Portfolio Details
	fmt.Println("Fetching portfolio details API...")
	// We need the ISIN in the request. Let's try a simple POST.
	// The research says: POST to https://www.dimensional.com/investment-api/portfolio-details
	// We need to know the request body. Let's try a simple JSON with the ISIN.
	payload := `{"isin": "IE000EGGFVG6"}`
	resp, err = client.Post("https://www.dimensional.com/investment-api/portfolio-details", "application/json", strings.NewReader(payload))
	if err != nil {
		fmt.Printf("Error fetching API: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("API Response: %s\n", string(body))
}
