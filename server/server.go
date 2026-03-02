package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/mickamy/http-tap/broker"
	tapv1 "github.com/mickamy/http-tap/gen/tap/v1"
	"github.com/mickamy/http-tap/proxy"
)

// Server exposes a gRPC TapService for TUI clients to connect to.
type Server struct {
	grpcServer *grpc.Server
}

// New creates a new Server backed by the given Broker and Proxy.
func New(b *broker.Broker, p proxy.Proxy) *Server {
	gs := grpc.NewServer()
	svc := &tapService{broker: b, proxy: p}
	tapv1.RegisterTapServiceServer(gs, svc)

	return &Server{grpcServer: gs}
}

// Serve starts the gRPC server on the given listener.
func (s *Server) Serve(lis net.Listener) error {
	if err := s.grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("server: serve: %w", err)
	}
	return nil
}

// Stop immediately stops the server.
func (s *Server) Stop() {
	s.grpcServer.Stop()
}

// GracefulStop gracefully stops the server.
func (s *Server) GracefulStop() {
	s.grpcServer.GracefulStop()
}

type tapService struct {
	tapv1.UnimplementedTapServiceServer

	broker *broker.Broker
	proxy  proxy.Proxy
}

func (s *tapService) Watch(_ *tapv1.WatchRequest, stream grpc.ServerStreamingServer[tapv1.WatchResponse]) error {
	ch, unsub := s.broker.Subscribe()
	defer unsub()

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("server: watch: %w", ctx.Err())
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(&tapv1.WatchResponse{
				Event: eventToProto(ev),
			}); err != nil {
				return fmt.Errorf("server: watch send: %w", err)
			}
		}
	}
}

func (s *tapService) Replay(ctx context.Context, req *tapv1.ReplayRequest) (*tapv1.ReplayResponse, error) {
	headers := unflattenHeaders(req.GetHeaders())

	ev, err := s.proxy.Replay(ctx, req.GetMethod(), req.GetPath(), headers, req.GetBody())
	if err != nil {
		return nil, fmt.Errorf("server: replay: %w", err)
	}
	return &tapv1.ReplayResponse{
		Event: eventToProto(ev),
	}, nil
}

func eventToProto(ev proxy.Event) *tapv1.HTTPEvent {
	return &tapv1.HTTPEvent{
		Id:              ev.ID,
		Method:          ev.Method,
		Path:            ev.Path,
		Status:          ev.Status,
		StartTime:       timestamppb.New(ev.StartTime),
		Duration:        durationpb.New(ev.Duration),
		Error:           ev.Error,
		RequestHeaders:  flattenHeaders(ev.RequestHeaders),
		ResponseHeaders: flattenHeaders(ev.ResponseHeaders),
		RequestBody:     ev.RequestBody,
		ResponseBody:    ev.ResponseBody,
	}
}

// flattenHeaders converts http.Header (multi-value) to map[string]string
// by joining multiple values with ", ".
func flattenHeaders(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	m := make(map[string]string, len(h))
	for k, vs := range h {
		m[k] = strings.Join(vs, ", ")
	}
	return m
}

// unflattenHeaders converts map[string]string to http.Header.
func unflattenHeaders(m map[string]string) http.Header {
	if len(m) == 0 {
		return nil
	}
	h := make(http.Header, len(m))
	for k, v := range m {
		h.Set(k, v)
	}
	return h
}
