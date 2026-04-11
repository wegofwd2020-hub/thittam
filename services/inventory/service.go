package inventory

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// Service implements inventory-management business logic.
type Service struct {
	repo Repository
}

// NewService creates an inventory-management service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// CreateAsset creates a new asset, validating the category against the vertical config.
func (s *Service) CreateAsset(ctx context.Context, a *Asset) error {
	vcfg := vertical.MustFromContext(ctx)

	// Validate category exists in this vertical
	cat := vcfg.FindInventoryCategory(a.CategoryID)
	if cat == nil {
		return fmt.Errorf("%w: %q is not valid for vertical %s", ErrInvalidCategory, a.CategoryID, vcfg.ID)
	}

	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return s.repo.CreateAsset(ctx, a)
}

// GetAsset retrieves an asset by ID.
func (s *Service) GetAsset(ctx context.Context, tenantID, id uuid.UUID) (*Asset, error) {
	return s.repo.GetAsset(ctx, tenantID, id)
}

// ListAssets lists assets for a tenant.
func (s *Service) ListAssets(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]Asset, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListAssets(ctx, tenantID, status, limit, offset)
}

// CheckOutAsset checks out an asset, verifying it is available.
func (s *Service) CheckOutAsset(ctx context.Context, c *AssetCheckout) error {
	asset, err := s.repo.GetAsset(ctx, c.TenantID, c.AssetID)
	if err != nil {
		return fmt.Errorf("get asset: %w", err)
	}

	if asset.Status != "available" {
		return fmt.Errorf("%w: current status is %q", ErrAssetNotAvailable, asset.Status)
	}

	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}

	// Create checkout record and update asset status
	if err := s.repo.CheckOutAsset(ctx, c); err != nil {
		return err
	}
	return s.repo.UpdateAssetStatus(ctx, c.TenantID, c.AssetID, "checked_out")
}

// CheckInAsset checks in an asset and sets status back to available.
func (s *Service) CheckInAsset(ctx context.Context, tenantID, checkoutID, assetID uuid.UUID, conditionIn string) error {
	if err := s.repo.CheckInAsset(ctx, checkoutID, conditionIn); err != nil {
		return err
	}
	return s.repo.UpdateAssetStatus(ctx, tenantID, assetID, "available")
}

// GetInventoryCategories returns the vertical's inventory categories.
func (s *Service) GetInventoryCategories(ctx context.Context) []vertical.InventoryCategory {
	vcfg := vertical.MustFromContext(ctx)
	return vcfg.InventoryCategories
}

// GetCheckout retrieves a single checkout record by ID.
func (s *Service) GetCheckout(ctx context.Context, id uuid.UUID) (*AssetCheckout, error) {
	return s.repo.GetCheckout(ctx, id)
}

// ListCheckouts lists all checkouts for an asset.
func (s *Service) ListCheckouts(ctx context.Context, assetID uuid.UUID) ([]AssetCheckout, error) {
	return s.repo.ListCheckouts(ctx, assetID)
}
