package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

// newDiscardLogger returns a slog.Logger that writes to io.Discard so tests
// don't spam stdout with "pushgateway push failed" noise.
func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

// TestPurgeMetrics_CountersRegistered mirrors the sweeper's
// TestNewSweeperMetrics_CounterVecLabelsAreValid: exercise every metric path
// so a typo in a name or label arity fails loudly at test time rather than
// during a 02:45 UTC production run.
//
// runPurge itself can't be unit-tested against a fake service — svc is the
// concrete *iam.Service, not an interface — so loop correctness (continue on
// per-request failure, abort on systemic list error) is covered by
// services/iam's PurgeApprovedTenant / ListApprovedPurges tests.
func TestPurgeMetrics_CountersRegistered(t *testing.T) {
	t.Parallel()
	m := newPurgeMetrics()
	m.purgedTotal.Inc()
	m.failuresTotal.Inc()
	m.runsTotal.WithLabelValues("success").Inc()
	m.runsTotal.WithLabelValues("error").Inc()

	if got := testutil.CollectAndCount(m.registry); got == 0 {
		t.Fatal("expected metrics registered on the registry")
	}
	if err := testutil.CollectAndCompare(m.purgedTotal, strings.NewReader(`
# HELP thittam_purge_worker_purged_total Tenants hard-deleted by the purge worker.
# TYPE thittam_purge_worker_purged_total counter
thittam_purge_worker_purged_total 1
`)); err != nil {
		t.Fatalf("purged_total mismatch: %v", err)
	}
}

func TestPushMetrics_EmptyURLSkipsSilently(t *testing.T) {
	t.Parallel()
	// No Pushgateway process running, no URL configured — should just
	// return without error. This is the local-dev happy path.
	m := newPurgeMetrics()
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

	m := newPurgeMetrics()
	m.runsTotal.WithLabelValues("success").Inc()

	pushMetrics(context.Background(), newDiscardLogger(), srv.URL, "host-under-test", m.registry)

	// Pushgateway URL layout: /metrics/job/<job>/<label>/<value>/...
	assert.Contains(t, gotPath, "/metrics/job/purge-worker")
	assert.Contains(t, gotPath, "/instance/host-under-test",
		"instance grouping label must be in the URL path")
	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Contains(t, gotBody, "thittam_purge_worker_runs_total",
		"payload should include at least one of our metrics")
}

func TestPushMetrics_ServerErrorLoggedButNotReturned(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := newPurgeMetrics()
	pushMetrics(context.Background(), newDiscardLogger(), srv.URL, "host-1", m.registry)
}

func TestPushInstance_FallsBackToHostname(t *testing.T) {
	// Not t.Parallel() — uses t.Setenv.
	t.Setenv("POD_NAME", "")
	got := pushInstance()
	assert.NotEmpty(t, got)
	assert.NotEqual(t, "unknown", got, "hostname should be resolvable in a CI runner")
}

func TestPushInstance_PrefersPodName(t *testing.T) {
	// Not t.Parallel() — uses t.Setenv.
	t.Setenv("POD_NAME", "purge-worker-28934932-abcde")
	got := pushInstance()
	assert.Equal(t, "purge-worker-28934932-abcde", got)
}
