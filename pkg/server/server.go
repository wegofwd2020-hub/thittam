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

	"github.com/wegofwd2020/thittam/pkg/observability"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// Config holds the server configuration.
type Config struct {
	Name        string           // service name (e.g., "project-management")
	Port        int
	MetricsPort int              // health/metrics HTTP port (default: 9090)
	Loader      *vertical.Loader // vertical config loader (nil = skip vertical interceptor)
}

// Server wraps a gRPC server with interceptors and lifecycle management.
type Server struct {
	cfg     Config
	gs      *grpc.Server
	metrics *observability.Metrics
	health  *observability.HealthServer
	logger  Logger
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
// Interceptor order (outermost first): recovery → metrics → vertical → handler.
func New(cfg Config, logger Logger) *Server {
	if logger == nil {
		logger = defaultLogger{name: cfg.Name}
	}
	if cfg.MetricsPort == 0 {
		cfg.MetricsPort = 9090
	}

	// Create metrics collectors
	metrics := observability.NewMetrics(sanitizeName(cfg.Name))

	unaryInterceptors := []grpc.UnaryServerInterceptor{
		recoveryUnaryInterceptor(logger),
		observability.UnaryMetricsInterceptor(metrics),
	}
	streamInterceptors := []grpc.StreamServerInterceptor{
		recoveryStreamInterceptor(logger),
		observability.StreamMetricsInterceptor(metrics),
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

	// Create health server
	health := observability.NewHealthServer(cfg.Name, cfg.MetricsPort)

	return &Server{cfg: cfg, gs: gs, metrics: metrics, health: health, logger: logger}
}

// sanitizeName converts service names to Prometheus-safe subsystem names.
// "project-management" → "project_management"
func sanitizeName(name string) string {
	result := make([]byte, len(name))
	for i, c := range name {
		if c == '-' {
			result[i] = '_'
		} else {
			result[i] = byte(c)
		}
	}
	return string(result)
}

// GRPCServer returns the underlying grpc.Server for service registration.
func (s *Server) GRPCServer() *grpc.Server {
	return s.gs
}

// Metrics returns the Prometheus metrics collectors for custom business metrics.
func (s *Server) Metrics() *observability.Metrics {
	return s.metrics
}

// Health returns the health server for registering dependency checkers.
func (s *Server) Health() *observability.HealthServer {
	return s.health
}

// RegisterHealthChecker adds a named health check (e.g., "postgres", "redis").
func (s *Server) RegisterHealthChecker(name string, checker observability.HealthChecker) {
	s.health.RegisterChecker(name, checker)
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

	// Start health/metrics HTTP server
	if err := s.health.Start(); err != nil {
		return fmt.Errorf("health server: %w", err)
	}

	s.logger.Info("starting",
		"service", s.cfg.Name,
		"grpc_addr", addr,
		"metrics_addr", fmt.Sprintf(":%d", s.cfg.MetricsPort),
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
