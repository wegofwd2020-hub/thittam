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
func (s *Service) ListAssets(ctx context.Context, tenantID uuid.UUID, status, categoryID, search string, limit, offset int) ([]Asset, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListAssets(ctx, tenantID, status, categoryID, search, limit, offset)
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

// CheckInAsset resolves the open checkout for the asset, records the
// check-in (including any reported damage), and updates the asset status
// accordingly.
func (s *Service) CheckInAsset(ctx context.Context, tenantID, assetID uuid.UUID, in CheckInInput) (*AssetCheckout, error) {
	co, err := s.repo.GetActiveCheckout(ctx, tenantID, assetID)
	if err != nil {
		return nil, err // ErrNoActiveCheckout maps to FailedPrecondition
	}
	updated, err := s.repo.CheckInAsset(ctx, tenantID, co.ID, in)
	if err != nil {
		return nil, err
	}
	status := "available"
	if in.ReportDamage {
		status = "under_repair"
	}
	if err := s.repo.UpdateAssetStatus(ctx, tenantID, assetID, status); err != nil {
		return nil, err
	}
	return updated, nil
}

// GetInventoryCategories returns the vertical's inventory categories.
func (s *Service) GetInventoryCategories(ctx context.Context) []vertical.InventoryCategory {
	vcfg := vertical.MustFromContext(ctx)
	return vcfg.InventoryCategories
}

// GetCheckout retrieves a single checkout record by ID, scoped to the
// caller's tenant.
func (s *Service) GetCheckout(ctx context.Context, tenantID, id uuid.UUID) (*AssetCheckout, error) {
	return s.repo.GetCheckout(ctx, tenantID, id)
}

// ListCheckouts lists checkouts for an asset, scoped to the caller's tenant.
// limit is clamped to (0, 200]; the DB LIMIT enforces the cap. after is an
// opaque RFC3339 cursor on checked_out_at for keyset pagination.
func (s *Service) ListCheckouts(ctx context.Context, tenantID, assetID uuid.UUID, limit int, after string) ([]AssetCheckout, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	return s.repo.ListCheckouts(ctx, tenantID, assetID, limit, after)
}
