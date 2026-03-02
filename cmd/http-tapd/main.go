package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mickamy/http-tap/broker"
	"github.com/mickamy/http-tap/proxy"
	"github.com/mickamy/http-tap/server"
	"github.com/mickamy/http-tap/web"
)

var version = "dev"

func main() {
	fs := flag.NewFlagSet("http-tapd", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "http-tapd — HTTP proxy daemon for http-tap\n\nUsage:\n  http-tapd [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}

	listen := fs.String("listen", "", "client listen address (required)")
	upstream := fs.String("upstream", "", "upstream HTTP server address (required)")
	grpcAddr := fs.String("grpc", ":9090", "gRPC server address for TUI")
	httpAddr := fs.String("http", "", "HTTP server address for web UI (e.g. :8080)")
	tlsSkipVerify := fs.Bool("tls-skip-verify", false, "skip TLS certificate verification for HTTPS upstream")
	showVersion := fs.Bool("version", false, "show version and exit")

	_ = fs.Parse(os.Args[1:])

	if *showVersion {
		fmt.Printf("http-tapd %s\n", version)
		return
	}

	if *listen == "" || *upstream == "" {
		fs.Usage()
		os.Exit(1)
	}

	if err := run(*listen, *upstream, *grpcAddr, *httpAddr, *tlsSkipVerify); err != nil {
		log.Fatal(err)
	}
}

func run(listenAddr, upstream, grpcAddr, httpAddr string, tlsSkipVerify bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	b := broker.New(256)

	var opts []proxy.Option
	if tlsSkipVerify {
		opts = append(opts, proxy.WithTLSSkipVerify())
	}

	p, err := proxy.New(listenAddr, upstream, opts...)
	if err != nil {
		return fmt.Errorf("proxy: %w", err)
	}

	// gRPC server for TUI clients.
	var lc net.ListenConfig
	grpcLis, err := lc.Listen(ctx, "tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("listen grpc %s: %w", grpcAddr, err)
	}
	srv := server.New(b, p)
	go func() {
		log.Printf("gRPC server listening on %s", grpcAddr)
		if err := srv.Serve(grpcLis); err != nil {
			log.Printf("grpc serve: %v", err)
		}
	}()

	// HTTP server for web UI (optional).
	if httpAddr != "" {
		httpLis, lisErr := lc.Listen(ctx, "tcp", httpAddr)
		if lisErr != nil {
			return fmt.Errorf("listen http %s: %w", httpAddr, lisErr)
		}
		webSrv := web.New(b, p)
		go func() {
			log.Printf("HTTP server listening on %s", httpAddr)
			if err := webSrv.Serve(httpLis); err != nil {
				log.Printf("http serve: %v", err)
			}
		}()
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = webSrv.Shutdown(shutdownCtx)
		}()
	}

	go func() {
		for ev := range p.Events() {
			b.Publish(ev)
		}
	}()

	log.Printf("proxying %s -> %s", listenAddr, upstream)
	if err := p.ListenAndServe(ctx); err != nil {
		return fmt.Errorf("proxy: %w", err)
	}

	srv.GracefulStop()
	return nil
}
