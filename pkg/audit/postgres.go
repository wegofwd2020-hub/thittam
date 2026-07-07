package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres is the append-only audit_log Store backed by a pgx pool.
// It performs INSERT and SELECT only — never UPDATE or DELETE.
type Postgres struct{ pool *pgxpool.Pool }

// NewPostgres returns a Postgres audit Store over the given pool.
func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

const insertAuditSQL = `
INSERT INTO audit_log
	(tenant_id, actor_id, actor_email, actor_ip, action, resource_type,
	 resource_id, production_id, old_state, new_state, metadata, occurred_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, COALESCE($12, now()))`

// insertArgs maps an Event to the 12 positional args for insertAuditSQL.
// Zero/empty optional fields become SQL NULL (id omitted → DB default;
// occurred_at zero → COALESCE picks now()).
func insertArgs(e Event) []any {
	return []any{
		e.TenantID, e.ActorID, e.ActorEmail, nullStr(e.ActorIP),
		string(e.Action), string(e.ResourceType), e.ResourceID,
		nullUUID(e.ProductionID), nullJSON(e.OldState), nullJSON(e.NewState),
		nullJSON(e.Metadata), nullTime(e.OccurredAt),
	}
}

func (p *Postgres) Insert(ctx context.Context, e Event) error {
	if _, err := p.pool.Exec(ctx, insertAuditSQL, insertArgs(e)...); err != nil {
		return fmt.Errorf("audit/db: insert: %w", err)
	}
	return nil
}

func (p *Postgres) InsertBatch(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("audit/db: begin batch: %w", err)
	}
	defer tx.Rollback(ctx) // no-op after a successful Commit
	for _, e := range events {
		if _, err := tx.Exec(ctx, insertAuditSQL, insertArgs(e)...); err != nil {
			return fmt.Errorf("audit/db: batch insert: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (p *Postgres) Query(ctx context.Context, f QueryFilter) ([]Event, error) {
	args := []any{f.TenantID}
	where := "tenant_id = $1"
	add := func(v any, col string) {
		args = append(args, v)
		where += fmt.Sprintf(" AND %s = $%d", col, len(args))
	}
	if f.ActorID != nil {
		add(*f.ActorID, "actor_id")
	}
	if f.ResourceType != nil {
		add(string(*f.ResourceType), "resource_type")
	}
	if f.ResourceID != nil {
		add(*f.ResourceID, "resource_id")
	}
	if f.Action != nil {
		add(string(*f.Action), "action")
	}
	if f.From != nil {
		args = append(args, *f.From)
		where += fmt.Sprintf(" AND occurred_at >= $%d", len(args))
	}
	if f.To != nil {
		args = append(args, *f.To)
		where += fmt.Sprintf(" AND occurred_at <= $%d", len(args))
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	args = append(args, limit, f.Offset)
	sql := fmt.Sprintf(`
		SELECT id, tenant_id, actor_id, actor_email, actor_ip, action, resource_type,
		       resource_id, production_id, old_state, new_state, metadata, occurred_at
		FROM audit_log WHERE %s
		ORDER BY occurred_at DESC LIMIT $%d OFFSET $%d`,
		where, len(args)-1, len(args))

	rows, err := p.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("audit/db: query: %w", err)
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var (
			e            Event
			actorIP      *string
			productionID *uuid.UUID
			oldState     []byte
			newState     []byte
			metadata     []byte
		)
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.ActorID, &e.ActorEmail, &actorIP, &e.Action,
			&e.ResourceType, &e.ResourceID, &productionID, &oldState, &newState,
			&metadata, &e.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf("audit/db: scan: %w", err)
		}
		if actorIP != nil {
			e.ActorIP = *actorIP
		}
		e.ProductionID = productionID
		e.OldState = json.RawMessage(oldState)
		e.NewState = json.RawMessage(newState)
		e.Metadata = json.RawMessage(metadata)
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- null helpers: return nil (→ SQL NULL) for zero values ---

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func nullUUID(u *uuid.UUID) any {
	if u == nil {
		return nil
	}
	return *u
}
func nullJSON(j json.RawMessage) any {
	if len(j) == 0 {
		return nil
	}
	return []byte(j)
}
func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

var _ Store = (*Postgres)(nil)
