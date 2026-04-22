package iam

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/wegofwd2020/thittam/pkg/audit"
)

// Lifecycle timings driven by the retention policy in
// thittam_docs/docs/developers/operations/tenant-onboarding.md §1.3a (#92).
const (
	// SuspensionGracePeriod is how long a tenant stays in 'suspended'
	// (no access, full data retained) before the sweeper transitions it
	// to 'grace' (read-only, same data).
	SuspensionGracePeriod = 30 * 24 * time.Hour

	// GraceToDeactivatedDuration is measured from suspended_at, not from
	// the grace transition — the policy expresses the whole budget as
	// "90 days after payment lapse".
	GraceToDeactivatedDuration = 90 * 24 * time.Hour

	// DeactivatedToPurgeDuration is the retention window after
	// deactivation before the tenant becomes eligible for hard-delete.
	DeactivatedToPurgeDuration = 180 * 24 * time.Hour
)

// SystemActorRetentionSweeper is the actor_email recorded on audit events
// emitted by the retention sweeper. ActorID stays uuid.Nil — audit queries
// that want to surface automated activity filter on this email.
const SystemActorRetentionSweeper = "system:retention-sweeper"

// Tenant status values. The string literals are DB-level constants; code
// that needs to compare or set them should import these.
const (
	TenantStatusActive        = "active"
	TenantStatusSuspended     = "suspended"
	TenantStatusGrace         = "grace"
	TenantStatusDeactivated   = "deactivated"
	TenantStatusPurgeEligible = "purge_eligible"
)

// LifecycleTransition describes a single state-machine move produced by
// AdvanceTenantLifecycle. Returned to the caller so the sweeper binary
// can emit structured logs / metrics.
type LifecycleTransition struct {
	TenantID   uuid.UUID
	TenantName string
	FromStatus string
	ToStatus   string
	OccurredAt time.Time
}

// WithAuditLogger attaches an audit.Logger so AdvanceTenantLifecycle
// writes an append-only audit event per transition (Rule #7).
// Optional: if nil, transitions still happen but no audit event is
// emitted. Returns the receiver so calls can be chained.
func (s *Service) WithAuditLogger(l *audit.Logger) *Service {
	s.audit = l
	return s
}

// AdvanceTenantLifecycle inspects the tenant's current status and
// elapsed time since suspended_at / deactivated_at, and performs at
// most one transition:
//
//	suspended    → grace           after SuspensionGracePeriod
//	grace        → deactivated     after GraceToDeactivatedDuration
//	deactivated  → purge_eligible  after DeactivatedToPurgeDuration
//
// Idempotent under concurrent callers: the repo layer uses a status-
// guarded UPDATE, so only one worker wins the transition and the
// others observe "no transition". Returns a non-nil *LifecycleTransition
// exactly when a state change was applied.
func (s *Service) AdvanceTenantLifecycle(
	ctx context.Context,
	id uuid.UUID,
	now time.Time,
) (*LifecycleTransition, error) {
	t, err := s.repo.GetTenant(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("iam: advance lifecycle %s: %w", id, err)
	}

	nextStatus, eligible := nextLifecycleStatus(t, now)
	if !eligible {
		return nil, nil
	}

	updated, ok, err := s.repo.TransitionTenantStatus(ctx, id, t.Status, nextStatus)
	if err != nil {
		return nil, fmt.Errorf("iam: transition %s → %s for %s: %w", t.Status, nextStatus, id, err)
	}
	if !ok {
		// Raced with another worker or a manual status change between
		// our read and the conditional UPDATE — treat as no-op.
		return nil, nil
	}

	transition := &LifecycleTransition{
		TenantID:   updated.ID,
		TenantName: updated.Name,
		FromStatus: t.Status,
		ToStatus:   updated.Status,
		OccurredAt: now,
	}

	if s.audit != nil {
		s.audit.Log(audit.Event{
			TenantID:     id,
			ActorID:      uuid.Nil,
			ActorEmail:   SystemActorRetentionSweeper,
			Action:       audit.ActionStatusChanged,
			ResourceType: audit.ResourceTenant,
			ResourceID:   id,
			OldState:     mustMarshalStatus(t.Status),
			NewState:     mustMarshalStatus(updated.Status),
			OccurredAt:   now,
		})
	}

	return transition, nil
}

// ListTenantsDueForLifecycle returns tenants whose next automatic
// lifecycle transition is due at or before `now`. Limit caps the
// batch; callers page by re-invoking with the same `now` until the
// returned slice is empty.
func (s *Service) ListTenantsDueForLifecycle(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]*Tenant, error) {
	if limit <= 0 {
		limit = 500
	}
	return s.repo.ListTenantsDueForLifecycle(ctx, now, limit)
}

// nextLifecycleStatus returns the status the tenant should move to
// given `now`, or (_, false) when no transition is due. Pure function
// of the tenant row + time — no side effects, trivially unit-testable.
func nextLifecycleStatus(t *Tenant, now time.Time) (string, bool) {
	switch t.Status {
	case TenantStatusSuspended:
		if t.SuspendedAt == nil {
			return "", false
		}
		if now.Sub(*t.SuspendedAt) < SuspensionGracePeriod {
			return "", false
		}
		return TenantStatusGrace, true
	case TenantStatusGrace:
		if t.SuspendedAt == nil {
			return "", false
		}
		if now.Sub(*t.SuspendedAt) < GraceToDeactivatedDuration {
			return "", false
		}
		return TenantStatusDeactivated, true
	case TenantStatusDeactivated:
		if t.DeactivatedAt == nil {
			return "", false
		}
		if now.Sub(*t.DeactivatedAt) < DeactivatedToPurgeDuration {
			return "", false
		}
		return TenantStatusPurgeEligible, true
	default:
		return "", false
	}
}

func mustMarshalStatus(status string) json.RawMessage {
	// map[string]string with a known key set cannot fail to marshal.
	b, _ := json.Marshal(map[string]string{"status": status})
	return b
}
