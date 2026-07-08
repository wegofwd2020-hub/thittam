package iam

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/wegofwd2020/thittam/pkg/audit"
)

// isHoldableStatus reports whether a tenant's status has a running retention
// clock that an operator hold can pause or extend. 'active' has no clock;
// 'purge_eligible' is terminal.
func isHoldableStatus(status string) bool {
	switch status {
	case TenantStatusSuspended, TenantStatusGrace, TenantStatusDeactivated:
		return true
	default:
		return false
	}
}

// SetTenantRetention applies a status-preserving legal hold to a suspended
// tenant, pausing the retention sweeper (#119). A nil holdUntil is an
// indefinite pause; a future holdUntil extends the hold until that time.
// freezeReason must be non-empty (the handler enforces this). If the tenant
// already has an active hold and overwrite is false the call is rejected with
// ErrTenantHoldExists; the returned error names the existing freeze_reason.
func (s *Service) SetTenantRetention(
	ctx context.Context,
	id uuid.UUID,
	holdUntil *time.Time,
	freezeReason string,
	overwrite bool,
) (*Tenant, error) {
	before, err := s.repo.GetTenant(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("iam: set tenant retention %s: %w", id, err)
	}

	if !isHoldableStatus(before.Status) {
		return nil, fmt.Errorf("iam: set tenant retention %s (status %s): %w", id, before.Status, ErrTenantNotHoldable)
	}

	if holdUntil != nil && !holdUntil.After(time.Now().UTC()) {
		return nil, fmt.Errorf("iam: set tenant retention %s: %w", id, ErrHoldUntilInPast)
	}

	overwrote := before.FreezeReason != nil && *before.FreezeReason != ""
	if overwrote && !overwrite {
		return nil, fmt.Errorf("iam: set tenant retention %s (existing hold %q): %w", id, *before.FreezeReason, ErrTenantHoldExists)
	}

	// Guard against narrowing an indefinite hold (e.g. litigation) into a
	// dated one: after the dated hold_until passes the sweeper would resume,
	// even though the underlying reason for an indefinite hold may still
	// apply. Only blocks indefinite-source -> dated; indefinite -> indefinite
	// (reason change, holdUntil stays nil) and dated -> anything are allowed.
	existingIndefinite := before.FreezeReason != nil && *before.FreezeReason != "" && before.HoldUntil == nil
	if existingIndefinite && holdUntil != nil {
		return nil, fmt.Errorf("iam: set tenant retention %s: %w", id, ErrHoldNarrowsIndefinite)
	}

	after, err := s.repo.SetTenantLegalHold(ctx, id, holdUntil, freezeReason)
	if err != nil {
		return nil, fmt.Errorf("iam: set tenant retention %s: %w", id, err)
	}

	if s.audit != nil {
		actor, _ := audit.ActorFromContext(ctx)
		var prevReason *string
		if overwrote {
			prevReason = before.FreezeReason
		}
		s.audit.Log(audit.Event{
			TenantID:     id,
			ActorID:      actor.UserID,
			ActorEmail:   actor.Email,
			ActorIP:      actor.IP,
			Action:       audit.ActionLegalHoldApplied,
			ResourceType: audit.ResourceTenant,
			ResourceID:   id,
			OldState:     mustMarshalHoldState(before),
			NewState:     mustMarshalHoldState(after),
			Metadata:     mustMarshalRetentionMeta(overwrote, prevReason),
			OccurredAt:   time.Now().UTC(),
		})
	}

	return after, nil
}

// mustMarshalRetentionMeta encodes the overwrite context for a
// SetTenantRetention audit event. previous_reason is omitted when nothing was
// overwritten. Panics only on a marshal bug (mirrors the mustMarshal* helpers
// in lifecycle.go).
func mustMarshalRetentionMeta(overwrote bool, previousReason *string) json.RawMessage {
	b, err := json.Marshal(struct {
		OverwrotePrevious bool    `json:"overwrote_previous"`
		PreviousReason    *string `json:"previous_reason,omitempty"`
	}{overwrote, previousReason})
	if err != nil {
		panic(fmt.Sprintf("iam: marshal retention override metadata: %v", err))
	}
	return b
}
