package inventory

import (
	"context"
	"testing"

	"github.com/google/uuid"
	inventoryv1 "github.com/wegofwd2020/thittam/gen/inventory/v1"
	"github.com/wegofwd2020/thittam/pkg/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func ctxWithTenant(tenantID uuid.UUID) context.Context {
	return tenant.WithID(ctxWithVertical(), tenantID)
}

func newHandler() *Handler {
	return NewHandler(NewService(&mockRepo{}))
}

// --- CreateAsset ---

func TestHandler_CreateAsset_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := newHandler()

	resp, err := h.CreateAsset(ctxWithTenant(tenantID), &inventoryv1.CreateAssetRequest{
		AssetCode:     "CAM-001",
		Name:          "RED V-Raptor",
		CategoryId:    "camera",
		OwnershipType: "owned",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
	assert.Equal(t, tenantID.String(), resp.GetTenantId())
}

func TestHandler_CreateAsset_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateAsset(ctxWithVertical(), &inventoryv1.CreateAssetRequest{
		AssetCode:  "CAM-001",
		CategoryId: "camera",
	})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// --- GetAsset ---

func TestHandler_GetAsset_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	assetID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getAssetFn: func(_ context.Context, tid, id uuid.UUID) (*Asset, error) {
			return &Asset{ID: id, TenantID: tid, Status: "available"}, nil
		},
	}))

	resp, err := h.GetAsset(ctxWithTenant(tenantID), &inventoryv1.GetAssetRequest{Id: assetID.String()})
	require.NoError(t, err)
	assert.Equal(t, assetID.String(), resp.GetId())
}

func TestHandler_GetAsset_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetAsset(ctxWithVertical(), &inventoryv1.GetAssetRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_GetAsset_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetAsset(ctxWithTenant(uuid.New()), &inventoryv1.GetAssetRequest{Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ListAssets ---

func TestHandler_ListAssets_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listAssetsFn: func(_ context.Context, _ uuid.UUID, _ string, _, _ int) ([]Asset, error) {
			return []Asset{{ID: uuid.New(), TenantID: tenantID, Status: "available"}}, nil
		},
	}))

	resp, err := h.ListAssets(ctxWithTenant(tenantID), &inventoryv1.ListAssetsRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetAssets(), 1)
}

func TestHandler_ListAssets_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListAssets(ctxWithVertical(), &inventoryv1.ListAssetsRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// --- CheckOutAsset ---

func TestHandler_CheckOutAsset_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	assetID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getAssetFn: func(_ context.Context, tid, id uuid.UUID) (*Asset, error) {
			return &Asset{ID: id, TenantID: tid, Status: "available"}, nil
		},
	}))

	resp, err := h.CheckOutAsset(ctxWithTenant(tenantID), &inventoryv1.CheckOutAssetRequest{
		AssetId:      assetID.String(),
		ProductionId: uuid.New().String(),
		CheckedOutTo: uuid.New().String(),
		ConditionOut: "good",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
}

func TestHandler_CheckOutAsset_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CheckOutAsset(ctxWithVertical(), &inventoryv1.CheckOutAssetRequest{
		AssetId:      uuid.New().String(),
		ProductionId: uuid.New().String(),
		CheckedOutTo: uuid.New().String(),
	})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_CheckOutAsset_InvalidAssetID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CheckOutAsset(ctxWithTenant(uuid.New()), &inventoryv1.CheckOutAssetRequest{
		AssetId: "bad", ProductionId: uuid.New().String(), CheckedOutTo: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CheckOutAsset_InvalidProductionID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CheckOutAsset(ctxWithTenant(uuid.New()), &inventoryv1.CheckOutAssetRequest{
		AssetId: uuid.New().String(), ProductionId: "bad", CheckedOutTo: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CheckOutAsset_InvalidCheckedOutTo(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CheckOutAsset(ctxWithTenant(uuid.New()), &inventoryv1.CheckOutAssetRequest{
		AssetId: uuid.New().String(), ProductionId: uuid.New().String(), CheckedOutTo: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- CheckInAsset ---

func TestHandler_CheckInAsset_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	assetID := uuid.New()
	checkoutID := uuid.New()
	h := newHandler()

	resp, err := h.CheckInAsset(ctxWithTenant(tenantID), &inventoryv1.CheckInAssetRequest{
		CheckoutId:  checkoutID.String(),
		AssetId:     assetID.String(),
		ConditionIn: "good",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_CheckInAsset_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CheckInAsset(ctxWithVertical(), &inventoryv1.CheckInAssetRequest{
		CheckoutId: uuid.New().String(), AssetId: uuid.New().String(),
	})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_CheckInAsset_InvalidCheckoutID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CheckInAsset(ctxWithTenant(uuid.New()), &inventoryv1.CheckInAssetRequest{
		CheckoutId: "bad", AssetId: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CheckInAsset_InvalidAssetID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CheckInAsset(ctxWithTenant(uuid.New()), &inventoryv1.CheckInAssetRequest{
		CheckoutId: uuid.New().String(), AssetId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ListCheckouts ---

func TestHandler_ListCheckouts_Success(t *testing.T) {
	t.Parallel()
	assetID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listCheckoutsFn: func(_ context.Context, _ uuid.UUID) ([]AssetCheckout, error) {
			return []AssetCheckout{{ID: uuid.New(), AssetID: assetID}}, nil
		},
	}))

	resp, err := h.ListCheckouts(context.Background(), &inventoryv1.ListCheckoutsRequest{AssetId: assetID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetCheckouts(), 1)
}

func TestHandler_ListCheckouts_InvalidAssetID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListCheckouts(context.Background(), &inventoryv1.ListCheckoutsRequest{AssetId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- GetInventoryCategories ---

func TestHandler_GetInventoryCategories(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().GetInventoryCategories(ctxWithVertical(), &inventoryv1.GetInventoryCategoriesRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetCategories(), 2)
	assert.Equal(t, "camera", resp.GetCategories()[0].GetId())
}

// --- grpcErr ---

func TestGrpcErr_AllCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err      error
		wantCode codes.Code
	}{
		{ErrAssetNotFound, codes.NotFound},
		{ErrAssetNotAvailable, codes.FailedPrecondition},
		{ErrInvalidCategory, codes.InvalidArgument},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.wantCode, status.Code(grpcErr(tc.err)))
	}
}
