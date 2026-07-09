package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests use t.Setenv, which is incompatible with t.Parallel() — do not
// add t.Parallel() here. They live in package server (not server_test)
// because they inspect the unexported s.gs field via GetServiceInfo().

// Config.Name must differ between tests: observability.NewMetrics registers
// Prometheus collectors on the global default registry keyed by
// namespace+subsystem+name, and calling New() twice with the same Name
// panics with "duplicate metrics collector registration attempted".
func TestNew_ReflectionOffByDefault(t *testing.T) {
	t.Setenv("GRPC_REFLECTION", "")
	s := New(Config{Name: "t-reflection-off", Port: 0, MetricsPort: 0}, nil)
	require.NotNil(t, s)
	_, ok := s.gs.GetServiceInfo()["grpc.reflection.v1.ServerReflection"]
	assert.False(t, ok, "reflection must not be registered by default")
}

func TestNew_ReflectionOnViaEnv(t *testing.T) {
	t.Setenv("GRPC_REFLECTION", "1")
	s := New(Config{Name: "t-reflection-on", Port: 0, MetricsPort: 0}, nil)
	_, ok := s.gs.GetServiceInfo()["grpc.reflection.v1.ServerReflection"]
	assert.True(t, ok)
}
