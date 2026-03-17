# http-tap

Real-time HTTP traffic viewer — proxy daemon + TUI / Web UI.

http-tap sits between your application and your HTTP server, capturing every request and displaying it in an interactive terminal UI or web browser. Inspect headers, view request/response bodies, copy data, and replay requests — all without changing your application code.

## Installation

### Homebrew

```bash
brew install mickamy/tap/http-tap
```

### Go

```bash
go install github.com/mickamy/http-tap@latest
go install github.com/mickamy/http-tap/cmd/http-tapd@latest
```

### Build from source

```bash
git clone https://github.com/mickamy/http-tap.git
cd http-tap
make install
```

## Quick start

**1. Start the proxy daemon**

```bash
# Proxy listens on :8080, forwards to upstream HTTP server on :9000
# Web UI available at :8081
http-tapd -listen :8080 -upstream http://localhost:9000 -http :8081
```

**2. Point your application at the proxy**

Connect your app to the proxy port (`:8080`) instead of the upstream port. No code changes needed — http-tapd is a transparent HTTP reverse proxy.

**3. Launch the TUI or open the Web UI**

```bash
# TUI
http-tap localhost:9090

# Or open Web UI in your browser
open http://localhost:8081
```

All HTTP requests flowing through the proxy appear in real-time.

## Usage

### http-tapd

```
http-tapd — HTTP proxy daemon for http-tap

Usage:
  http-tapd [flags]

Flags:
  -listen          client listen address (required)
  -upstream        upstream HTTP server address (required)
  -grpc            gRPC server address for TUI (default: ":9090")
  -http            HTTP server address for web UI (e.g. :8081)
  -tls-skip-verify skip TLS certificate verification for HTTPS upstream
  -version         show version and exit
```

### http-tap

```
http-tap — Watch HTTP traffic in real-time

Usage:
  http-tap [flags] <addr>

Flags:
  -version  show version and exit
```

`<addr>` is the gRPC address of http-tapd (e.g. `localhost:9090`).

## Keybindings (TUI)

### List view

| Key               | Action                               |
|-------------------|--------------------------------------|
| `j` / `↓`         | Move down                            |
| `k` / `↑`         | Move up                              |
| `Ctrl+d` / `PgDn` | Half-page down                       |
| `Ctrl+u` / `PgUp` | Half-page up                         |
| `/`               | Incremental search                   |
| `s`               | Toggle sort (chronological/duration) |
| `Enter`           | Inspect request                      |
| `e`               | Toggle error filter                  |
| `a`               | Analytics view                       |
| `w`               | Export (JSON/Markdown)               |
| `Esc`             | Clear search filter                  |
| `q`               | Quit                                 |

### Inspector view

| Key       | Action                |
|-----------|-----------------------|
| `j` / `↓` | Scroll down           |
| `k` / `↑` | Scroll up             |
| `c`       | Copy request body     |
| `C`       | Copy response body    |
| `e`       | Edit request & resend |
| `q`       | Back to list          |

### Analytics view

| Key       | Action                                  |
|-----------|-----------------------------------------|
| `j` / `↓` | Move down                               |
| `k` / `↑` | Move up                                 |
| `Ctrl+d`  | Half-page down                          |
| `Ctrl+u`  | Half-page up                            |
| `s`       | Cycle sort (total/count/avg/error rate) |
| `q`       | Back to list                            |

## How it works

```
┌─────────────┐      ┌───────────────────────┐      ┌─────────────────┐
│ Application │─────▶│  http-tapd (proxy)    │─────▶│ HTTP Server     │
└─────────────┘      │                       │      └─────────────────┘
                     │  captures requests    │
                     │  via reverse proxy    │
                     └─────┬─────────┬───────┘
                           │         │
               gRPC stream │         │ SSE
                     ┌─────▼───┐ ┌───▼───────┐
                     │ TUI     │ │ Web UI    │
                     │(http-tap│ │(:8081)    │
                     └─────────┘ └───────────┘
```

http-tapd acts as an HTTP reverse proxy that transparently forwards requests to the upstream server. It captures request/response headers, bodies (up to 64 KB), status codes, and timing for each request. Events are streamed to TUI clients via gRPC and to the Web UI via Server-Sent Events (SSE).

### HTTPS upstream

Use `-tls-skip-verify` to proxy to HTTPS upstreams with self-signed certificates:

```bash
http-tapd -listen :8080 -upstream https://localhost:9443 -tls-skip-verify
```

### Edit & Resend

Press `e` in the TUI inspector to open the captured request in `$EDITOR` as JSON (method, path, headers, body). After editing, the modified request is sent to the upstream server via the proxy, and the result appears in the event stream.

The Web UI also supports replay via the Replay button in the detail panel.

### Export

Press `w` in the TUI to export captured requests:
- `j` — JSON format
- `m` — Markdown format

Exports include request details and per-endpoint analytics (count, errors, avg/p95/max duration).

## Example

An example server and client are included for testing:

```bash
# Start the example upstream server
go run ./example/server

# Start the proxy daemon with web UI
http-tapd -listen :8080 -upstream http://localhost:9000 -grpc :9090 -http :8081

# Generate traffic
go run ./example/client

# Watch in TUI
http-tap localhost:9090

# Or open Web UI
open http://localhost:8081
```

## License

[MIT](./LICENSE)
