package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// hopByHopHeaders lists headers that must not be forwarded by a proxy
// (RFC 7230 Section 6.1).
var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

// ReverseProxy is an HTTP reverse proxy that captures request/response traffic.
type ReverseProxy struct {
	listenAddr string
	upstream   *url.URL
	events     chan Event
	server     *http.Server
	transport  *http.Transport
}

// Option configures the reverse proxy.
type Option func(*ReverseProxy)

// WithTLSSkipVerify disables TLS certificate verification for HTTPS upstreams.
func WithTLSSkipVerify() Option {
	return func(rp *ReverseProxy) {
		rp.transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // user-opt-in
	}
}

// New creates a new ReverseProxy.
// listenAddr is the address to listen on (e.g. ":8080").
// upstreamAddr is the upstream server URL (e.g. "http://localhost:9000").
func New(listenAddr, upstreamAddr string, opts ...Option) (*ReverseProxy, error) {
	u, err := url.Parse(upstreamAddr)
	if err != nil {
		return nil, fmt.Errorf("proxy: parse upstream: %w", err)
	}

	rp := &ReverseProxy{
		listenAddr: listenAddr,
		upstream:   u,
		events:     make(chan Event, 256),
		transport:  &http.Transport{},
	}

	for _, opt := range opts {
		opt(rp)
	}

	rp.server = &http.Server{
		Addr:              listenAddr,
		Handler:           rp,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return rp, nil
}

// ListenAndServe starts the proxy and blocks until ctx is cancelled.
func (rp *ReverseProxy) ListenAndServe(ctx context.Context) error {
	lis, err := net.Listen("tcp", rp.listenAddr) //nolint:noctx // uses ctx for shutdown
	if err != nil {
		return fmt.Errorf("proxy: listen %s: %w", rp.listenAddr, err)
	}

	go func() {
		<-ctx.Done()
		_ = rp.server.Close()
	}()

	if err := rp.server.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("proxy: serve: %w", err)
	}
	close(rp.events)
	return nil
}

// Events returns the channel of captured events.
func (rp *ReverseProxy) Events() <-chan Event {
	return rp.events
}

// Close stops the proxy.
func (rp *ReverseProxy) Close() error {
	return rp.server.Close() //nolint:wrapcheck // pass-through
}

// Replay sends an HTTP request to the upstream server and returns the resulting event.
// The event is also published to the events channel.
func (rp *ReverseProxy) Replay(ctx context.Context, method, path string, headers http.Header, body []byte) (Event, error) {
	start := time.Now()

	upstreamURL := rp.buildUpstreamURL(path)

	var reqBody io.Reader
	if len(body) > 0 {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, upstreamURL, reqBody)
	if err != nil {
		return Event{}, fmt.Errorf("replay: build request: %w", err)
	}
	if headers != nil {
		copyHeaders(req.Header, headers)
	}

	resp, err := rp.transport.RoundTrip(req)
	if err != nil {
		return Event{}, fmt.Errorf("replay: roundtrip: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respData, err := io.ReadAll(io.LimitReader(resp.Body, MaxCaptureSize))
	if err != nil {
		return Event{}, fmt.Errorf("replay: read response: %w", err)
	}

	ev := Event{
		ID:              uuid.New().String(),
		Method:          method,
		Path:            path,
		Status:          int32(resp.StatusCode),
		StartTime:       start,
		Duration:        time.Since(start),
		RequestHeaders:  req.Header.Clone(),
		ResponseHeaders: resp.Header.Clone(),
		RequestBody:     body,
		ResponseBody:    DecompressGzip(respData),
	}

	select {
	case rp.events <- ev:
	default:
	}

	return ev, nil
}

// ServeHTTP handles each proxied request.
func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	reqCapture := NewCaptureReader(r.Body, MaxCaptureSize)

	upstreamURL := rp.buildUpstreamURL(r.URL.RequestURI())

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, reqCapture)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	copyHeaders(outReq.Header, r.Header)
	removeHopByHop(outReq.Header)

	if clientIP, _, splitErr := net.SplitHostPort(r.RemoteAddr); splitErr == nil {
		if prior := outReq.Header.Get("X-Forwarded-For"); prior != "" {
			outReq.Header.Set("X-Forwarded-For", prior+", "+clientIP)
		} else {
			outReq.Header.Set("X-Forwarded-For", clientIP)
		}
	}

	resp, err := rp.transport.RoundTrip(outReq)
	if err != nil {
		rp.emitEvent(Event{
			ID:             uuid.New().String(),
			Method:         r.Method,
			Path:           r.URL.RequestURI(),
			Status:         http.StatusBadGateway,
			StartTime:      start,
			Duration:       time.Since(start),
			Error:          err.Error(),
			RequestHeaders: r.Header.Clone(),
			RequestBody:    DecompressGzip(reqCapture.Bytes()),
		})
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	copyHeaders(w.Header(), resp.Header)
	removeHopByHop(w.Header())
	w.WriteHeader(resp.StatusCode)

	respCapture := NewCaptureReader(resp.Body, MaxCaptureSize)

	if f, ok := w.(http.Flusher); ok {
		buf := make([]byte, 32*1024)
		for {
			n, readErr := respCapture.Read(buf)
			if n > 0 {
				_, _ = w.Write(buf[:n])
				f.Flush()
			}
			if readErr != nil {
				break
			}
		}
	} else {
		_, _ = io.Copy(w, respCapture)
	}

	rp.emitEvent(Event{
		ID:              uuid.New().String(),
		Method:          r.Method,
		Path:            r.URL.RequestURI(),
		Status:          int32(resp.StatusCode),
		StartTime:       start,
		Duration:        time.Since(start),
		RequestHeaders:  r.Header.Clone(),
		ResponseHeaders: resp.Header.Clone(),
		RequestBody:     DecompressGzip(reqCapture.Bytes()),
		ResponseBody:    DecompressGzip(respCapture.Bytes()),
	})
}

func (rp *ReverseProxy) emitEvent(ev Event) {
	select {
	case rp.events <- ev:
	default:
	}
}

func (rp *ReverseProxy) buildUpstreamURL(reqPath string) string {
	u := *rp.upstream
	if idx := strings.IndexByte(reqPath, '?'); idx >= 0 {
		u.Path = singleJoiningSlash(rp.upstream.Path, reqPath[:idx])
		u.RawQuery = reqPath[idx+1:]
	} else {
		u.Path = singleJoiningSlash(rp.upstream.Path, reqPath)
	}
	return u.String()
}

func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

func removeHopByHop(h http.Header) {
	for _, key := range hopByHopHeaders {
		h.Del(key)
	}
}

func singleJoiningSlash(a, b string) string {
	aSlash := strings.HasSuffix(a, "/")
	bSlash := strings.HasPrefix(b, "/")
	switch {
	case aSlash && bSlash:
		return a + b[1:]
	case !aSlash && !bSlash:
		return a + "/" + b
	}
	return a + b
}
