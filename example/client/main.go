package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	addr := "http://localhost:8080" // proxy address
	if a := os.Getenv("PROXY_ADDR"); a != "" {
		addr = a
	}

	if err := run(addr); err != nil {
		log.Fatal(err)
	}
}

func run(addr string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client := &http.Client{Timeout: 10 * time.Second}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for i := 1; ; i++ {
		doRequest(ctx, client, "GET", addr+"/api/users", "")
		doRequest(ctx, client, "GET", addr+fmt.Sprintf("/api/users/%d", (i%3)+1), "")
		doRequest(ctx, client, "POST", addr+"/api/users", fmt.Sprintf(`{"name":"User-%d"}`, i))
		doRequest(ctx, client, "GET", addr+"/api/slow", "")

		if i%3 == 0 {
			doRequest(ctx, client, "GET", addr+"/api/error", "")
			doRequest(ctx, client, "DELETE", addr+"/api/users/999", "")
			doRequest(ctx, client, "POST", addr+"/api/users", "invalid-json")
		}

		select {
		case <-ctx.Done():
			fmt.Println("shutting down")
			return nil
		case <-ticker.C:
		}
	}
}

func doRequest(ctx context.Context, client *http.Client, method, url, body string) {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader) //nolint:gosec // G704: URL is from configuration
	if err != nil {
		log.Printf("[%s %s] error creating request: %v", method, url, err) //nolint:gosec // G706: example code
		return
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req) //nolint:gosec
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Printf("[%s %s] error: %v", method, url, err) //nolint:gosec // G706: example code
		return
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("[%s %s] %d %s\n", method, url, resp.StatusCode, string(respBody))
}
