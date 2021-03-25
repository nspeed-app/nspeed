package main

import (
	"context"
	"net/http"
	"time"
)

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	req, _ := http.NewRequestWithContext(ctx, "GET", "http://httpbin.org/delay/3", nil) // it will return later 3 sec

	client := &http.Client{}

	go func() {
		time.Sleep(time.Second * 2)
		println("Cancel")
		cancel()
	}()

	println("Do")
	resp, err := client.Do(req)
	println("Do finished")

	if err != nil {
		println("error", err) // cancel caught
		return
	}

	println(resp.StatusCode)
}
