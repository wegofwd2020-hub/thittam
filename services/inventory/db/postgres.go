package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/wegofwd2020/thittam/services/inventory"
)

// Postgres implements inventory.Repository backed by PostgreSQL.
type Postgres struct {
	q  *Queries
	db *pgxpool.Pool
}

// NewPostgres creates a Postgres-backed inventory repository.
func NewPostgres(db *pgxpool.Pool) *Postgres {
	return &Postgres{q: New(db), db: db}
}

// Compile-time interface check.
var _ inventory.Repository = (*Postgres)(nil)

// --- Assets ---

func (p *Postgres) CreateAsset(ctx context.Context, a *inventory.Asset) error {
	_, err := p.q.CreateAsset(ctx, CreateAssetParams{
		ID:            a.ID,
		TenantID:      a.TenantID,
		AssetCode:     a.AssetCode,
		Name:          a.Name,
		CategoryID:    a.CategoryID,
		Description:   pgTextFromString(a.Description),
		OwnershipType: a.OwnershipType,
		Status:        a.Status,
		PurchaseDate:  pgDateFromPtrString(a.PurchaseDate),
		PurchaseCost:  pgNumericFromDecimal(a.PurchaseCost),
		SerialNumber:  pgTextFromString(a.SerialNumber),
	})
	if err != nil {
		return fmt.Errorf("inventory: create asset: %w", err)
	}
	return nil
}

func (p *Postgres) GetAsset(ctx context.Context, tenantID, id uuid.UUID) (*inventory.Asset, error) {
	row, err := p.q.GetAsset(ctx, GetAssetParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, inventory.ErrAssetNotFound
		}
		return nil, fmt.Errorf("inventory: get asset: %w", err)
	}
	return assetFromDB(row), nil
}

func (p *Postgres) ListAssets(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]inventory.Asset, error) {
	rows, err := p.q.ListAssets(ctx, ListAssetsParams{
		TenantID: tenantID,
		Column2:  status,   // status filter; "" = no filter
		Column3:  "",       // category filter not exposed by this interface
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("inventory: list assets: %w", err)
	}
	result := make([]inventory.Asset, len(rows))
	for i, row := range rows {
		result[i] = *assetFromDB(row)
	}
	return result, nil
}

func (p *Postgres) UpdateAssetStatus(ctx context.Context, tenantID, id uuid.UUID, status string) error {
	_, err := p.q.UpdateAssetStatus(ctx, UpdateAssetStatusParams{
		ID:       id,
		TenantID: tenantID,
		Status:   status,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return inventory.ErrAssetNotFound
		}
		return fmt.Errorf("inventory: update asset status: %w", err)
	}
	return nil
}

// --- Checkouts ---

func (p *Postgres) CheckOutAsset(ctx context.Context, c *inventory.AssetCheckout) error {
	_, err := p.q.CheckoutAsset(ctx, CheckoutAssetParams{
		ID:             c.ID,
		AssetID:        c.AssetID,
		ProductionID:   c.ProductionID,
		TenantID:       c.TenantID,
		CheckedOutTo:   c.CheckedOutTo,
		ExpectedReturn: pgDateFromPtrString(c.ExpectedReturn),
		ConditionOut:   pgTextFromString(c.ConditionOut),
	})
	if err != nil {
		return fmt.Errorf("inventory: checkout asset: %w", err)
	}
	return nil
}

func (p *Postgres) CheckInAsset(ctx context.Context, checkoutID uuid.UUID, conditionIn string) error {
	// Resolve tenant_id — the sqlc query requires it for the WHERE clause.
	tenantID, err := p.resolveTenantForCheckout(ctx, checkoutID)
	if err != nil {
		return err
	}

	_, err = p.q.CheckinAsset(ctx, CheckinAssetParams{
		ID:          checkoutID,
		TenantID:    tenantID,
		ConditionIn: pgTextFromString(conditionIn),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return inventory.ErrAssetNotFound
		}
		return fmt.Errorf("inventory: check in asset: %w", err)
	}
	return nil
}

func (p *Postgres) GetCheckout(ctx context.Context, id uuid.UUID) (*inventory.AssetCheckout, error) {
	const sql = `SELECT id, asset_id, production_id, tenant_id, checked_out_to,
		checked_out_at, expected_return, checked_in_at, condition_out, condition_in
		FROM asset_checkouts WHERE id = $1`

	var row AssetCheckout
	err := p.db.QueryRow(ctx, sql, id).Scan(
		&row.ID,
		&row.AssetID,
		&row.ProductionID,
		&row.TenantID,
		&row.CheckedOutTo,
		&row.CheckedOutAt,
		&row.ExpectedReturn,
		&row.CheckedInAt,
		&row.ConditionOut,
		&row.ConditionIn,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, inventory.ErrAssetNotFound
		}
		return nil, fmt.Errorf("inventory: get checkout: %w", err)
	}
	return checkoutFromDB(row), nil
}

// ListCheckouts lists all checkout records for an asset.
// A raw query is used because the sqlc-generated ListCheckouts has a
// production_id filter that cannot be disabled via a zero UUID.
func (p *Postgres) ListCheckouts(ctx context.Context, assetID uuid.UUID) ([]inventory.AssetCheckout, error) {
	const sql = `SELECT id, asset_id, production_id, tenant_id, checked_out_to,
		checked_out_at, expected_return, checked_in_at, condition_out, condition_in
		FROM asset_checkouts WHERE asset_id = $1 ORDER BY checked_out_at DESC`

	rows, err := p.db.Query(ctx, sql, assetID)
	if err != nil {
		return nil, fmt.Errorf("inventory: list checkouts: %w", err)
	}
	defer rows.Close()

	var result []inventory.AssetCheckout
	for rows.Next() {
		var row AssetCheckout
		if err := rows.Scan(
			&row.ID,
			&row.AssetID,
			&row.ProductionID,
			&row.TenantID,
			&row.CheckedOutTo,
			&row.CheckedOutAt,
			&row.ExpectedReturn,
			&row.CheckedInAt,
			&row.ConditionOut,
			&row.ConditionIn,
		); err != nil {
			return nil, fmt.Errorf("inventory: scan checkout: %w", err)
		}
		result = append(result, *checkoutFromDB(row))
	}
	return result, rows.Err()
}

// resolveTenantForCheckout looks up the tenant_id for a checkout record.
func (p *Postgres) resolveTenantForCheckout(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	var tenantID uuid.UUID
	err := p.db.QueryRow(ctx, "SELECT tenant_id FROM asset_checkouts WHERE id = $1", id).Scan(&tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, inventory.ErrAssetNotFound
		}
		return uuid.Nil, fmt.Errorf("inventory: resolve tenant for checkout %s: %w", id, err)
	}
	return tenantID, nil
}

// --- model conversion helpers ---

func assetFromDB(row Asset) *inventory.Asset {
	return &inventory.Asset{
		ID:            row.ID,
		TenantID:      row.TenantID,
		AssetCode:     row.AssetCode,
		Name:          row.Name,
		CategoryID:    row.CategoryID,
		Description:   stringFromPgText(row.Description),
		OwnershipType: row.OwnershipType,
		Status:        row.Status,
		PurchaseDate:  ptrStringFromPgDate(row.PurchaseDate),
		PurchaseCost:  decimalFromPgNumeric(row.PurchaseCost),
		SerialNumber:  stringFromPgText(row.SerialNumber),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func checkoutFromDB(row AssetCheckout) *inventory.AssetCheckout {
	c := &inventory.AssetCheckout{
		ID:           row.ID,
		AssetID:      row.AssetID,
		ProductionID: row.ProductionID,
		TenantID:     row.TenantID,
		CheckedOutTo: row.CheckedOutTo,
		CheckedOutAt: row.CheckedOutAt,
		ConditionOut: stringFromPgText(row.ConditionOut),
		ConditionIn:  stringFromPgText(row.ConditionIn),
	}
	c.ExpectedReturn = ptrStringFromPgDate(row.ExpectedReturn)
	if row.CheckedInAt.Valid {
		t := row.CheckedInAt.Time
		c.CheckedInAt = &t
	}
	return c
}

// --- pgtype conversion helpers ---

func pgNumericFromDecimal(d decimal.Decimal) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(d.String())
	return n
}

func decimalFromPgNumeric(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.NaN || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

func pgTextFromString(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func stringFromPgText(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}

// pgDateFromPtrString converts an optional ISO date string to pgtype.Date.
func pgDateFromPtrString(s *string) pgtype.Date {
	if s == nil || *s == "" {
		return pgtype.Date{}
	}
	var d pgtype.Date
	_ = d.Scan(*s)
	return d
}

// ptrStringFromPgDate converts a pgtype.Date to an optional ISO date string.
func ptrStringFromPgDate(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format(time.DateOnly)
	return &s
}
