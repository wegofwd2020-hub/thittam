package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	nats "github.com/nats-io/nats.go"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/events"
	"github.com/wegofwd2020/thittam/pkg/jetstream"
	"github.com/wegofwd2020/thittam/services/ledger"
)

// mockJS captures publish calls for assertion.
type mockJS struct {
	subject string
	payload []byte
	err     error
}

func (m *mockJS) Publish(subj string, data []byte, _ ...nats.PubOpt) (*nats.PubAck, error) {
	m.subject = subj
	m.payload = data
	return &nats.PubAck{}, m.err
}

var (
	fixedTenantID  = uuid.MustParse("a0000000-0000-0000-0000-000000000001")
	fixedPeriodID  = uuid.MustParse("b0000000-0000-0000-0000-000000000001")
	fixedJournalID = uuid.MustParse("c0000000-0000-0000-0000-000000000001")
	fixedTime      = time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)
)

func newJournalEntry() *ledger.JournalEntry {
	return &ledger.JournalEntry{
		ID:          fixedJournalID,
		TenantID:    fixedTenantID,
		PeriodID:    fixedPeriodID,
		EntryNumber: "JE-2026-00001",
		Narration:   "Equipment hire — Desert Shoot",
		PostedAt:    &fixedTime,
		Lines: []ledger.JournalLine{
			{DebitAmount: decimal.NewFromFloat(5000.00), CreditAmount: decimal.Zero},
			{DebitAmount: decimal.Zero, CreditAmount: decimal.NewFromFloat(5000.00)},
		},
	}
}

func TestLedgerPublisher_PublishJournalPosted_Subject(t *testing.T) {
	t.Parallel()
	js := &mockJS{}
	p := &ledgerPublisher{pub: jetstream.NewPublisher(js)}

	require.NoError(t, p.PublishJournalPosted(context.Background(), newJournalEntry()))
	assert.Equal(t, events.SubjectLedgerJournalPosted, js.subject)
}

func TestLedgerPublisher_PublishJournalPosted_Payload(t *testing.T) {
	t.Parallel()
	js := &mockJS{}
	p := &ledgerPublisher{pub: jetstream.NewPublisher(js)}

	je := newJournalEntry()
	require.NoError(t, p.PublishJournalPosted(context.Background(), je))

	var env events.Envelope
	require.NoError(t, json.Unmarshal(js.payload, &env))
	assert.Equal(t, fixedTenantID, env.TenantID)

	var pl events.LedgerJournalPostedPayload
	require.NoError(t, json.Unmarshal(env.Payload, &pl))
	assert.Equal(t, fixedJournalID.String(), pl.JournalEntryID)
	assert.Equal(t, "JE-2026-00001", pl.EntryNumber)
	assert.Equal(t, "Equipment hire — Desert Shoot", pl.Narration)
}

func TestLedgerPublisher_PublishJournalPosted_TotalsFromLines(t *testing.T) {
	t.Parallel()
	// Verify the adapter sums debit and credit amounts from the journal lines.
	js := &mockJS{}
	p := &ledgerPublisher{pub: jetstream.NewPublisher(js)}

	je := newJournalEntry() // lines: 5000 debit, 5000 credit
	require.NoError(t, p.PublishJournalPosted(context.Background(), je))

	var env events.Envelope
	require.NoError(t, json.Unmarshal(js.payload, &env))
	var pl events.LedgerJournalPostedPayload
	require.NoError(t, json.Unmarshal(env.Payload, &pl))
	assert.Equal(t, "5000.00", pl.TotalDebit)
	assert.Equal(t, "5000.00", pl.TotalCredit)
}

func TestLedgerPublisher_PublishJournalPosted_NilPostedAt_UsesZeroTime(t *testing.T) {
	t.Parallel()
	// When PostedAt is nil the adapter falls back to zero time — no panic.
	js := &mockJS{}
	p := &ledgerPublisher{pub: jetstream.NewPublisher(js)}

	je := newJournalEntry()
	je.PostedAt = nil
	assert.NoError(t, p.PublishJournalPosted(context.Background(), je))
}

func TestNatsChecker_Unhealthy(t *testing.T) {
	t.Parallel()
	checker := &natsChecker{nc: &nats.Conn{}}
	assert.Error(t, checker.CheckHealth(context.Background()))
}

func TestRequireenv_ReturnsValue(t *testing.T) {
	// t.Setenv is incompatible with t.Parallel — must run sequentially.
	t.Setenv("TEST_LEDGER_VAR", "ok")
	assert.Equal(t, "ok", requireenv("TEST_LEDGER_VAR"))
}
