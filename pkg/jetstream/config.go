// Package jetstream defines the canonical NATS JetStream stream and consumer
// configurations for Thittam.
//
// # Stream topology
//
//	EVENTS          thittam.>                     — all domain events (full history)
//	FINANCIAL       thittam.budget.> thittam.expense.> thittam.ledger.>   — financial subjects only (7-day retention for fast replay)
//	FINANCIAL_DLQ   $JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.FINANCIAL.> — dead-letter advisory stream
//
// # Dead-letter strategy
//
// NATS JetStream does not have a first-class DLQ concept. Instead:
//  1. Each consumer has MaxDeliver=5 and an exponential BackOff policy.
//  2. When NATS exhausts all delivery attempts it publishes a
//     MaxDeliveries advisory to $JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.<stream>.<consumer>.
//  3. The FINANCIAL_DLQ stream captures these advisories so they are not lost.
//  4. An ops runbook (docs/operations/nats-dlq.md) describes how to replay
//     the original message using the advisory's StreamSeq field.
//
// # Backoff schedule (per consumer)
//
//	Attempt 1 — immediate
//	Attempt 2 — 5 s
//	Attempt 3 — 30 s
//	Attempt 4 — 5 min
//	Attempt 5 — 30 min  →  advisory published, message moves to DLQ stream
//
// # Ack timeout
//
// AckWait is 30 s — the consumer must Ack/Nak within 30 s of delivery or the
// message is counted as a redelivery. This matches the ProjectionLagSLA in
// services/reporting/consumer.go.
package jetstream

import "time"

// StreamName constants — single source of truth for stream provisioning and
// Prometheus alert labels.
const (
	StreamEvents       = "EVENTS"
	StreamFinancial    = "FINANCIAL"
	StreamFinancialDLQ = "FINANCIAL_DLQ"
)

// ConsumerName constants — durable consumer names per subscribing service.
const (
	ConsumerReportingFinancial    = "reporting-financial"
	ConsumerNotificationsFinancial = "notifications-financial"
)

// FinancialSubjects are the NATS subjects that carry money-critical events.
// Any event on these subjects that cannot be processed after MaxDeliverAttempts
// is promoted to the dead-letter stream.
var FinancialSubjects = []string{
	"thittam.budget.>",
	"thittam.expense.>",
	"thittam.ledger.>",
}

// MaxDeliverAttempts is the number of times NATS will attempt to deliver a
// message before publishing a MaxDeliveries advisory to the DLQ stream.
const MaxDeliverAttempts = 5

// AckWait is how long a consumer has to acknowledge (or nack) a message before
// NATS counts the delivery as failed and retries. Matches ProjectionLagSLA.
const AckWait = 30 * time.Second

// DeliveryBackOff is the per-attempt wait before redelivery. The final entry
// is repeated for all subsequent attempts beyond len(DeliveryBackOff).
var DeliveryBackOff = []time.Duration{
	5 * time.Second,  // attempt 2
	30 * time.Second, // attempt 3
	5 * time.Minute,  // attempt 4
	30 * time.Minute, // attempt 5 → DLQ advisory after this
}

// DLQAdvisorySubjectPattern is the NATS subject pattern NATS publishes
// MaxDeliveries advisories to. The FINANCIAL_DLQ stream subscribes to this.
const DLQAdvisorySubjectPattern = "$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.FINANCIAL.>"

// StreamConfig describes a JetStream stream for provisioning.
type StreamConfig struct {
	// Name is the stream name.
	Name string

	// Subjects is the list of NATS subjects this stream captures.
	Subjects []string

	// MaxAge is the maximum age of a message before it is automatically deleted.
	// 0 means unlimited.
	MaxAge time.Duration

	// MaxBytes is the maximum total stream size in bytes. -1 means unlimited.
	MaxBytes int64

	// Replicas is the number of stream replicas. Use 1 for local dev,
	// 3 for production (requires a 3-node NATS cluster).
	Replicas int

	// Description is a human-readable description of the stream's purpose.
	Description string
}

// ConsumerConfig describes a durable push consumer for provisioning.
type ConsumerConfig struct {
	// StreamName is the stream this consumer belongs to.
	StreamName string

	// DurableName is the stable consumer name. Survives server restarts.
	DurableName string

	// FilterSubjects restricts which subjects this consumer receives.
	// Empty means the consumer receives all subjects in the stream.
	FilterSubjects []string

	// MaxDeliver is the maximum delivery attempts before a MaxDeliveries
	// advisory is published. Must equal MaxDeliverAttempts.
	MaxDeliver int

	// AckWait is the ack timeout per delivery attempt.
	AckWait time.Duration

	// BackOff is the per-attempt wait schedule before redelivery.
	BackOff []time.Duration

	// Description is a human-readable description.
	Description string
}

// FinancialStreams returns the canonical stream configs for the FINANCIAL and
// FINANCIAL_DLQ streams. Call this during service startup or infra provisioning.
func FinancialStreams() []StreamConfig {
	return []StreamConfig{
		{
			Name:        StreamFinancial,
			Subjects:    FinancialSubjects,
			MaxAge:      7 * 24 * time.Hour, // 7 days — sufficient for replay
			MaxBytes:    -1,
			Replicas:    1, // override to 3 in production
			Description: "Financial domain events (budget, expense, ledger). DLQ-enabled via FINANCIAL_DLQ.",
		},
		{
			Name: StreamFinancialDLQ,
			Subjects: []string{
				DLQAdvisorySubjectPattern,
			},
			MaxAge:      7 * 24 * time.Hour, // keep DLQ entries for 7 days for ops investigation
			MaxBytes:    -1,
			Replicas:    1,
			Description: "Dead-letter advisory stream: captures NATS MaxDeliveries advisories for FINANCIAL consumers.",
		},
	}
}

// AllStreams returns all stream configs (EVENTS + financial + DLQ).
func AllStreams() []StreamConfig {
	all := []StreamConfig{
		{
			Name:        StreamEvents,
			Subjects:    []string{"thittam.>"},
			MaxAge:      24 * time.Hour, // full event log; 24 h is sufficient for replay
			MaxBytes:    -1,
			Replicas:    1,
			Description: "All Thittam domain events. Reporting-analytics and notifications subscribe here.",
		},
	}
	return append(all, FinancialStreams()...)
}

// FinancialConsumers returns the durable consumer configs for all services that
// consume financial events. Each consumer has MaxDeliver + BackOff configured
// so exhausted messages trigger DLQ advisories.
func FinancialConsumers() []ConsumerConfig {
	return []ConsumerConfig{
		{
			StreamName:     StreamFinancial,
			DurableName:    ConsumerReportingFinancial,
			FilterSubjects: FinancialSubjects,
			MaxDeliver:     MaxDeliverAttempts,
			AckWait:        AckWait,
			BackOff:        DeliveryBackOff,
			Description:    "Reporting-analytics service financial event consumer. Powers expense/budget projection tables.",
		},
		{
			StreamName:     StreamFinancial,
			DurableName:    ConsumerNotificationsFinancial,
			FilterSubjects: FinancialSubjects,
			MaxDeliver:     MaxDeliverAttempts,
			AckWait:        AckWait,
			BackOff:        DeliveryBackOff,
			Description:    "Notifications service financial event consumer. Triggers approval-request and over-budget alerts.",
		},
	}
}
