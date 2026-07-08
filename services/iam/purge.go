package iam

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/wegofwd2020/thittam/pkg/audit"
)

// SystemActorPurgeWorker is the audit actor_email recorded on audit events
// emitted by the purge-worker (mirrors SystemActorRetentionSweeper).
const SystemActorPurgeWorker = "system:purge-worker"

// RequestTenantPurge opens a pending two-person purge request for a
// purge_eligible tenant (#92 Stage 3). The caller is recorded as
// requested_by; the tenant's current name/slug are snapshotted onto the
// request for forensic purposes (the live row is tombstoned on execution).
//
// reason must be non-empty; the handler enforces this (mirrors
// SetTenantRetention's freezeReason contract).
//
// If an open (pending or approved) request already exists for the tenant,
// the repository rejects the insert with ErrPurgeRequestExists, which is
// propagated unwrapped-by-errors.Is (via %w).
func (s *Service) RequestTenantPurge(ctx context.Context, tenantID uuid.UUID, reason string) (*TenantPurgeRequest, error) {
	tenant, err := s.repo.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("iam: request tenant purge %s: %w", tenantID, err)
	}
	if tenant.Status != TenantStatusPurgeEligible {
		return nil, fmt.Errorf("iam: request tenant purge %s (status %s): %w", tenantID, tenant.Status, ErrTenantNotPurgeable)
	}

	actor, _ := audit.ActorFromContext(ctx)
	req := &TenantPurgeRequest{
		ID:            uuid.New(),
		TenantID:      tenantID,
		Status:        PurgeRequestPending,
		RequestedBy:   actor.UserID,
		RequestReason: reason,
		TenantName:    tenant.Name,
		TenantSlug:    tenant.Slug,
	}
	if err := s.repo.CreateTenantPurgeRequest(ctx, req); err != nil {
		return nil, fmt.Errorf("iam: request tenant purge %s: %w", tenantID, err)
	}

	s.auditPurge(ctx, audit.ActionPurgeRequested, tenantID, actor, reason)
	return req, nil
}

// ApproveTenantPurge approves the open pending request for a tenant. The
// approver MUST differ from the requester (two-person control) and the
// tenant must still be purge_eligible at approval time — this re-check
// guards against the tenant being reactivated or otherwise leaving the
// purge-eligible state between request and approval. No deletion happens
// here; the purge-worker executes approved requests asynchronously via
// PurgeApprovedTenant.
func (s *Service) ApproveTenantPurge(ctx context.Context, tenantID uuid.UUID, reason string) (*TenantPurgeRequest, error) {
	open, err := s.repo.GetOpenTenantPurgeRequest(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("iam: approve tenant purge %s: %w", tenantID, err)
	}
	if open.Status != PurgeRequestPending {
		return nil, fmt.Errorf("iam: approve tenant purge %s: %w", tenantID, ErrPurgeRequestNotFound)
	}

	actor, _ := audit.ActorFromContext(ctx)
	if actor.UserID == open.RequestedBy {
		return nil, fmt.Errorf("iam: approve tenant purge %s: %w", tenantID, ErrSelfApproval)
	}

	tenant, err := s.repo.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("iam: approve tenant purge %s: %w", tenantID, err)
	}
	if tenant.Status != TenantStatusPurgeEligible {
		return nil, fmt.Errorf("iam: approve tenant purge %s (status %s): %w", tenantID, tenant.Status, ErrTenantNotPurgeable)
	}

	approved, err := s.repo.ApproveTenantPurgeRequest(ctx, open.ID, actor.UserID)
	if err != nil {
		return nil, fmt.Errorf("iam: approve tenant purge %s: %w", tenantID, err)
	}

	s.auditPurge(ctx, audit.ActionPurgeApproved, tenantID, actor, reason)
	return approved, nil
}

// CancelTenantPurge cancels the open (pending or approved) request for a
// tenant — a safety valve for the window before the purge-worker executes.
func (s *Service) CancelTenantPurge(ctx context.Context, tenantID uuid.UUID, reason string) (*TenantPurgeRequest, error) {
	open, err := s.repo.GetOpenTenantPurgeRequest(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("iam: cancel tenant purge %s: %w", tenantID, err)
	}

	actor, _ := audit.ActorFromContext(ctx)
	cancelled, err := s.repo.CancelTenantPurgeRequest(ctx, open.ID, actor.UserID)
	if err != nil {
		return nil, fmt.Errorf("iam: cancel tenant purge %s: %w", tenantID, err)
	}

	s.auditPurge(ctx, audit.ActionPurgeCancelled, tenantID, actor, reason)
	return cancelled, nil
}

// PurgeApprovedTenant executes an approved purge request (worker-facing —
// called by the purge-worker binary, not by an RPC handler). On success it
// emits ActionTenantPurged under the system actor. On failure it records
// the failure reason on the request via MarkTenantPurgeRequestFailed (so
// operators can see why a run failed and retry) and returns a wrapped
// error — callers MUST treat this as an aborted purge, not a completed one.
func (s *Service) PurgeApprovedTenant(ctx context.Context, req *TenantPurgeRequest) error {
	if err := s.repo.PurgeTenantSchemaAndTombstone(ctx, req.TenantID, req.ID); err != nil {
		if _, ferr := s.repo.MarkTenantPurgeRequestFailed(ctx, req.ID, err.Error()); ferr != nil {
			return fmt.Errorf("iam: purge %s failed (%v) and mark-failed errored: %w", req.TenantID, err, ferr)
		}
		return fmt.Errorf("iam: purge %s: %w", req.TenantID, err)
	}

	if s.audit != nil {
		s.audit.Log(audit.Event{
			TenantID:     req.TenantID,
			ActorEmail:   SystemActorPurgeWorker,
			Action:       audit.ActionTenantPurged,
			ResourceType: audit.ResourceTenant,
			ResourceID:   req.TenantID,
			OccurredAt:   time.Now().UTC(),
		})
	}
	return nil
}

// auditPurge emits a purge-lifecycle audit event carrying the ctx actor and
// a free-text reason (via mustMarshalClearReason, the existing
// {"reason": "..."} helper from lifecycle.go).
func (s *Service) auditPurge(ctx context.Context, action audit.Action, tenantID uuid.UUID, actor audit.ActorInfo, reason string) {
	if s.audit == nil {
		return
	}
	s.audit.Log(audit.Event{
		TenantID:     tenantID,
		ActorID:      actor.UserID,
		ActorEmail:   actor.Email,
		ActorIP:      actor.IP,
		Action:       action,
		ResourceType: audit.ResourceTenant,
		ResourceID:   tenantID,
		Metadata:     mustMarshalClearReason(reason),
		OccurredAt:   time.Now().UTC(),
	})
}
