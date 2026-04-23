package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDiscardLogger returns a slog.Logger that writes to io.Discard so
// tests don't spam stdout with "pushgateway push failed" noise.
func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestNewSweeperMetrics_CounterVecLabelsAreValid(t *testing.T) {
	t.Parallel()
	m := newSweeperMetrics()

	// Exercise every labeled CounterVec path so that a typo in label
	// names or arities fails loudly at test time rather than during a
	// 02:15 UTC production run.
	m.runsTotal.WithLabelValues("success").Inc()
	m.runsTotal.WithLabelValues("error").Inc()
	m.transitionsTotal.WithLabelValues("suspended", "grace").Inc()
	m.transitionsTotal.WithLabelValues("grace", "deactivated").Inc()
	m.transitionsTotal.WithLabelValues("deactivated", "purge_eligible").Inc()
	m.failuresTotal.WithLabelValues("suspended", "grace").Inc()
	m.failuresTotal.WithLabelValues("unknown", "unknown").Inc()
}

func TestPushMetrics_EmptyURLSkipsSilently(t *testing.T) {
	t.Parallel()
	// No Pushgateway process running, no URL configured — should just
	// return without error. This is the local-dev happy path.
	m := newSweeperMetrics()
	pushMetrics(context.Background(), newDiscardLogger(), "", "host-1", m.registry)
}

func TestPushMetrics_SendsToServer(t *testing.T) {
	t.Parallel()

	var gotPath, gotMethod, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := newSweeperMetrics()
	// Drop a value so the payload is non-empty (Pushgateway accepts
	// empty pushes but they're not useful to inspect here).
	m.runsTotal.WithLabelValues("success").Inc()

	pushMetrics(context.Background(), newDiscardLogger(), srv.URL, "host-under-test", m.registry)

	// Pushgateway URL layout: /metrics/job/<job>/<label>/<value>/...
	assert.Contains(t, gotPath, "/metrics/job/retention-sweeper")
	assert.Contains(t, gotPath, "/instance/host-under-test",
		"instance grouping label must be in the URL path")
	// PushContext uses PUT by default (replace all metrics under group).
	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Contains(t, gotBody, "thittam_retention_sweep_runs_total",
		"payload should include at least one of our metrics")
}

func TestPushMetrics_ServerErrorLoggedButNotReturned(t *testing.T) {
	t.Parallel()

	// Pushgateway returning 500 must not propagate as a sweep failure —
	// observability plumbing errors don't invalidate the sweep result.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := newSweeperMetrics()
	// Does not panic, does not return anything — just logs and moves on.
	pushMetrics(context.Background(), newDiscardLogger(), srv.URL, "host-1", m.registry)
}

func TestPushMetrics_UnreachableURLHandledGracefully(t *testing.T) {
	t.Parallel()

	// Connection refused (nothing listening) — same "log + continue"
	// contract as a 500 response. Connection errors are the common
	// case when Pushgateway hasn't been deployed yet.
	m := newSweeperMetrics()
	pushMetrics(
		context.Background(),
		newDiscardLogger(),
		"http://127.0.0.1:1",
		"host-1",
		m.registry,
	)
}

func TestPushInstance_FallsBackToHostname(t *testing.T) {
	// Not t.Parallel() — uses t.Setenv.
	// POD_NAME unset — we fall back to os.Hostname(), which always
	// returns *something* on Linux/macOS/Windows. Just assert we got
	// a non-empty string rather than pinning a specific value.
	t.Setenv("POD_NAME", "")
	got := pushInstance()
	assert.NotEmpty(t, got)
	assert.NotEqual(t, "unknown", got, "hostname should be resolvable in a CI runner")
}

func TestPushInstance_PrefersPodName(t *testing.T) {
	// Not t.Parallel() — uses t.Setenv.
	t.Setenv("POD_NAME", "retention-sweeper-28934932-abcde")
	got := pushInstance()
	assert.Equal(t, "retention-sweeper-28934932-abcde", got)
}

// --- Helpers used by the runSweep-side tests below ---

// gatherText pulls the current registry state and formats it as the
// text exposition format. Handy for whole-payload assertions without
// digging through protobuf structures.
func gatherText(t *testing.T, reg *prometheus.Registry) string {
	t.Helper()
	got, err := reg.Gather()
	require.NoError(t, err)

	var b strings.Builder
	for _, mf := range got {
		b.WriteString(mf.GetName())
		b.WriteString("\n")
	}
	return b.String()
}

func TestNewSweeperMetrics_InitialStateIsEmptyButRegistered(t *testing.T) {
	t.Parallel()
	// Before any Inc() / Set(), the registry should still list every
	// metric (via its descriptor) so Prometheus scrapes see them as 0
	// rather than as "missing". Without this, first-run dashboards
	// would show "No data" panels until the first successful sweep.
	m := newSweeperMetrics()

	// Nudge each metric once so Gather() actually surfaces it — a
	// zero-value CounterVec with no observed labels returns 0 metric
	// families. We're sanity-checking the names are spelled right.
	m.runsTotal.WithLabelValues("success").Add(0)
	m.tenantsSeenTotal.Add(0)
	m.durationSeconds.Set(0)
	m.oldestPendingSeconds.Set(0)
	m.tenantsOnHold.Set(0)
	m.lastSuccessTimestamp.Set(0)
	m.transitionsTotal.WithLabelValues("suspended", "grace").Add(0)
	m.failuresTotal.WithLabelValues("suspended", "grace").Add(0)

	text := gatherText(t, m.registry)
	for _, name := range []string{
		"thittam_retention_sweep_runs_total",
		"thittam_retention_sweep_duration_seconds",
		"thittam_retention_transitions_total",
		"thittam_retention_tenants_seen_total",
		"thittam_retention_failures_total",
		"thittam_retention_oldest_pending_seconds",
		"thittam_retention_tenants_on_hold",
		"thittam_retention_sweep_last_success_timestamp_seconds",
	} {
		assert.Contains(t, text, name)
	}
}
