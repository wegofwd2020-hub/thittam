// Package inventory implements the inventory-management service.
package inventory

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Asset represents an equipment item, prop, location, or other trackable resource.
type Asset struct {
	ID            uuid.UUID       `json:"id"`
	TenantID      uuid.UUID       `json:"tenant_id"`
	AssetCode     string          `json:"asset_code"`
	Name          string          `json:"name"`
	CategoryID    string          `json:"category_id"`
	Description   string          `json:"description,omitempty"`
	OwnershipType string          `json:"ownership_type"` // owned, rented, borrowed
	Status        string          `json:"status"`         // available, checked_out, under_repair, retired
	PurchaseDate  *string         `json:"purchase_date,omitempty"`
	PurchaseCost  decimal.Decimal `json:"purchase_cost,omitempty"`
	SerialNumber  string          `json:"serial_number,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// AssetCheckout represents a checkout/check-in record for an asset.
type AssetCheckout struct {
	ID             uuid.UUID  `json:"id"`
	AssetID        uuid.UUID  `json:"asset_id"`
	ProductionID   uuid.UUID  `json:"production_id"`
	TenantID       uuid.UUID  `json:"tenant_id"`
	CheckedOutTo   uuid.UUID  `json:"checked_out_to"`
	CheckedOutAt   time.Time  `json:"checked_out_at"`
	ExpectedReturn *string    `json:"expected_return,omitempty"`
	CheckedInAt    *time.Time `json:"checked_in_at,omitempty"`
	ConditionOut   string     `json:"condition_out,omitempty"`
	ConditionIn    string     `json:"condition_in,omitempty"`
}
