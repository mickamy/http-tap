package proxy_test

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mickamy/http-tap/proxy"
)

func TestServeHTTP_ForwardsRequest(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom", "upstream-value")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"echo": r.URL.Path})
	}))
	t.Cleanup(upstream.Close)

	p, err := proxy.New(":0", upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users?page=1", nil) //nolint:noctx // test code
	req.Header.Set("Accept", "application/json")

	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	if xc := rec.Header().Get("X-Custom"); xc != "upstream-value" {
		t.Errorf("X-Custom = %q, want %q", xc, "upstream-value")
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["echo"] != "/api/users" {
		t.Errorf("echo = %q, want %q", body["echo"], "/api/users")
	}

	ev := drainEvent(t, p.Events())
	if ev.Method != http.MethodGet {
		t.Errorf("event Method = %q, want %q", ev.Method, http.MethodGet)
	}
	if ev.Path != "/api/users?page=1" {
		t.Errorf("event Path = %q, want %q", ev.Path, "/api/users?page=1")
	}
	if ev.Status != http.StatusOK {
		t.Errorf("event Status = %d, want %d", ev.Status, http.StatusOK)
	}
	if ev.ID == "" {
		t.Error("event ID is empty")
	}
	if ev.Duration <= 0 {
		t.Error("event Duration should be positive")
	}
}

func TestServeHTTP_CapturesRequestBody(t *testing.T) {
	t.Parallel()

	var gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(upstream.Close)

	p, err := proxy.New(":0", upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	reqBody := `{"name":"Alice"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(reqBody)) //nolint:noctx // test code
	req.Header.Set("Content-Type", "application/json")

	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if gotBody != reqBody {
		t.Errorf("upstream received body %q, want %q", gotBody, reqBody)
	}

	ev := drainEvent(t, p.Events())
	if string(ev.RequestBody) != reqBody {
		t.Errorf("event RequestBody = %q, want %q", ev.RequestBody, reqBody)
	}
	if ev.Status != http.StatusCreated {
		t.Errorf("event Status = %d, want %d", ev.Status, http.StatusCreated)
	}
}

func TestServeHTTP_CapturesResponseBody(t *testing.T) {
	t.Parallel()

	respJSON := `{"id":"42"}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respJSON))
	}))
	t.Cleanup(upstream.Close)

	p, err := proxy.New(":0", upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users/42", nil) //nolint:noctx // test code

	p.ServeHTTP(rec, req)

	ev := drainEvent(t, p.Events())
	if string(ev.ResponseBody) != respJSON {
		t.Errorf("event ResponseBody = %q, want %q", ev.ResponseBody, respJSON)
	}
}

func TestServeHTTP_HopByHopRemoved(t *testing.T) {
	t.Parallel()

	var upstreamHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHeaders = r.Header.Clone()
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-App", "test")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	p, err := proxy.New(":0", upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil) //nolint:noctx // test code
	req.Header.Set("Connection", "close")
	req.Header.Set("Keep-Alive", "timeout=5")

	p.ServeHTTP(rec, req)

	// Hop-by-hop headers should be stripped from upstream request.
	if upstreamHeaders.Get("Connection") != "" {
		t.Error("Connection header was forwarded to upstream")
	}
	if upstreamHeaders.Get("Keep-Alive") != "" {
		t.Error("Keep-Alive header was forwarded to upstream")
	}

	// Hop-by-hop headers should be stripped from response.
	if rec.Header().Get("Connection") != "" {
		t.Error("Connection header was forwarded to client")
	}
	// X-App should be preserved.
	if rec.Header().Get("X-App") != "test" {
		t.Errorf("X-App = %q, want %q", rec.Header().Get("X-App"), "test")
	}
}

func TestServeHTTP_XForwardedFor(t *testing.T) {
	t.Parallel()

	var gotXFF string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotXFF = r.Header.Get("X-Forwarded-For")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	p, err := proxy.New(":0", upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil) //nolint:noctx // test code
	// httptest.NewRequest sets RemoteAddr to "192.0.2.1:1234".

	p.ServeHTTP(rec, req)

	if gotXFF != "192.0.2.1" {
		t.Errorf("X-Forwarded-For = %q, want %q", gotXFF, "192.0.2.1")
	}
}

func TestServeHTTP_XForwardedFor_Appended(t *testing.T) {
	t.Parallel()

	var gotXFF string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotXFF = r.Header.Get("X-Forwarded-For")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	p, err := proxy.New(":0", upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil) //nolint:noctx // test code
	req.Header.Set("X-Forwarded-For", "10.0.0.1")

	p.ServeHTTP(rec, req)

	if gotXFF != "10.0.0.1, 192.0.2.1" {
		t.Errorf("X-Forwarded-For = %q, want %q", gotXFF, "10.0.0.1, 192.0.2.1")
	}
}

func TestServeHTTP_GzipResponse(t *testing.T) {
	t.Parallel()

	original := `{"message":"compressed"}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_, _ = gz.Write([]byte(original))
		_ = gz.Close()
	}))
	t.Cleanup(upstream.Close)

	p, err := proxy.New(":0", upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/data", nil) //nolint:noctx // test code

	p.ServeHTTP(rec, req)

	ev := drainEvent(t, p.Events())
	// Captured body should be decompressed for readability.
	if string(ev.ResponseBody) != original {
		t.Errorf("event ResponseBody = %q, want %q", ev.ResponseBody, original)
	}
}

func TestServeHTTP_LargeBodyTruncated(t *testing.T) {
	t.Parallel()

	large := strings.Repeat("x", proxy.MaxCaptureSize+1000)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(large))
	}))
	t.Cleanup(upstream.Close)

	p, err := proxy.New(":0", upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil) //nolint:noctx // test code

	p.ServeHTTP(rec, req)

	// Full response forwarded to client.
	if rec.Body.Len() != len(large) {
		t.Errorf("response body length = %d, want %d", rec.Body.Len(), len(large))
	}

	// Captured body should be truncated.
	ev := drainEvent(t, p.Events())
	if len(ev.ResponseBody) != proxy.MaxCaptureSize {
		t.Errorf("captured ResponseBody length = %d, want %d", len(ev.ResponseBody), proxy.MaxCaptureSize)
	}
}

func TestServeHTTP_UpstreamError(t *testing.T) {
	t.Parallel()

	// Point to a closed server to trigger a connection error.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	upstreamURL := upstream.URL
	upstream.Close()

	p, err := proxy.New(":0", upstreamURL)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil) //nolint:noctx // test code

	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}

	ev := drainEvent(t, p.Events())
	if ev.Error == "" {
		t.Error("event Error should be non-empty on upstream failure")
	}
	if ev.Status != http.StatusBadGateway {
		t.Errorf("event Status = %d, want %d", ev.Status, http.StatusBadGateway)
	}
}

func TestReplay(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"method": r.Method,
			"path":   r.URL.RequestURI(),
			"body":   string(b),
		})
	}))
	t.Cleanup(upstream.Close)

	p, err := proxy.New(":0", upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	ctx := t.Context()
	headers := http.Header{"X-Test": {"replay"}}
	body := []byte(`{"name":"test"}`)

	ev, err := p.Replay(ctx, http.MethodPost, "/api/users", headers, body)
	if err != nil {
		t.Fatal(err)
	}

	if ev.Method != http.MethodPost {
		t.Errorf("Method = %q, want %q", ev.Method, http.MethodPost)
	}
	if ev.Path != "/api/users" {
		t.Errorf("Path = %q, want %q", ev.Path, "/api/users")
	}
	if ev.Status != http.StatusOK {
		t.Errorf("Status = %d, want %d", ev.Status, http.StatusOK)
	}
	if ev.ID == "" {
		t.Error("ID is empty")
	}
	if string(ev.RequestBody) != string(body) {
		t.Errorf("RequestBody = %q, want %q", ev.RequestBody, body)
	}
	if len(ev.ResponseBody) == 0 {
		t.Error("ResponseBody is empty")
	}
}

func TestReplay_WithQueryString(t *testing.T) {
	t.Parallel()

	var gotQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	p, err := proxy.New(":0", upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	ctx := t.Context()
	_, err = p.Replay(ctx, http.MethodGet, "/search?q=hello&page=2", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if gotQuery != "q=hello&page=2" {
		t.Errorf("upstream query = %q, want %q", gotQuery, "q=hello&page=2")
	}
}

// drainEvent reads one event from the channel with a timeout.
func drainEvent(t *testing.T, ch <-chan proxy.Event) proxy.Event {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for event")
		return proxy.Event{}
	}
}
