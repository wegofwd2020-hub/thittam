package main

// Prometheus metrics for the retention sweeper (#92 Stage 5).
//
// The sweeper is a Kubernetes CronJob — a short-lived batch job with
// no long-running HTTP listener to scrape. Metrics flow out via the
// Prometheus Pushgateway instead: we register counters/gauges on a
// local registry, update them during the sweep, then push at the
// end of the run. Pushgateway persists the last successful push
// under the job label until a newer push overwrites it, matching
// our "last successful run snapshot" semantics for dashboards.
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

// sweeperMetrics bundles all Prometheus metrics emitted by a single
// sweep run. The registry is local (not the global one) so the push
// payload contains only our metrics, not go_* / process_* defaults.
type sweeperMetrics struct {
	registry *prometheus.Registry

	runsTotal            *prometheus.CounterVec // outcome="success|error"
	durationSeconds      prometheus.Gauge
	transitionsTotal     *prometheus.CounterVec // from,to
	tenantsSeenTotal     prometheus.Counter
	failuresTotal        *prometheus.CounterVec // from,to
	oldestPendingSeconds prometheus.Gauge
	tenantsOnHold        prometheus.Gauge
	lastSuccessTimestamp prometheus.Gauge
}

// newSweeperMetrics registers all metrics on a fresh registry.
// Registration panics (duplicate metric names) are a programming
// error — we want the process to exit fast rather than push partial
// state.
func newSweeperMetrics() *sweeperMetrics {
	reg := prometheus.NewRegistry()
	m := &sweeperMetrics{
		registry: reg,

		runsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "thittam_retention_sweep_runs_total",
			Help: "Total retention-sweeper runs, labeled by outcome.",
		}, []string{"outcome"}),

		durationSeconds: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "thittam_retention_sweep_duration_seconds",
			Help: "Duration of the most recent retention sweep run.",
		}),

		transitionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "thittam_retention_transitions_total",
			Help: "Tenant lifecycle state transitions applied by the sweeper.",
		}, []string{"from", "to"}),

		tenantsSeenTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "thittam_retention_tenants_seen_total",
			Help: "Candidates examined during the most recent sweep run.",
		}),

		failuresTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "thittam_retention_failures_total",
			Help: "Tenants whose lifecycle transition returned an error.",
		}, []string{"from", "to"}),

		oldestPendingSeconds: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "thittam_retention_oldest_pending_seconds",
			Help: "How overdue the oldest examined tenant was for its next transition.",
		}),

		tenantsOnHold: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "thittam_retention_tenants_on_hold",
			Help: "Total tenants with an active legal hold (freeze_reason not null).",
		}),

		lastSuccessTimestamp: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "thittam_retention_sweep_last_success_timestamp_seconds",
			Help: "Unix timestamp of the most recent successful sweep completion.",
		}),
	}

	reg.MustRegister(
		m.runsTotal,
		m.durationSeconds,
		m.transitionsTotal,
		m.tenantsSeenTotal,
		m.failuresTotal,
		m.oldestPendingSeconds,
		m.tenantsOnHold,
		m.lastSuccessTimestamp,
	)
	return m
}

// pushMetrics pushes the registered metrics to Pushgateway. An empty
// url is treated as "push disabled" — returns silently so local-dev
// sweeps work without a Pushgateway running.
//
// A failed push is logged but does NOT bubble up as a sweep-run
// failure: the sweep succeeded from the DB's point of view, and we
// don't want to mask that outcome because observability plumbing is
// broken.
func pushMetrics(
	ctx context.Context,
	logger *slog.Logger,
	url, instance string,
	reg *prometheus.Registry,
) {
	if url == "" {
		logger.Info("pushgateway URL not set — skipping metrics push")
		return
	}
	pusher := push.New(url, "retention-sweeper").
		Gatherer(reg).
		Grouping("instance", instance)
	if err := pusher.PushContext(ctx); err != nil {
		logger.Warn("pushgateway push failed — continuing",
			"error", err,
			"url", url,
			"instance", instance,
		)
		return
	}
	logger.Info("pushed metrics to pushgateway", "url", url, "instance", instance)
}

// pushInstance returns the instance label for pushing. Prefer
// POD_NAME (injected via K8s downward API) so concurrent pods don't
// overwrite each other's pushes; fall back to the hostname for
// local-dev runs.
func pushInstance() string {
	if p := os.Getenv("POD_NAME"); p != "" {
		return p
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "unknown"
}
