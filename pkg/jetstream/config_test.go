package jetstream_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/jetstream"
)

func TestAllStreams_NamesAreUnique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool)
	for _, s := range jetstream.AllStreams() {
		require.False(t, seen[s.Name], "duplicate stream name: %s", s.Name)
		seen[s.Name] = true
	}
}

func TestAllStreams_EveryStreamHasSubjects(t *testing.T) {
	t.Parallel()
	for _, s := range jetstream.AllStreams() {
		assert.NotEmpty(t, s.Subjects, "stream %s has no subjects", s.Name)
		for _, sub := range s.Subjects {
			assert.NotEmpty(t, sub, "stream %s has an empty subject", s.Name)
		}
	}
}

func TestFinancialStreams_IncludeDLQ(t *testing.T) {
	t.Parallel()
	streams := jetstream.FinancialStreams()
	names := make([]string, len(streams))
	for i, s := range streams {
		names[i] = s.Name
	}
	assert.Contains(t, names, jetstream.StreamFinancial)
	assert.Contains(t, names, jetstream.StreamFinancialDLQ)
}

func TestFinancialDLQStream_SubscribesToAdvisory(t *testing.T) {
	t.Parallel()
	var dlq *jetstream.StreamConfig
	for _, s := range jetstream.FinancialStreams() {
		if s.Name == jetstream.StreamFinancialDLQ {
			copy := s
			dlq = &copy
			break
		}
	}
	require.NotNil(t, dlq, "FINANCIAL_DLQ stream must be configured")

	// DLQ must subscribe to the NATS MaxDeliveries advisory subject.
	found := false
	for _, sub := range dlq.Subjects {
		if strings.HasPrefix(sub, "$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES") {
			found = true
			break
		}
	}
	assert.True(t, found, "FINANCIAL_DLQ stream must capture MaxDeliveries advisories, got: %v", dlq.Subjects)
}

func TestFinancialDLQStream_RetainsMessagesForSevenDays(t *testing.T) {
	t.Parallel()
	for _, s := range jetstream.FinancialStreams() {
		if s.Name == jetstream.StreamFinancialDLQ {
			assert.GreaterOrEqual(t, s.MaxAge, 7*24*time.Hour,
				"DLQ must retain messages for at least 7 days for ops investigation")
			return
		}
	}
	t.Fatal("FINANCIAL_DLQ stream not found")
}

func TestFinancialConsumers_AllHaveMaxDeliver(t *testing.T) {
	t.Parallel()
	consumers := jetstream.FinancialConsumers()
	require.NotEmpty(t, consumers, "must define at least one financial consumer")

	for _, c := range consumers {
		assert.Equal(t, jetstream.MaxDeliverAttempts, c.MaxDeliver,
			"consumer %s MaxDeliver must equal MaxDeliverAttempts", c.DurableName)
		assert.Equal(t, jetstream.AckWait, c.AckWait,
			"consumer %s AckWait must equal the canonical AckWait", c.DurableName)
		assert.NotEmpty(t, c.BackOff,
			"consumer %s must define a BackOff schedule", c.DurableName)
	}
}

func TestFinancialConsumers_BackOffIsMonotonicallyIncreasing(t *testing.T) {
	t.Parallel()
	for _, c := range jetstream.FinancialConsumers() {
		for i := 1; i < len(c.BackOff); i++ {
			assert.GreaterOrEqual(t, c.BackOff[i], c.BackOff[i-1],
				"consumer %s backoff[%d] < backoff[%d] — backoff must not decrease",
				c.DurableName, i, i-1)
		}
	}
}

func TestFinancialConsumers_NamesAreUnique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool)
	for _, c := range jetstream.FinancialConsumers() {
		key := c.StreamName + "/" + c.DurableName
		require.False(t, seen[key], "duplicate consumer: %s", key)
		seen[key] = true
	}
}

func TestFinancialConsumers_FilterSubjectsAreFinancial(t *testing.T) {
	t.Parallel()
	for _, c := range jetstream.FinancialConsumers() {
		for _, sub := range c.FilterSubjects {
			isFinancial := strings.HasPrefix(sub, "thittam.budget.") ||
				strings.HasPrefix(sub, "thittam.expense.") ||
				strings.HasPrefix(sub, "thittam.ledger.")
			assert.True(t, isFinancial,
				"consumer %s has non-financial filter subject: %s", c.DurableName, sub)
		}
	}
}

func TestMaxDeliverAttempts_IsInRecommendedRange(t *testing.T) {
	t.Parallel()
	assert.GreaterOrEqual(t, jetstream.MaxDeliverAttempts, 3,
		"MaxDeliverAttempts below recommended minimum of 3")
	assert.LessOrEqual(t, jetstream.MaxDeliverAttempts, 5,
		"MaxDeliverAttempts above recommended maximum of 5")
}

func TestAckWait_MatchesProjectionLagSLA(t *testing.T) {
	t.Parallel()
	// AckWait must equal the ProjectionLagSLA (30 s) so a consumer that times out
	// on one message does not exceed the lag SLA on its own.
	assert.Equal(t, 30*time.Second, jetstream.AckWait,
		"AckWait must equal ProjectionLagSLA (30 s)")
}
