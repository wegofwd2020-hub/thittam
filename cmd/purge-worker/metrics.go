package main

// Prometheus metrics for the purge worker (#92 Stage 5).
//
// Mirrors cmd/retention-sweeper/metrics.go: the worker is a Kubernetes
// CronJob — a short-lived batch job with no long-running HTTP listener to
// scrape. Metrics flow out via the Prometheus Pushgateway instead: we
// register counters/gauges on a local registry, update them during the run,
// then push at the end. Pushgateway persists the last successful push under
// the job label until a newer push overwrites it, matching our "last
// successful run snapshot" semantics for dashboards.
//
// Naming follows the repo convention (`thittam_<subsystem>_<metric>`)
// established in pkg/observability/metrics.go.

import (
	"context"
	"log/slog"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
)

// purgeMetrics bundles all Prometheus metrics emitted by a single purge run.
// The registry is local (not the global one) so the push payload contains
// only our metrics, not go_* / process_* defaults.
type purgeMetrics struct {
	registry *prometheus.Registry

	runsTotal            *prometheus.CounterVec // outcome=success|error
	purgedTotal          prometheus.Counter
	failuresTotal        prometheus.Counter
	lastSuccessTimestamp prometheus.Gauge
}

// newPurgeMetrics registers all metrics on a fresh registry. Registration
// panics (duplicate metric names) are a programming error — we want the
// process to exit fast rather than push partial state.
func newPurgeMetrics() *purgeMetrics {
	reg := prometheus.NewRegistry()
	m := &purgeMetrics{
		registry: reg,
		runsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "thittam_purge_worker_runs_total",
			Help: "Total purge-worker runs, labeled by outcome.",
		}, []string{"outcome"}),
		purgedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "thittam_purge_worker_purged_total",
			Help: "Tenants hard-deleted by the purge worker.",
		}),
		failuresTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "thittam_purge_worker_failures_total",
			Help: "Approved purge requests that failed to execute.",
		}),
		lastSuccessTimestamp: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "thittam_purge_worker_last_success_timestamp_seconds",
			Help: "Unix timestamp of the most recent successful purge run.",
		}),
	}
	reg.MustRegister(m.runsTotal, m.purgedTotal, m.failuresTotal, m.lastSuccessTimestamp)
	return m
}

// pushMetrics pushes the registered metrics to Pushgateway. An empty url is
// treated as "push disabled" — returns silently so local-dev runs work
// without a Pushgateway running.
//
// A failed push is logged but does NOT bubble up as a run failure: the run
// succeeded from the DB's point of view, and we don't want to mask that
// outcome because observability plumbing is broken.
func pushMetrics(ctx context.Context, logger *slog.Logger, url, instance string, reg *prometheus.Registry) {
	if url == "" {
		logger.Info("pushgateway URL not set — skipping metrics push")
		return
	}
	pusher := push.New(url, "purge-worker").Gatherer(reg).Grouping("instance", instance)
	if err := pusher.PushContext(ctx); err != nil {
		logger.Warn("pushgateway push failed — continuing", "error", err, "url", url, "instance", instance)
		return
	}
	logger.Info("pushed metrics to pushgateway", "url", url, "instance", instance)
}

// pushInstance returns the instance label for pushing. Prefer POD_NAME
// (injected via K8s downward API) so concurrent pods don't overwrite each
// other's pushes; fall back to the hostname for local-dev runs.
func pushInstance() string {
	if p := os.Getenv("POD_NAME"); p != "" {
		return p
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "unknown"
}
