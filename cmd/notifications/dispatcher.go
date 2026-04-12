package main

// dispatcher.go — routes decoded NATS events to notifications.Service.Send calls.
//
// Each handler function receives the parsed payload and dispatches one or more
// notification send requests. Handlers return nil on success (triggers Ack) and a
// non-nil error on transient failure (triggers Nak + redelivery).
//
// Design notes:
//   - RecipientID is set to uuid.Nil when the recipient cannot be determined from
//     the event payload alone (e.g. broadcast events). The service skips sends for
//     nil recipients — a separate fan-out query would be needed for real production
//     use but is out of scope for this wiring pass.
//   - The channel is always "email" for now. Multi-channel fan-out can be added
//     later by iterating over the recipient's preferred channels.
//   - Unknown event types are silently acknowledged (forward-compatible Rule #16).

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/events"
	"github.com/wegofwd2020/thittam/services/notifications"
)

type dispatcher struct {
	svc *notifications.Service
}

// dispatch routes an envelope to the appropriate send handler based on event type.
// Returns nil for unknown types (forward-compatible).
func (d *dispatcher) dispatch(ctx context.Context, env *events.Envelope) error {
	switch env.Type {
	case events.SubjectExpenseSubmitted:
		return d.onExpenseSubmitted(ctx, env)
	case events.SubjectExpenseApproved:
		return d.onExpenseApproved(ctx, env)
	case events.SubjectBudgetApproved:
		return d.onBudgetApproved(ctx, env)
	case events.SubjectProjectCreated:
		return d.onProjectCreated(ctx, env)
	case events.SubjectProjectStatusChanged:
		return d.onProjectStatusChanged(ctx, env)
	case events.SubjectProjectMemberAssigned:
		return d.onProjectMemberAssigned(ctx, env)
	case events.SubjectDocumentSigned:
		return d.onDocumentSigned(ctx, env)
	case events.SubjectLedgerJournalPosted:
		return d.onLedgerJournalPosted(ctx, env)
	default:
		// Unknown event type — acknowledge silently.
		slog.DebugContext(ctx, "notifications: unknown event type — skipping",
			"event_type", env.Type,
			"event_id", env.EventID,
		)
		return nil
	}
}

func (d *dispatcher) onExpenseSubmitted(ctx context.Context, env *events.Envelope) error {
	var p events.ExpenseSubmittedPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return err
	}
	_, err := d.svc.Send(ctx, &notifications.SendRequest{
		TenantID:    env.TenantID,
		RecipientID: uuid.Nil, // approver lookup required — placeholder
		Channel:     "email",
		EventType:   events.SubjectExpenseSubmitted,
		Data: map[string]any{
			"expense_id":    p.ExpenseID,
			"description":   p.Description,
			"amount":        p.Amount,
			"submitted_by":  p.SubmittedBy,
			"project_title": p.ProjectTitle,
		},
	})
	return ignoreTemplateMissing(err)
}

func (d *dispatcher) onExpenseApproved(ctx context.Context, env *events.Envelope) error {
	var p events.ExpenseApprovedPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return err
	}
	_, err := d.svc.Send(ctx, &notifications.SendRequest{
		TenantID:    env.TenantID,
		RecipientID: uuid.Nil, // submitter lookup required — placeholder
		Channel:     "email",
		EventType:   events.SubjectExpenseApproved,
		Data: map[string]any{
			"expense_id":    p.ExpenseID,
			"amount":        p.Amount,
			"category_name": p.CategoryName,
			"approved_at":   p.ApprovedAt,
		},
	})
	return ignoreTemplateMissing(err)
}

func (d *dispatcher) onBudgetApproved(ctx context.Context, env *events.Envelope) error {
	var p events.BudgetApprovedPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return err
	}
	_, err := d.svc.Send(ctx, &notifications.SendRequest{
		TenantID:    env.TenantID,
		RecipientID: uuid.Nil,
		Channel:     "email",
		EventType:   events.SubjectBudgetApproved,
		Data: map[string]any{
			"budget_id":       p.BudgetID,
			"production_id":   p.ProductionID,
			"total_budgeted":  p.TotalBudgeted,
			"total_actual":    p.TotalActual,
			"total_committed": p.TotalCommitted,
		},
	})
	return ignoreTemplateMissing(err)
}

func (d *dispatcher) onProjectStatusChanged(ctx context.Context, env *events.Envelope) error {
	var p events.ProjectStatusChangedPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return err
	}
	_, err := d.svc.Send(ctx, &notifications.SendRequest{
		TenantID:    env.TenantID,
		RecipientID: uuid.Nil,
		Channel:     "in_app",
		EventType:   events.SubjectProjectStatusChanged,
		Data: map[string]any{
			"project_id":    p.ProjectID,
			"title":         p.Title,
			"old_status":    p.OldStatus,
			"new_status":    p.NewStatus,
			"current_phase": p.CurrentPhase,
		},
	})
	return ignoreTemplateMissing(err)
}

func (d *dispatcher) onDocumentSigned(ctx context.Context, env *events.Envelope) error {
	var p events.DocumentSignedPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return err
	}
	_, err := d.svc.Send(ctx, &notifications.SendRequest{
		TenantID:    env.TenantID,
		RecipientID: p.SignedBy,
		Channel:     "email",
		EventType:   events.SubjectDocumentSigned,
		Data: map[string]any{
			"document_id":   p.DocumentID,
			"document_name": p.DocumentName,
			"signed_at":     p.SignedAt,
		},
	})
	return ignoreTemplateMissing(err)
}

func (d *dispatcher) onProjectCreated(ctx context.Context, env *events.Envelope) error {
	var p events.ProjectCreatedPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return err
	}
	_, err := d.svc.Send(ctx, &notifications.SendRequest{
		TenantID:    env.TenantID,
		RecipientID: uuid.Nil, // broadcast to tenant admins — lookup required
		Channel:     "in_app",
		EventType:   events.SubjectProjectCreated,
		Data: map[string]any{
			"project_id": p.ProjectID,
			"title":      p.Title,
			"status":     p.Status,
			"start_date": p.StartDate,
		},
	})
	return ignoreTemplateMissing(err)
}

func (d *dispatcher) onProjectMemberAssigned(ctx context.Context, env *events.Envelope) error {
	var p events.ProjectMemberAssignedPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return err
	}
	_, err := d.svc.Send(ctx, &notifications.SendRequest{
		TenantID:    env.TenantID,
		RecipientID: uuid.Nil, // assigned member lookup required
		Channel:     "in_app",
		EventType:   events.SubjectProjectMemberAssigned,
		Data: map[string]any{
			"project_id":    p.ProjectID,
			"project_title": p.ProjectTitle,
			"member_count":  p.MemberCount,
		},
	})
	return ignoreTemplateMissing(err)
}

func (d *dispatcher) onLedgerJournalPosted(ctx context.Context, env *events.Envelope) error {
	var p events.LedgerJournalPostedPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return err
	}
	_, err := d.svc.Send(ctx, &notifications.SendRequest{
		TenantID:    env.TenantID,
		RecipientID: uuid.Nil, // finance team lookup required
		Channel:     "email",
		EventType:   events.SubjectLedgerJournalPosted,
		Data: map[string]any{
			"journal_entry_id": p.JournalEntryID,
			"entry_number":     p.EntryNumber,
			"total_debit":      p.TotalDebit,
			"total_credit":     p.TotalCredit,
			"narration":        p.Narration,
			"posted_at":        p.PostedAt,
		},
	})
	return ignoreTemplateMissing(err)
}

// ignoreTemplateMissing converts ErrTemplateNotFound into nil so that missing
// templates do not cause Nak loops. The event is acknowledged as "processed" —
// the operator must add a template if they want the notification sent.
func ignoreTemplateMissing(err error) error {
	if err == notifications.ErrTemplateNotFound {
		return nil
	}
	return err
}
