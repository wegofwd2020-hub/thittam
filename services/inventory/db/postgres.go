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

func (p *Postgres) ListAssets(ctx context.Context, tenantID uuid.UUID, status, categoryID, search string, limit, offset int) ([]inventory.Asset, error) {
	rows, err := p.q.ListAssets(ctx, ListAssetsParams{
		TenantID: tenantID,
		Column2:  status,     // status filter; "" = no filter
		Column3:  categoryID, // category filter; "" = no filter
		Limit:    int32(limit),
		Offset:   int32(offset),
		Column6:  search, // name/asset_code search; "" = no filter
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

func (p *Postgres) GetActiveCheckout(ctx context.Context, tenantID, assetID uuid.UUID) (*inventory.AssetCheckout, error) {
	row, err := p.q.GetActiveCheckout(ctx, GetActiveCheckoutParams{
		AssetID:  assetID,
		TenantID: tenantID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, inventory.ErrNoActiveCheckout
		}
		return nil, fmt.Errorf("inventory: get active checkout: %w", err)
	}
	return checkoutFromDB(row), nil
}

func (p *Postgres) CheckInAsset(ctx context.Context, tenantID, checkoutID uuid.UUID, in inventory.CheckInInput) (*inventory.AssetCheckout, error) {
	row, err := p.q.CheckinAsset(ctx, CheckinAssetParams{
		ID:                checkoutID,
		TenantID:          tenantID,
		ConditionIn:       pgTextFromString(in.ConditionIn),
		Notes:             pgTextFromString(in.Notes),
		ReportDamage:      in.ReportDamage,
		DamageSeverity:    pgTextFromString(in.DamageSeverity),
		DamageDescription: pgTextFromString(in.DamageDescription),
		RepairCost:        pgNumericFromDecimalPtr(in.RepairCost),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, inventory.ErrAssetNotFound
		}
		return nil, fmt.Errorf("inventory: check in asset: %w", err)
	}
	return checkoutFromDB(row), nil
}

func (p *Postgres) GetCheckout(ctx context.Context, tenantID, id uuid.UUID) (*inventory.AssetCheckout, error) {
	row, err := p.q.GetCheckout(ctx, GetCheckoutParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, inventory.ErrAssetNotFound
		}
		return nil, fmt.Errorf("inventory: get checkout: %w", err)
	}
	return checkoutFromDB(row), nil
}

// ListCheckouts lists checkout records for an asset, scoped to the caller's
// tenant, newest-first, using a keyset cursor (after) on checked_out_at.
func (p *Postgres) ListCheckouts(ctx context.Context, tenantID, assetID uuid.UUID, limit int, after string) ([]inventory.AssetCheckout, error) {
	var afterTS pgtype.Timestamptz
	if after != "" {
		t, err := time.Parse(time.RFC3339, after)
		if err != nil {
			return nil, fmt.Errorf("inventory/db: invalid after cursor: %w", err)
		}
		afterTS = pgtype.Timestamptz{Time: t, Valid: true}
	}
	rows, err := p.q.ListCheckouts(ctx, ListCheckoutsParams{
		TenantID: tenantID,
		AssetID:  pgtype.UUID{Bytes: assetID, Valid: true},
		After:    afterTS,
		Limit:    int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("inventory: list checkouts: %w", err)
	}
	result := make([]inventory.AssetCheckout, len(rows))
	for i, row := range rows {
		result[i] = *checkoutFromDB(row)
	}
	return result, nil
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
		ID:                row.ID,
		AssetID:           row.AssetID,
		ProductionID:      row.ProductionID,
		TenantID:          row.TenantID,
		CheckedOutTo:      row.CheckedOutTo,
		CheckedOutAt:      row.CheckedOutAt,
		ConditionOut:      stringFromPgText(row.ConditionOut),
		ConditionIn:       stringFromPgText(row.ConditionIn),
		Notes:             stringFromPgText(row.Notes),
		ReportDamage:      row.ReportDamage,
		DamageSeverity:    stringFromPgText(row.DamageSeverity),
		DamageDescription: stringFromPgText(row.DamageDescription),
		RepairCost:        decimalPtrFromPgNumeric(row.RepairCost),
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

// pgNumericFromDecimalPtr converts an optional decimal to pgtype.Numeric,
// returning an invalid (NULL) Numeric when d is nil.
func pgNumericFromDecimalPtr(d *decimal.Decimal) pgtype.Numeric {
	if d == nil {
		return pgtype.Numeric{}
	}
	return pgNumericFromDecimal(*d)
}

// decimalPtrFromPgNumeric converts a pgtype.Numeric to an optional decimal,
// returning nil when the DB value is NULL.
func decimalPtrFromPgNumeric(n pgtype.Numeric) *decimal.Decimal {
	if !n.Valid {
		return nil
	}
	d := decimalFromPgNumeric(n)
	return &d
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
