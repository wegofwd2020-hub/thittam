package inventory

import (
	"context"
	"testing"

	"github.com/google/uuid"
	inventoryv1 "github.com/wegofwd2020/thittam/gen/inventory/v1"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// allowAllPerm grants every permission. Authorization semantics are covered by
// pkg/interceptor's own tests; these handler tests exercise handler logic.
type allowAllPerm struct{}

func (allowAllPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return true, nil
}

// denyPerm denies every permission, so a denial test can prove the gate fires
// before the repository is reached.
type denyPerm struct{}

func (denyPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return false, nil
}

// ctxWithTenant injects vertical config, a tenant ID, and a synthetic caller
// (RequirePermission now runs unconditionally, so every handler test needs a
// caller in context or it fails Unauthenticated before reaching handler logic).
func ctxWithTenant(tenantID uuid.UUID) context.Context {
	ctx := tenant.WithID(ctxWithVertical(), tenantID)
	return interceptor.WithCaller(ctx, interceptor.CallerInfo{UserID: uuid.New(), TenantID: tenantID})
}

func newHandler() *Handler {
	return NewHandler(NewService(&mockRepo{})).WithPermissionChecker(allowAllPerm{})
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
	})).WithPermissionChecker(allowAllPerm{})

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

func TestHandler_GetAsset_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		getAssetFn: func(_ context.Context, _, _ uuid.UUID) (*Asset, error) {
			t.Fatal("repository reached: GetAsset must deny before querying")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.GetAsset(ctxWithTenant(uuid.New()), &inventoryv1.GetAssetRequest{
		Id: uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- ListAssets ---

func TestHandler_ListAssets_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listAssetsFn: func(_ context.Context, _ uuid.UUID, _ string, _, _ int) ([]Asset, error) {
			return []Asset{{ID: uuid.New(), TenantID: tenantID, Status: "available"}}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.ListAssets(ctxWithTenant(tenantID), &inventoryv1.ListAssetsRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetAssets(), 1)
}

func TestHandler_ListAssets_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListAssets(ctxWithVertical(), &inventoryv1.ListAssetsRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_ListAssets_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		listAssetsFn: func(_ context.Context, _ uuid.UUID, _ string, _, _ int) ([]Asset, error) {
			t.Fatal("repository reached: ListAssets must deny before querying")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.ListAssets(ctxWithTenant(uuid.New()), &inventoryv1.ListAssetsRequest{})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
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
	})).WithPermissionChecker(allowAllPerm{})

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
		listCheckoutsFn: func(_ context.Context, _, _ uuid.UUID) ([]AssetCheckout, error) {
			return []AssetCheckout{{ID: uuid.New(), AssetID: assetID}}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.ListCheckouts(ctxWithTenant(uuid.New()), &inventoryv1.ListCheckoutsRequest{AssetId: assetID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetCheckouts(), 1)
}

func TestHandler_ListCheckouts_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListCheckouts(context.Background(), &inventoryv1.ListCheckoutsRequest{AssetId: uuid.New().String()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_ListCheckouts_InvalidAssetID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListCheckouts(ctxWithTenant(uuid.New()), &inventoryv1.ListCheckoutsRequest{AssetId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ListCheckouts_Denied(t *testing.T) {
	t.Parallel()
	assetID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listCheckoutsFn: func(_ context.Context, _, _ uuid.UUID) ([]AssetCheckout, error) {
			t.Fatal("repository reached: ListCheckouts must deny before querying")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.ListCheckouts(ctxWithTenant(uuid.New()), &inventoryv1.ListCheckoutsRequest{
		AssetId: assetID.String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- cross-tenant regression (#157) ---

// checkoutRecordingRepo records the tenant ID each checkout query receives.
// inventory's mockRepo returns a usable AssetCheckout by default, so a
// status-code assertion alone would pass against unscoped code.
type checkoutRecordingRepo struct {
	mockRepo
	gotListTenant    uuid.UUID
	gotGetTenant     uuid.UUID
	gotCheckInTenant uuid.UUID
}

func (r *checkoutRecordingRepo) ListCheckouts(ctx context.Context, tenantID, assetID uuid.UUID) ([]AssetCheckout, error) {
	r.gotListTenant = tenantID
	return nil, nil
}

func (r *checkoutRecordingRepo) GetCheckout(ctx context.Context, tenantID, id uuid.UUID) (*AssetCheckout, error) {
	r.gotGetTenant = tenantID
	return &AssetCheckout{ID: id, TenantID: tenantID}, nil
}

func (r *checkoutRecordingRepo) CheckInAsset(ctx context.Context, tenantID, checkoutID uuid.UUID, conditionIn string) error {
	r.gotCheckInTenant = tenantID
	return nil
}

func TestHandler_ListCheckouts_PassesCallerTenantToRepo(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	repo := &checkoutRecordingRepo{}
	h := NewHandler(NewService(repo)).WithPermissionChecker(allowAllPerm{})

	_, err := h.ListCheckouts(ctxWithTenant(callerTenant), &inventoryv1.ListCheckoutsRequest{
		AssetId: uuid.New().String(),
	})

	require.NoError(t, err)
	require.Equal(t, callerTenant, repo.gotListTenant,
		"ListCheckouts must query with the caller's tenant")
}

func TestHandler_CheckInAsset_PassesCallerTenantToRepo(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	repo := &checkoutRecordingRepo{}
	h := NewHandler(NewService(repo)).WithPermissionChecker(allowAllPerm{})

	_, err := h.CheckInAsset(ctxWithTenant(callerTenant), &inventoryv1.CheckInAssetRequest{
		CheckoutId:  uuid.New().String(),
		AssetId:     uuid.New().String(),
		ConditionIn: "good",
	})

	require.NoError(t, err)
	require.Equal(t, callerTenant, repo.gotCheckInTenant,
		"CheckInAsset must write with the caller's tenant, which Service already held")
	require.Equal(t, callerTenant, repo.gotGetTenant,
		"the read-back after check-in must also be tenant-scoped")
}

// --- GetInventoryCategories ---

func TestHandler_GetInventoryCategories(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().GetInventoryCategories(ctxWithVertical(), &inventoryv1.GetInventoryCategoriesRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetCategories(), 2)
	assert.Equal(t, "camera", resp.GetCategories()[0].GetId())
}

// --- fail-closed regression (#138) ---

// TestCreateAsset_NoPermissionChecker_Denies proves the fail-open is closed:
// a handler with no PermissionChecker wired must deny writes, not silently
// skip the check. Without this test, someone could reinstate the
// the nil-checker guard around RequirePermission and nothing would catch it.
func TestCreateAsset_NoPermissionChecker_Denies(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{})) // deliberately no WithPermissionChecker

	_, err := h.CreateAsset(ctxWithTenant(uuid.New()), &inventoryv1.CreateAssetRequest{
		AssetCode:     "CAM-001",
		Name:          "RED V-Raptor",
		CategoryId:    "camera",
		OwnershipType: "owned",
	})
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err), "a missing permission checker must deny, not skip the check")
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
