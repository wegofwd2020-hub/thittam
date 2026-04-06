-- 009_notification_templates.sql — Notification templates for XYZ_CBA Productions
-- Covers the key expense workflow events and document upload events.
-- Body templates use Go html/template syntax; variables are injected at render time.
-- Run after 001_tenant.sql.
--
-- Template UUID pattern: e3000000-0000-0000-0000-0000000000XX
-- Tenant:                d0000000-0000-0000-0000-000000000001

INSERT INTO notification_templates (id, tenant_id, event_type, channel, subject, body_template, is_active) VALUES

-- ── expense.submitted ─────────────────────────────────────

('e3000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001',
 'expense.submitted', 'email',
 'Expense submitted for approval: {{.description}}',
 '<p>Hi {{.approver_name}},</p>
<p><strong>{{.submitter_name}}</strong> has submitted an expense for your approval.</p>
<ul>
  <li><strong>Description:</strong> {{.description}}</li>
  <li><strong>Amount:</strong> ₹{{.amount}}</li>
  <li><strong>Production:</strong> {{.production_title}}</li>
  <li><strong>Category:</strong> {{.category}}</li>
</ul>
<p>Please log in to review and approve or reject this expense.</p>',
 true),

('e3000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001',
 'expense.submitted', 'in_app',
 NULL,
 '{{.submitter_name}} submitted ₹{{.amount}} expense "{{.description}}" for your approval.',
 true),

-- ── expense.approved ──────────────────────────────────────

('e3000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001',
 'expense.approved', 'email',
 'Your expense has been approved: {{.description}}',
 '<p>Hi {{.submitter_name}},</p>
<p>Your expense has been <strong>approved</strong> by {{.approver_name}}.</p>
<ul>
  <li><strong>Description:</strong> {{.description}}</li>
  <li><strong>Amount:</strong> ₹{{.amount}}</li>
  <li><strong>Production:</strong> {{.production_title}}</li>
</ul>
<p>The expense will be processed for payment in the next payment run.</p>',
 true),

('e3000000-0000-0000-0000-000000000004',
 'd0000000-0000-0000-0000-000000000001',
 'expense.approved', 'in_app',
 NULL,
 '{{.approver_name}} approved your ₹{{.amount}} expense "{{.description}}".',
 true),

-- ── expense.rejected ──────────────────────────────────────

('e3000000-0000-0000-0000-000000000005',
 'd0000000-0000-0000-0000-000000000001',
 'expense.rejected', 'email',
 'Your expense was rejected: {{.description}}',
 '<p>Hi {{.submitter_name}},</p>
<p>Your expense was <strong>rejected</strong> by {{.approver_name}}.</p>
<ul>
  <li><strong>Description:</strong> {{.description}}</li>
  <li><strong>Amount:</strong> ₹{{.amount}}</li>
  <li><strong>Reason:</strong> {{.rejection_reason}}</li>
</ul>
<p>Please revise and resubmit if appropriate.</p>',
 true),

('e3000000-0000-0000-0000-000000000006',
 'd0000000-0000-0000-0000-000000000001',
 'expense.rejected', 'in_app',
 NULL,
 '{{.approver_name}} rejected your ₹{{.amount}} expense "{{.description}}": {{.rejection_reason}}',
 true),

-- ── document.uploaded ────────────────────────────────────

('e3000000-0000-0000-0000-000000000007',
 'd0000000-0000-0000-0000-000000000001',
 'document.uploaded', 'in_app',
 NULL,
 '{{.uploader_name}} uploaded "{{.document_name}}" to {{.folder_name}}.',
 true),

-- ── budget.approved ───────────────────────────────────────

('e3000000-0000-0000-0000-000000000008',
 'd0000000-0000-0000-0000-000000000001',
 'budget.approved', 'email',
 'Budget approved: {{.production_title}} {{.budget_label}}',
 '<p>Hi {{.recipient_name}},</p>
<p>The budget <strong>{{.budget_label}}</strong> for <strong>{{.production_title}}</strong>
has been approved by {{.approver_name}}.</p>
<ul>
  <li><strong>Total:</strong> ₹{{.total_amount}}</li>
  <li><strong>Approved at:</strong> {{.approved_at}}</li>
</ul>',
 true),

('e3000000-0000-0000-0000-000000000009',
 'd0000000-0000-0000-0000-000000000001',
 'budget.approved', 'in_app',
 NULL,
 '{{.approver_name}} approved budget {{.budget_label}} for {{.production_title}} (₹{{.total_amount}}).',
 true)

ON CONFLICT (tenant_id, event_type, channel) DO NOTHING;
