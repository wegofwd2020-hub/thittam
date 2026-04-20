package stub

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/payments"
)

var _ payments.Gateway = (*Driver)(nil) // compile-time interface check

func newIntent(idem string) payments.Intent {
	return payments.Intent{
		TenantID:       uuid.New(),
		Amount:         decimal.RequireFromString("150.00"),
		Currency:       "INR",
		Method:         payments.MethodUPI,
		IdempotencyKey: idem,
		Description:    "verify-rbac-tmp",
	}
}

func TestCreateIntent_RequiresIdempotencyKey(t *testing.T) {
	t.Parallel()
	d := New()
	in := newIntent("")
	_, err := d.CreateIntent(context.Background(), in)
	require.ErrorIs(t, err, payments.ErrIdempotencyKeyMissing)
}

func TestCreateIntent_PopulatesState(t *testing.T) {
	t.Parallel()
	d := New()
	got, err := d.CreateIntent(context.Background(), newIntent("idem-1"))
	require.NoError(t, err)
	assert.Equal(t, payments.ProviderStub, got.ProviderName)
	assert.NotEmpty(t, got.ProviderIntentID)
	assert.Equal(t, payments.StatusPending, got.Status)
	assert.False(t, got.CreatedAt.IsZero())
	assert.Equal(t, got.CreatedAt, got.UpdatedAt)
}

func TestCreateIntent_IdempotentOnRetry(t *testing.T) {
	t.Parallel()
	d := New()
	first, err := d.CreateIntent(context.Background(), newIntent("idem-same"))
	require.NoError(t, err)
	second, err := d.CreateIntent(context.Background(), newIntent("idem-same"))
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, first.ProviderIntentID, second.ProviderIntentID)
}

func TestGetIntent(t *testing.T) {
	t.Parallel()
	d := New()
	created, err := d.CreateIntent(context.Background(), newIntent("idem-get"))
	require.NoError(t, err)

	got, err := d.GetIntent(context.Background(), created.ProviderIntentID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)

	_, err = d.GetIntent(context.Background(), "stub_intent_nonexistent")
	assert.ErrorIs(t, err, payments.ErrNotFound)
}

func TestTransitionIntent(t *testing.T) {
	t.Parallel()
	d := New()
	created, err := d.CreateIntent(context.Background(), newIntent("idem-trans"))
	require.NoError(t, err)
	d.TransitionIntent(created.ProviderIntentID, payments.StatusSucceeded)

	got, err := d.GetIntent(context.Background(), created.ProviderIntentID)
	require.NoError(t, err)
	assert.Equal(t, payments.StatusSucceeded, got.Status)
	assert.True(t, got.UpdatedAt.After(created.UpdatedAt))
}

func TestCapturePayout(t *testing.T) {
	t.Parallel()
	d := New()
	p := payments.Payout{
		TenantID:       uuid.New(),
		Amount:         decimal.RequireFromString("500.00"),
		Currency:       "INR",
		Method:         payments.MethodUPI,
		IdempotencyKey: "payout-idem-1",
		RecipientToken: "tok_recipient_xyz",
	}
	got, err := d.CapturePayout(context.Background(), p)
	require.NoError(t, err)
	assert.Equal(t, payments.StatusProcessing, got.Status)
	assert.NotEmpty(t, got.ProviderPayoutID)

	// Idempotency on retry.
	again, err := d.CapturePayout(context.Background(), p)
	require.NoError(t, err)
	assert.Equal(t, got.ID, again.ID)
}

func TestHandleWebhook_ValidEvent(t *testing.T) {
	t.Parallel()
	d := New()
	body, _ := json.Marshal(payments.Event{
		ID:   "evt_123",
		Type: payments.EventIntentSucceeded,
	})
	got, err := d.HandleWebhook(context.Background(), body, "sig_ok")
	require.NoError(t, err)
	assert.Equal(t, payments.EventIntentSucceeded, got.Type)
	assert.Equal(t, payments.ProviderStub, got.ProviderName)
	assert.False(t, got.ReceivedAt.IsZero())
}

func TestHandleWebhook_EmptySignatureRejected(t *testing.T) {
	t.Parallel()
	d := New()
	body, _ := json.Marshal(payments.Event{ID: "evt_sig", Type: payments.EventIntentFailed})
	_, err := d.HandleWebhook(context.Background(), body, "")
	assert.ErrorIs(t, err, payments.ErrInvalidSignature)
}

func TestHandleWebhook_DuplicateEventRejected(t *testing.T) {
	t.Parallel()
	d := New()
	body, _ := json.Marshal(payments.Event{
		ID:   "evt_replay",
		Type: payments.EventIntentSucceeded,
	})
	_, err := d.HandleWebhook(context.Background(), body, "sig")
	require.NoError(t, err)
	_, err = d.HandleWebhook(context.Background(), body, "sig")
	assert.ErrorIs(t, err, payments.ErrDuplicateEvent)
}

func TestHandleWebhook_CustomSignatureValidator(t *testing.T) {
	t.Parallel()
	d := New()
	d.SetSignatureValidator(func(_ []byte, sig string) error {
		if sig != "expected-sig" {
			return payments.ErrInvalidSignature
		}
		return nil
	})
	body, _ := json.Marshal(payments.Event{ID: "evt_custom", Type: payments.EventIntentSucceeded})

	_, err := d.HandleWebhook(context.Background(), body, "wrong")
	assert.ErrorIs(t, err, payments.ErrInvalidSignature)

	_, err = d.HandleWebhook(context.Background(), body, "expected-sig")
	assert.NoError(t, err)
}
