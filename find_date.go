package main

import (
	"fmt"
	"net/http"
	"time"
)

func main() {
	isin := "IE000EGGFVG6"
	today := time.Now()
	
	for i := 0; i < 10; i++ {
		date := today.AddDate(0, 0, -i).Format("20060102")
		url := fmt.Sprintf("https://tools-blob.dimensional.com/etf/%s/%s.csv", date, isin)
		
		resp, err := http.Get(url)
		if err != nil {
			fmt.Printf("%s: error %v\n", date, err)
			continue
		}
		resp.Body.Close()
		
		if resp.StatusCode == 200 {
			fmt.Printf("Found valid CSV date: %s\n", date)
			return
		}
		fmt.Printf("%s: HTTP %d\n", date, resp.StatusCode)
	}
	fmt.Println("No valid CSV date found in the last 10 days.")
}
