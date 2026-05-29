package main

import (
	"fmt"
	"net/http"
	"time"
)

func main() {
	isin := "IE000EGGFVG6"
	today := time.Now()
	
	fmt.Println("Probing for the most recent available CSV date...")
	for i := 0; i < 14; i++ {
		date := today.AddDate(0, 0, -i)
		dateStr := date.Format("20060102")
		url := fmt.Sprintf("https://tools-blob.dimensional.com/etf/%s/%s.csv", dateStr, isin)
		
		resp, err := http.Get(url)
		if err != nil {
			fmt.Printf("%s: error %v\n", dateStr, err)
			continue
		}
		resp.Body.Close()
		
		if resp.StatusCode == 200 {
			fmt.Printf("Found valid CSV date: %s (found after %d days offset)\n", dateStr, i)
			return
		}
		fmt.Printf("%s: HTTP %d\n", dateStr, resp.StatusCode)
	}
	fmt.Println("No valid CSV date found in the last 14 days.")
}
