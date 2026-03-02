package proxy

import (
	"context"
	"net/http"
	"time"
)

// MaxCaptureSize is the maximum number of bytes captured per request/response body.
const MaxCaptureSize = 64 * 1024

// Event represents a captured HTTP request/response pair.
type Event struct {
	ID              string
	Method          string // HTTP method: GET, POST, PUT, DELETE, etc.
	Path            string // Request path including query string, e.g. /api/users?page=1
	Status          int32  // HTTP status code: 200, 404, 500, etc.
	StartTime       time.Time
	Duration        time.Duration
	Error           string // Non-empty when the upstream request failed.
	RequestHeaders  http.Header
	ResponseHeaders http.Header
	RequestBody     []byte // Captured request body (up to MaxCaptureSize).
	ResponseBody    []byte // Captured response body (up to MaxCaptureSize).
}

// Proxy is the interface for HTTP reverse proxies.
type Proxy interface {
	// ListenAndServe accepts client connections and relays them to the upstream server.
	ListenAndServe(ctx context.Context) error
	// Events returns the channel of captured events.
	Events() <-chan Event
	// Replay sends a request to the upstream server and returns the resulting event.
	Replay(ctx context.Context, method, path string, headers http.Header, body []byte) (Event, error)
	// Close stops the proxy.
	Close() error
}
