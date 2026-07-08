// Package audit provides an append-only audit trail for all security-sensitive
// and financial operations across the Thittam platform. Audit writes are
// non-blocking — events are buffered in a channel and flushed asynchronously.
package audit

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Action represents the verb of an audit event.
type Action string

const (
	ActionCreated        Action = "created"
	ActionUpdated        Action = "updated"
	ActionDeleted        Action = "deleted"
	ActionSubmitted      Action = "submitted"
	ActionApproved       Action = "approved"
	ActionRejected       Action = "rejected"
	ActionLocked         Action = "locked"
	ActionArchived       Action = "archived"
	ActionPosted         Action = "posted"
	ActionVoided         Action = "voided"
	ActionCheckedOut     Action = "checked_out"
	ActionCheckedIn      Action = "checked_in"
	ActionRetired        Action = "retired"
	ActionDamageReported Action = "damage_reported"
	ActionLoginSuccess   Action = "login_success"
	ActionLoginFailed    Action = "login_failed"
	ActionTokenRefreshed Action = "token_refreshed"
	ActionLogout         Action = "logout"
	ActionRoleAssigned   Action = "role_assigned"
	ActionRoleRevoked    Action = "role_revoked"
	ActionDeactivated    Action = "deactivated"
	ActionStatusChanged  Action = "status_changed"
	ActionPeriodOpened   Action = "period_opened"
	ActionPeriodClosed   Action = "period_closed"
	ActionOverBudget     Action = "over_budget_flagged"
	ActionImpersonated        Action = "impersonated"         // deprecated: use ActionImpersonationStarted
	ActionImpersonationStarted Action = "impersonation_started"
	ActionImpersonationEnded   Action = "impersonation_ended"
	ActionConfigChanged        Action = "config_changed"
	// ActionLegalHoldApplied is emitted by IAM SuspendTenant when the
	// caller supplies a freeze_reason to pause the retention sweeper
	// for litigation / regulatory holds (#92 Stage 4). Also emitted by
	// SetTenantRetention, the operator hold/extend RPC for tenants already
	// suspended (#119).
	ActionLegalHoldApplied Action = "legal_hold_applied"
	// ActionLegalHoldCleared is emitted by IAM ClearTenantLegalHold
	// when a previously-applied hold is released. Not emitted for
	// calls against a tenant that had no active hold — the
	// idempotent no-op path stays audit-quiet (the generic RPC audit
	// interceptor still records the call itself).
	ActionLegalHoldCleared Action = "legal_hold_cleared"
)

// ResourceType identifies the entity being acted upon.
type ResourceType string

const (
	ResourceProduction      ResourceType = "production"
	ResourceBudget          ResourceType = "budget"
	ResourceBudgetLineItem  ResourceType = "budget_line_item"
	ResourcePurchaseOrder   ResourceType = "purchase_order"
	ResourceExpense         ResourceType = "expense"
	ResourceJournalEntry    ResourceType = "journal_entry"
	ResourceAccountingPeriod ResourceType = "accounting_period"
	ResourceAsset           ResourceType = "asset"
	ResourceUser            ResourceType = "user"
	ResourceTenant          ResourceType = "tenant"
	ResourceVertical        ResourceType = "vertical"
	ResourceRole            ResourceType = "role"
	ResourceLogin           ResourceType = "login"
	ResourcePlatformUser        ResourceType = "platform_user"
	ResourceImpersonationSession ResourceType = "impersonation_session"
)

// Event represents a single audit log entry.
type Event struct {
	ID           uuid.UUID       `json:"id"`
	TenantID     uuid.UUID       `json:"tenant_id"`
	ActorID      uuid.UUID       `json:"actor_id"`
	ActorEmail   string          `json:"actor_email"`
	ActorIP      string          `json:"actor_ip,omitempty"`
	Action       Action          `json:"action"`
	ResourceType ResourceType    `json:"resource_type"`
	ResourceID   uuid.UUID       `json:"resource_id"`
	ProductionID *uuid.UUID      `json:"production_id,omitempty"`
	OldState     json.RawMessage `json:"old_state,omitempty"`
	NewState     json.RawMessage `json:"new_state,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	OccurredAt   time.Time       `json:"occurred_at"`
}

// QueryFilter defines the parameters for querying audit logs.
type QueryFilter struct {
	TenantID     uuid.UUID
	ActorID      *uuid.UUID
	ResourceType *ResourceType
	ResourceID   *uuid.UUID
	Action       *Action
	From         *time.Time
	To           *time.Time
	Limit        int
	Offset       int
}
