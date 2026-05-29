package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
)

func main() {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// 1. Bypass Wall
	resp, err := client.Post("https://www.dimensional.com/audience-selector-api/accept-professional-affirmation", "application/json", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	resp.Body.Close()

	// 2. Fetch Fund Center
	url := "https://etf.dimensional.com/public/v2/fundcenter?allowMorningstarFixedIncome=true"
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("x-selected-country", "GB")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36")
	
	resp, err = client.Do(req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
}
