// Package server provides a shared gRPC server factory for all Thittam services.
// It configures the interceptor chain (recovery → vertical → auth) and handles
// graceful shutdown.
package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/wegofwd2020/thittam/pkg/vertical"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// Config holds the server configuration.
type Config struct {
	Name   string // service name (e.g., "project-management")
	Port   int
	Loader *vertical.Loader // vertical config loader (nil = skip vertical interceptor)
}

// Server wraps a gRPC server with interceptors and lifecycle management.
type Server struct {
	cfg    Config
	gs     *grpc.Server
	logger Logger
}

// Logger defines structured logging.
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
}

type defaultLogger struct{ name string }

func (l defaultLogger) Info(msg string, kv ...interface{})  { log.Printf("[INFO] %s: %s %v", l.name, msg, kv) }
func (l defaultLogger) Error(msg string, kv ...interface{}) { log.Printf("[ERROR] %s: %s %v", l.name, msg, kv) }

// New creates a gRPC server with the standard interceptor chain.
// Interceptor order (outermost first): recovery → vertical → (custom).
func New(cfg Config, logger Logger) *Server {
	if logger == nil {
		logger = defaultLogger{name: cfg.Name}
	}

	unaryInterceptors := []grpc.UnaryServerInterceptor{
		recoveryUnaryInterceptor(logger),
	}
	streamInterceptors := []grpc.StreamServerInterceptor{
		recoveryStreamInterceptor(logger),
	}

	if cfg.Loader != nil {
		unaryInterceptors = append(unaryInterceptors, vertical.UnaryInterceptor(cfg.Loader))
		streamInterceptors = append(streamInterceptors, vertical.StreamInterceptor(cfg.Loader))
	}

	gs := grpc.NewServer(
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	)

	reflection.Register(gs)

	return &Server{cfg: cfg, gs: gs, logger: logger}
}

// GRPCServer returns the underlying grpc.Server for service registration.
func (s *Server) GRPCServer() *grpc.Server {
	return s.gs
}

// Run starts the gRPC server and blocks until interrupted.
func (s *Server) Run() error {
	addr := fmt.Sprintf(":%d", s.cfg.Port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	// Graceful shutdown on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		s.logger.Info("shutting down", "service", s.cfg.Name)
		s.gs.GracefulStop()
	}()

	s.logger.Info("starting",
		"service", s.cfg.Name,
		"addr", addr,
	)

	return s.gs.Serve(lis)
}

// recoveryUnaryInterceptor catches panics in handlers and returns Internal errors.
func recoveryUnaryInterceptor(logger Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered",
					"method", info.FullMethod,
					"panic", fmt.Sprintf("%v", r),
					"stack", string(debug.Stack()),
				)
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}

// recoveryStreamInterceptor catches panics in streaming handlers.
func recoveryStreamInterceptor(logger Logger) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered",
					"method", info.FullMethod,
					"panic", fmt.Sprintf("%v", r),
					"stack", string(debug.Stack()),
				)
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(srv, ss)
	}
}
