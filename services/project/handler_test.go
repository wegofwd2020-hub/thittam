package project

import (
	"context"
	"testing"

	"github.com/google/uuid"
	projectv1 "github.com/wegofwd2020/thittam/gen/project/v1"
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

// callerCtx is for handlers that do not check tenant.IDFromContext themselves
// (e.g. UpdatePhaseStatus, RemoveCrewMember) but still hit RequirePermission.
func callerCtx() context.Context {
	return interceptor.WithCaller(ctxWithVertical(), interceptor.CallerInfo{UserID: uuid.New(), TenantID: uuid.New()})
}

func newHandler() *Handler {
	return NewHandler(NewService(&mockRepo{})).WithPermissionChecker(allowAllPerm{})
}

// --- CreateProduction ---

func TestHandler_CreateProduction_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := newHandler()

	resp, err := h.CreateProduction(ctxWithTenant(tenantID), &projectv1.CreateProductionRequest{
		Title:       "Ponniyin Selvan 3",
		Description: "Epic historical film",
		Genre:       "historical",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
	assert.Equal(t, tenantID.String(), resp.GetTenantId())
}

func TestHandler_CreateProduction_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateProduction(ctxWithVertical(), &projectv1.CreateProductionRequest{Title: "Film"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// --- GetProduction ---

func TestHandler_GetProduction_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	prodID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getProductionFn: func(_ context.Context, tid, id uuid.UUID) (*Production, error) {
			return &Production{ID: id, TenantID: tid, Title: "Test", Status: "active"}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.GetProduction(ctxWithTenant(tenantID), &projectv1.GetProductionRequest{Id: prodID.String()})
	require.NoError(t, err)
	assert.Equal(t, prodID.String(), resp.GetId())
}

func TestHandler_GetProduction_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetProduction(ctxWithVertical(), &projectv1.GetProductionRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_GetProduction_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetProduction(ctxWithTenant(uuid.New()), &projectv1.GetProductionRequest{Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetProduction_NotFound(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		getProductionFn: func(_ context.Context, _, _ uuid.UUID) (*Production, error) {
			return nil, ErrProductionNotFound
		},
	})).WithPermissionChecker(allowAllPerm{})
	_, err := h.GetProduction(ctxWithTenant(uuid.New()), &projectv1.GetProductionRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestHandler_GetProduction_Denied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getProductionFn: func(context.Context, uuid.UUID, uuid.UUID) (*Production, error) {
			t.Fatal("gate must fire before the repository is read")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.GetProduction(ctxWithTenant(tenantID), &projectv1.GetProductionRequest{
		Id: uuid.New().String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- ListProductions ---

func TestHandler_ListProductions_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listProductionsFn: func(_ context.Context, _ uuid.UUID, _ string, _, _ int) ([]Production, error) {
			return []Production{{ID: uuid.New(), TenantID: tenantID, Status: "active"}}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.ListProductions(ctxWithTenant(tenantID), &projectv1.ListProductionsRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetProductions(), 1)
}

func TestHandler_ListProductions_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListProductions(ctxWithVertical(), &projectv1.ListProductionsRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_ListProductions_Denied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listProductionsFn: func(context.Context, uuid.UUID, string, int, int) ([]Production, error) {
			t.Fatal("gate must fire before the repository is read")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.ListProductions(ctxWithTenant(tenantID), &projectv1.ListProductionsRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- UpdateProduction ---

func TestHandler_UpdateProduction_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	prodID := uuid.New()
	h := newHandler()

	resp, err := h.UpdateProduction(ctxWithTenant(tenantID), &projectv1.UpdateProductionRequest{
		Id:    prodID.String(),
		Title: "Updated Title",
	})
	require.NoError(t, err)
	assert.Equal(t, prodID.String(), resp.GetId())
}

func TestHandler_UpdateProduction_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().UpdateProduction(ctxWithVertical(), &projectv1.UpdateProductionRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_UpdateProduction_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().UpdateProduction(ctxWithTenant(uuid.New()), &projectv1.UpdateProductionRequest{Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ArchiveProduction ---

func TestHandler_ArchiveProduction_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	prodID := uuid.New()
	h := newHandler()

	resp, err := h.ArchiveProduction(ctxWithTenant(tenantID), &projectv1.ArchiveProductionRequest{Id: prodID.String()})
	require.NoError(t, err)
	assert.Equal(t, prodID.String(), resp.GetId())
}

func TestHandler_ArchiveProduction_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ArchiveProduction(ctxWithVertical(), &projectv1.ArchiveProductionRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_ArchiveProduction_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ArchiveProduction(ctxWithTenant(uuid.New()), &projectv1.ArchiveProductionRequest{Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- CreatePhase ---

func TestHandler_CreatePhase_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	prodID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getPhaseFn: func(_ context.Context, id uuid.UUID) (*Phase, error) {
			return &Phase{ID: id, PhaseType: "production", Status: "pending"}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.CreatePhase(ctxWithTenant(tenantID), &projectv1.CreatePhaseRequest{
		ProductionId: prodID.String(),
		PhaseType:    "production",
	})
	require.NoError(t, err)
	assert.Equal(t, "production", resp.GetPhaseType())
}

func TestHandler_CreatePhase_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreatePhase(ctxWithVertical(), &projectv1.CreatePhaseRequest{
		ProductionId: uuid.New().String(),
		PhaseType:    "production",
	})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_CreatePhase_InvalidProductionID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreatePhase(ctxWithTenant(uuid.New()), &projectv1.CreatePhaseRequest{
		ProductionId: "bad",
		PhaseType:    "production",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ListPhases ---

func TestHandler_ListPhases_Success(t *testing.T) {
	t.Parallel()
	prodID := uuid.New()
	h := newHandler()

	resp, err := h.ListPhases(ctxWithTenant(uuid.New()), &projectv1.ListPhasesRequest{ProductionId: prodID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetPhases(), 0)
}

func TestHandler_ListPhases_InvalidProductionID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListPhases(ctxWithTenant(uuid.New()), &projectv1.ListPhasesRequest{ProductionId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// tripwireListPhasesRepo wraps mockRepo to trap ListPhases. Unlike the other
// Repository methods, mockRepo.ListPhases has no injectable *Fn hook — it
// always returns (nil, nil) — so a denial test cannot prove the gate fires
// before the read via the usual field-override pattern. Embedding mockRepo
// and overriding just this one method gets the same tripwire guarantee.
type tripwireListPhasesRepo struct {
	mockRepo
	t *testing.T
}

func (r *tripwireListPhasesRepo) ListPhases(context.Context, uuid.UUID, int, int) ([]Phase, error) {
	r.t.Fatal("gate must fire before the repository is read")
	return nil, nil
}

func TestHandler_ListPhases_Denied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&tripwireListPhasesRepo{t: t})).WithPermissionChecker(denyPerm{})

	_, err := h.ListPhases(ctxWithTenant(tenantID), &projectv1.ListPhasesRequest{
		ProductionId: uuid.New().String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- UpdatePhaseStatus ---

func TestHandler_UpdatePhaseStatus_Success(t *testing.T) {
	t.Parallel()
	phaseID := uuid.New()
	callCount := 0
	h := NewHandler(NewService(&mockRepo{
		getPhaseFn: func(_ context.Context, id uuid.UUID) (*Phase, error) {
			callCount++
			phaseType := "production"
			if callCount > 1 {
				phaseType = "post_production"
			}
			return &Phase{ID: id, PhaseType: phaseType, Status: "active"}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.UpdatePhaseStatus(callerCtx(), &projectv1.UpdatePhaseStatusRequest{
		PhaseId:      phaseID.String(),
		NewPhaseType: "post_production",
	})
	require.NoError(t, err)
	assert.Equal(t, "post_production", resp.GetPhaseType())
}

func TestHandler_UpdatePhaseStatus_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().UpdatePhaseStatus(callerCtx(), &projectv1.UpdatePhaseStatusRequest{PhaseId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- AddCrewMember ---

func TestHandler_AddCrewMember_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	prodID := uuid.New()
	h := newHandler()

	resp, err := h.AddCrewMember(ctxWithTenant(tenantID), &projectv1.AddCrewMemberRequest{
		ProductionId: prodID.String(),
		Name:         "Jane Doe",
		Role:         "Director",
		Department:   "Direction",
		DayRate:      "50000.00",
	})
	require.NoError(t, err)
	assert.Equal(t, "Jane Doe", resp.GetName())
}

func TestHandler_AddCrewMember_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().AddCrewMember(ctxWithVertical(), &projectv1.AddCrewMemberRequest{
		ProductionId: uuid.New().String(),
		DayRate:      "1000.00",
	})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_AddCrewMember_InvalidProductionID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().AddCrewMember(ctxWithTenant(uuid.New()), &projectv1.AddCrewMemberRequest{
		ProductionId: "bad",
		DayRate:      "1000.00",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_AddCrewMember_InvalidDayRate(t *testing.T) {
	t.Parallel()
	_, err := newHandler().AddCrewMember(ctxWithTenant(uuid.New()), &projectv1.AddCrewMemberRequest{
		ProductionId: uuid.New().String(),
		DayRate:      "not-a-number",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ListCrewMembers ---

func TestHandler_ListCrewMembers_Success(t *testing.T) {
	t.Parallel()
	prodID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listCrewFn: func(_ context.Context, _ uuid.UUID) ([]CrewMember, error) {
			return []CrewMember{{ID: uuid.New(), Name: "John", Role: "DP"}}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.ListCrewMembers(ctxWithTenant(uuid.New()), &projectv1.ListCrewMembersRequest{ProductionId: prodID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetMembers(), 1)
}

func TestHandler_ListCrewMembers_InvalidProductionID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListCrewMembers(ctxWithTenant(uuid.New()), &projectv1.ListCrewMembersRequest{ProductionId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ListCrewMembers_Denied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listCrewFn: func(context.Context, uuid.UUID) ([]CrewMember, error) {
			t.Fatal("gate must fire before the repository is read")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.ListCrewMembers(ctxWithTenant(tenantID), &projectv1.ListCrewMembersRequest{
		ProductionId: uuid.New().String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- RemoveCrewMember ---

func TestHandler_RemoveCrewMember_Success(t *testing.T) {
	t.Parallel()
	memberID := uuid.New()
	resp, err := newHandler().RemoveCrewMember(callerCtx(), &projectv1.RemoveCrewMemberRequest{Id: memberID.String()})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_RemoveCrewMember_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().RemoveCrewMember(callerCtx(), &projectv1.RemoveCrewMemberRequest{Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- Vertical metadata ---

func TestHandler_GetEntityLabels(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().GetEntityLabels(ctxWithVertical(), &projectv1.GetEntityLabelsRequest{})
	require.NoError(t, err)
	assert.Equal(t, "Production", resp.GetProject())
	assert.Equal(t, "Crew Member", resp.GetTeamMember())
}

func TestHandler_GetPhaseTypes(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().GetPhaseTypes(ctxWithVertical(), &projectv1.GetPhaseTypesRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetPhaseTypes(), 5)
	assert.Equal(t, "development", resp.GetPhaseTypes()[0].GetId())
}

// --- fail-closed regression (#138) ---

// TestCreateProduction_NoPermissionChecker_Denies proves the fail-open is
// closed: a handler with no PermissionChecker wired must deny writes, not
// silently skip the check. Without this test, someone could reinstate the
// the nil-checker guard around RequirePermission and nothing would catch it.
func TestCreateProduction_NoPermissionChecker_Denies(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{})) // deliberately no WithPermissionChecker

	_, err := h.CreateProduction(ctxWithTenant(uuid.New()), &projectv1.CreateProductionRequest{
		Title: "Ponniyin Selvan 3",
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
		{ErrProductionNotFound, codes.NotFound},
		{ErrPhaseNotFound, codes.NotFound},
		{ErrCrewNotFound, codes.NotFound},
		{ErrDuplicateSlug, codes.AlreadyExists},
		{ErrInvalidPhaseType, codes.InvalidArgument},
		{ErrInvalidTransition, codes.FailedPrecondition},
		{ErrProductionArchived, codes.FailedPrecondition},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.wantCode, status.Code(grpcErr(tc.err)))
	}
}
