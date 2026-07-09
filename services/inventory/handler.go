package inventory

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	inventoryv1 "github.com/wegofwd2020/thittam/gen/inventory/v1"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/tenant"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Handler implements the gRPC InventoryService.
type Handler struct {
	inventoryv1.UnimplementedInventoryServiceServer
	svc  *Service
	perm interceptor.PermissionChecker // optional; nil skips RequirePermission
}

// NewHandler creates a Handler wrapping the given Service.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// WithPermissionChecker attaches an IAM permission checker so handlers can
// enforce ADR-014 project-scoped RBAC. Returns the receiver for chaining.
func (h *Handler) WithPermissionChecker(p interceptor.PermissionChecker) *Handler {
	h.perm = p
	return h
}

// Compile-time interface check.
var _ inventoryv1.InventoryServiceServer = (*Handler)(nil)

// --- Assets ---

func (h *Handler) CreateAsset(ctx context.Context, req *inventoryv1.CreateAssetRequest) (*inventoryv1.Asset, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "inventory:write"); err != nil {
		return nil, err
	}

	purchaseCost := decimal.Zero
	if s := req.GetPurchaseCost(); s != "" {
		var err error
		purchaseCost, err = decimal.NewFromString(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid purchase_cost: must be a decimal string")
		}
	}

	var purchaseDate *string
	if s := req.GetPurchaseDate(); s != "" {
		purchaseDate = &s
	}

	a := &Asset{
		TenantID:      tenantID,
		AssetCode:     req.GetAssetCode(),
		Name:          req.GetName(),
		CategoryID:    req.GetCategoryId(),
		Description:   req.GetDescription(),
		OwnershipType: req.GetOwnershipType(),
		Status:        "available",
		PurchaseDate:  purchaseDate,
		PurchaseCost:  purchaseCost,
		SerialNumber:  req.GetSerialNumber(),
	}

	if err := h.svc.CreateAsset(ctx, a); err != nil {
		return nil, grpcErr(err)
	}

	out, err := h.svc.GetAsset(ctx, tenantID, a.ID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return assetToProto(out), nil
}

func (h *Handler) GetAsset(ctx context.Context, req *inventoryv1.GetAssetRequest) (*inventoryv1.Asset, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid asset ID")
	}

	a, err := h.svc.GetAsset(ctx, tenantID, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return assetToProto(a), nil
}

func (h *Handler) ListAssets(ctx context.Context, req *inventoryv1.ListAssetsRequest) (*inventoryv1.ListAssetsResponse, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	assets, err := h.svc.ListAssets(ctx, tenantID, req.GetStatus(), int(req.GetLimit()), 0)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*inventoryv1.Asset, len(assets))
	for i := range assets {
		out[i] = assetToProto(&assets[i])
	}
	return &inventoryv1.ListAssetsResponse{Assets: out}, nil
}

// --- Checkouts ---

func (h *Handler) CheckOutAsset(ctx context.Context, req *inventoryv1.CheckOutAssetRequest) (*inventoryv1.AssetCheckout, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "inventory:checkout"); err != nil {
		return nil, err
	}

	assetID, err := uuid.Parse(req.GetAssetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid asset_id")
	}

	productionID, err := uuid.Parse(req.GetProductionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid production_id")
	}

	checkedOutTo, err := uuid.Parse(req.GetCheckedOutTo())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid checked_out_to: must be a valid UUID")
	}

	var expectedReturn *string
	if s := req.GetExpectedReturn(); s != "" {
		expectedReturn = &s
	}

	c := &AssetCheckout{
		AssetID:        assetID,
		ProductionID:   productionID,
		TenantID:       tenantID,
		CheckedOutTo:   checkedOutTo,
		ExpectedReturn: expectedReturn,
		ConditionOut:   req.GetConditionOut(),
	}

	if err := h.svc.CheckOutAsset(ctx, c); err != nil {
		return nil, grpcErr(err)
	}

	out, err := h.svc.GetCheckout(ctx, c.ID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return checkoutToProto(out), nil
}

func (h *Handler) CheckInAsset(ctx context.Context, req *inventoryv1.CheckInAssetRequest) (*inventoryv1.AssetCheckout, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "inventory:checkout"); err != nil {
		return nil, err
	}

	checkoutID, err := uuid.Parse(req.GetCheckoutId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid checkout_id")
	}

	assetID, err := uuid.Parse(req.GetAssetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid asset_id")
	}

	if err := h.svc.CheckInAsset(ctx, tenantID, checkoutID, assetID, req.GetConditionIn()); err != nil {
		return nil, grpcErr(err)
	}

	out, err := h.svc.GetCheckout(ctx, checkoutID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return checkoutToProto(out), nil
}

func (h *Handler) ListCheckouts(ctx context.Context, req *inventoryv1.ListCheckoutsRequest) (*inventoryv1.ListCheckoutsResponse, error) {
	assetID, err := uuid.Parse(req.GetAssetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid asset_id")
	}

	checkouts, err := h.svc.ListCheckouts(ctx, assetID)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*inventoryv1.AssetCheckout, len(checkouts))
	for i := range checkouts {
		out[i] = checkoutToProto(&checkouts[i])
	}
	return &inventoryv1.ListCheckoutsResponse{Checkouts: out}, nil
}

// --- Vertical metadata ---

func (h *Handler) GetInventoryCategories(ctx context.Context, _ *inventoryv1.GetInventoryCategoriesRequest) (*inventoryv1.GetInventoryCategoriesResponse, error) {
	cats := h.svc.GetInventoryCategories(ctx)
	out := make([]*inventoryv1.InventoryCategoryInfo, len(cats))
	for i, c := range cats {
		var depYears int32
		if c.DepreciationYears != nil {
			depYears = int32(*c.DepreciationYears)
		}
		out[i] = &inventoryv1.InventoryCategoryInfo{
			Id:                c.ID,
			Label:             c.Label,
			IsTrackable:       c.IsTrackable,
			DepreciationYears: depYears,
		}
	}
	return &inventoryv1.GetInventoryCategoriesResponse{Categories: out}, nil
}

// --- proto conversion helpers ---

func assetToProto(a *Asset) *inventoryv1.Asset {
	out := &inventoryv1.Asset{
		Id:            a.ID.String(),
		TenantId:      a.TenantID.String(),
		AssetCode:     a.AssetCode,
		Name:          a.Name,
		CategoryId:    a.CategoryID,
		Description:   a.Description,
		OwnershipType: a.OwnershipType,
		Status:        a.Status,
		SerialNumber:  a.SerialNumber,
		PurchaseCost:  a.PurchaseCost.StringFixed(2),
		CreatedAt:     timestamppb.New(a.CreatedAt),
		UpdatedAt:     timestamppb.New(a.UpdatedAt),
	}
	if a.PurchaseDate != nil {
		out.PurchaseDate = *a.PurchaseDate
	}
	return out
}

func checkoutToProto(c *AssetCheckout) *inventoryv1.AssetCheckout {
	out := &inventoryv1.AssetCheckout{
		Id:           c.ID.String(),
		AssetId:      c.AssetID.String(),
		ProductionId: c.ProductionID.String(),
		TenantId:     c.TenantID.String(),
		CheckedOutTo: c.CheckedOutTo.String(),
		CheckedOutAt: timestamppb.New(c.CheckedOutAt),
		ConditionOut: c.ConditionOut,
		ConditionIn:  c.ConditionIn,
	}
	if c.ExpectedReturn != nil {
		out.ExpectedReturn = *c.ExpectedReturn
	}
	if c.CheckedInAt != nil {
		out.CheckedInAt = timestamppb.New(*c.CheckedInAt)
	}
	return out
}

// grpcErr maps domain sentinel errors to gRPC status codes.
func grpcErr(err error) error {
	switch {
	case errors.Is(err, ErrAssetNotFound):
		return status.Error(codes.NotFound, "asset not found")
	case errors.Is(err, ErrAssetNotAvailable):
		return status.Error(codes.FailedPrecondition, "asset is not available for checkout")
	case errors.Is(err, ErrAlreadyCheckedOut):
		return status.Error(codes.FailedPrecondition, "asset is already checked out")
	case errors.Is(err, ErrInvalidCategory):
		return status.Error(codes.InvalidArgument, "invalid category for this vertical")
	case errors.Is(err, vertical.ErrNotFound):
		return status.Error(codes.FailedPrecondition, "vertical config not found for tenant")
	}
	return status.Error(codes.Internal, "internal error")
}
