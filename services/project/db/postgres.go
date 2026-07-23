package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/wegofwd2020/thittam/services/project"
)

// Postgres implements project.Repository backed by PostgreSQL.
type Postgres struct {
	q  *Queries
	db *pgxpool.Pool
}

// NewPostgres creates a Postgres-backed project repository.
func NewPostgres(db *pgxpool.Pool) *Postgres {
	return &Postgres{q: New(db), db: db}
}

// Compile-time interface check.
var _ project.Repository = (*Postgres)(nil)

// --- Productions ---

func (p *Postgres) CreateProduction(ctx context.Context, prod *project.Production) error {
	_, err := p.q.CreateProduction(ctx, CreateProductionParams{
		ID:          prod.ID,
		TenantID:    prod.TenantID,
		Title:       prod.Title,
		Slug:        prod.Slug,
		Description: pgTextFromStr(prod.Description),
		Genre:       pgTextFromStr(prod.Genre),
		Language:    pgTextFromStr(prod.Language),
		Status:      prod.Status,
		StartDate:   pgDateFromPtr(prod.StartDate),
		EndDate:     pgDateFromPtr(prod.EndDate),
		CreatedBy:   prod.CreatedBy,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return project.ErrDuplicateSlug
		}
		return fmt.Errorf("project: create production: %w", err)
	}
	return nil
}

func (p *Postgres) GetProduction(ctx context.Context, tenantID, id uuid.UUID) (*project.Production, error) {
	row, err := p.q.GetProduction(ctx, GetProductionParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, project.ErrProductionNotFound
		}
		return nil, fmt.Errorf("project: get production: %w", err)
	}
	return productionFromDB(row), nil
}

func (p *Postgres) ListProductions(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]project.Production, error) {
	rows, err := p.q.ListProductions(ctx, ListProductionsParams{
		TenantID: tenantID,
		Column2:  status,
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("project: list productions: %w", err)
	}
	result := make([]project.Production, len(rows))
	for i, row := range rows {
		result[i] = *productionFromDB(row)
	}
	return result, nil
}

func (p *Postgres) UpdateProduction(ctx context.Context, prod *project.Production) error {
	_, err := p.q.UpdateProduction(ctx, UpdateProductionParams{
		ID:          prod.ID,
		TenantID:    prod.TenantID,
		Column3:     prod.Title,       // COALESCE(NULLIF($3,''), title)
		Description: pgTextFromStr(prod.Description),
		Column5:     prod.Genre,       // COALESCE(NULLIF($5,''), genre)
		Column6:     prod.Status,      // COALESCE(NULLIF($6,''), status)
		StartDate:   pgDateFromPtr(prod.StartDate),
		EndDate:     pgDateFromPtr(prod.EndDate),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return project.ErrProductionNotFound
		}
		return fmt.Errorf("project: update production: %w", err)
	}
	return nil
}

func (p *Postgres) ArchiveProduction(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := p.q.ArchiveProduction(ctx, ArchiveProductionParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return project.ErrProductionNotFound
		}
		return fmt.Errorf("project: archive production: %w", err)
	}
	return nil
}

// --- Phases ---

func (p *Postgres) CreatePhase(ctx context.Context, phase *project.Phase) error {
	_, err := p.q.CreatePhase(ctx, CreatePhaseParams{
		ID:           phase.ID,
		ProductionID: phase.ProductionID,
		TenantID:     phase.TenantID,
		PhaseType:    phase.PhaseType,
		Status:       phase.Status,
		StartDate:    pgDateFromPtr(phase.StartDate),
		EndDate:      pgDateFromPtr(phase.EndDate),
	})
	if err != nil {
		return fmt.Errorf("project: create phase: %w", err)
	}
	return nil
}

func (p *Postgres) GetPhase(ctx context.Context, tenantID, id uuid.UUID) (*project.Phase, error) {
	row, err := p.q.GetPhase(ctx, GetPhaseParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, project.ErrPhaseNotFound
		}
		return nil, fmt.Errorf("project: get phase: %w", err)
	}
	return phaseFromDB(row), nil
}

func (p *Postgres) ListPhases(ctx context.Context, tenantID, productionID uuid.UUID, limit, offset int) ([]project.Phase, error) {
	rows, err := p.q.ListPhases(ctx, ListPhasesParams{
		ProductionID: productionID,
		TenantID:     tenantID,
		Limit:        int32(limit),
		Offset:       int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("project: list phases: %w", err)
	}
	result := make([]project.Phase, len(rows))
	for i, row := range rows {
		result[i] = *phaseFromDB(row)
	}
	return result, nil
}

func (p *Postgres) UpdatePhaseStatus(ctx context.Context, tenantID, id uuid.UUID, status string) error {
	_, err := p.q.UpdatePhaseStatus(ctx, UpdatePhaseStatusParams{ID: id, TenantID: tenantID, Status: status})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return project.ErrPhaseNotFound
		}
		return fmt.Errorf("project: update phase status: %w", err)
	}
	return nil
}

// --- Crew ---

func (p *Postgres) AddCrewMember(ctx context.Context, c *project.CrewMember) error {
	var userID pgtype.UUID
	if c.UserID != nil {
		userID = pgtype.UUID{Bytes: *c.UserID, Valid: true}
	}
	_, err := p.q.AddCrewMember(ctx, AddCrewMemberParams{
		ID:           c.ID,
		ProductionID: c.ProductionID,
		TenantID:     c.TenantID,
		UserID:       userID,
		Name:         c.Name,
		Role:         c.Role,
		Department:   pgTextFromStr(c.Department),
		DayRate:      pgNumericFromDecimal(c.DayRate),
		Currency:     pgTextFromStr(c.Currency),
		StartDate:    pgDateFromPtr(c.StartDate),
		EndDate:      pgDateFromPtr(c.EndDate),
	})
	if err != nil {
		return fmt.Errorf("project: add crew member: %w", err)
	}
	return nil
}

func (p *Postgres) ListCrewMembers(ctx context.Context, tenantID, productionID uuid.UUID, limit, offset int) ([]project.CrewMember, error) {
	rows, err := p.q.ListCrewMembers(ctx, ListCrewMembersParams{
		ProductionID: productionID,
		TenantID:     tenantID,
		Limit:        int32(limit),
		Offset:       int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("project: list crew members: %w", err)
	}
	result := make([]project.CrewMember, len(rows))
	for i, row := range rows {
		result[i] = *crewFromDB(row)
	}
	return result, nil
}

// RemoveCrewMember deletes a crew member and returns ErrCrewNotFound if the row
// does not exist in the caller's tenant. The sqlc-generated query is :exec with
// no affected-row count, so we use a direct Exec call instead.
func (p *Postgres) RemoveCrewMember(ctx context.Context, tenantID, id uuid.UUID) error {
	tag, err := p.db.Exec(ctx, "DELETE FROM crew_members WHERE id = $1 AND tenant_id = $2", id, tenantID)
	if err != nil {
		return fmt.Errorf("project: remove crew member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return project.ErrCrewNotFound
	}
	return nil
}

// --- model conversion helpers ---

func productionFromDB(row Production) *project.Production {
	return &project.Production{
		ID:          row.ID,
		TenantID:    row.TenantID,
		Title:       row.Title,
		Slug:        row.Slug,
		Description: strFromPgText(row.Description),
		Genre:       strFromPgText(row.Genre),
		Language:    strFromPgText(row.Language),
		Status:      row.Status,
		StartDate:   ptrFromPgDate(row.StartDate),
		EndDate:     ptrFromPgDate(row.EndDate),
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func phaseFromDB(row Phase) *project.Phase {
	return &project.Phase{
		ID:           row.ID,
		ProductionID: row.ProductionID,
		TenantID:     row.TenantID,
		PhaseType:    row.PhaseType,
		Status:       row.Status,
		StartDate:    ptrFromPgDate(row.StartDate),
		EndDate:      ptrFromPgDate(row.EndDate),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

func crewFromDB(row CrewMember) *project.CrewMember {
	var userID *uuid.UUID
	if row.UserID.Valid {
		id := uuid.UUID(row.UserID.Bytes)
		userID = &id
	}
	return &project.CrewMember{
		ID:           row.ID,
		ProductionID: row.ProductionID,
		TenantID:     row.TenantID,
		UserID:       userID,
		Name:         row.Name,
		Role:         row.Role,
		Department:   strFromPgText(row.Department),
		DayRate:      decimalFromPgNumeric(row.DayRate),
		Currency:     strFromPgText(row.Currency),
		StartDate:    ptrFromPgDate(row.StartDate),
		EndDate:      ptrFromPgDate(row.EndDate),
		CreatedAt:    row.CreatedAt,
	}
}

// --- pgtype conversion helpers ---

func pgTextFromStr(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

func strFromPgText(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

// pgDateFromPtr converts a *string ISO date ("2006-01-02") to pgtype.Date.
func pgDateFromPtr(s *string) pgtype.Date {
	if s == nil || *s == "" {
		return pgtype.Date{Valid: false}
	}
	var d pgtype.Date
	_ = d.Scan(*s)
	return d
}

// ptrFromPgDate converts a pgtype.Date to a *string ISO date.
func ptrFromPgDate(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format(time.DateOnly)
	return &s
}

// pgNumericFromDecimal converts a decimal.Decimal to pgtype.Numeric via its
// string representation so we do not depend on shopspring internals.
func pgNumericFromDecimal(d decimal.Decimal) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(d.String())
	return n
}

// decimalFromPgNumeric converts a pgtype.Numeric to decimal.Decimal.
// Returns decimal.Zero for NULL / NaN values.
func decimalFromPgNumeric(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.NaN || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

// isUniqueViolation returns true when err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
