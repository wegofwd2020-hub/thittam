package inventory

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines data access for the inventory-management service.
type Repository interface {
	// Assets
	CreateAsset(ctx context.Context, a *Asset) error
	GetAsset(ctx context.Context, tenantID, id uuid.UUID) (*Asset, error)
	ListAssets(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]Asset, error)
	UpdateAssetStatus(ctx context.Context, tenantID, id uuid.UUID, status string) error

	// Checkouts
	CheckOutAsset(ctx context.Context, c *AssetCheckout) error
	CheckInAsset(ctx context.Context, checkoutID uuid.UUID, conditionIn string) error
	ListCheckouts(ctx context.Context, assetID uuid.UUID) ([]AssetCheckout, error)
}
