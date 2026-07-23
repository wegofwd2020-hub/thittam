package project

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	projectv1 "github.com/wegofwd2020/thittam/gen/project/v1"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/tenant"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Handler implements the gRPC ProjectService.
type Handler struct {
	projectv1.UnimplementedProjectServiceServer
	svc  *Service
	perm interceptor.PermissionChecker // nil fails closed: RequirePermission returns Internal (#138)
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
var _ projectv1.ProjectServiceServer = (*Handler)(nil)

// --- Productions ---

func (h *Handler) CreateProduction(ctx context.Context, req *projectv1.CreateProductionRequest) (*projectv1.Production, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "production:write"); err != nil {
		return nil, err
	}

	prod := &Production{
		TenantID:    tenantID,
		Title:       req.GetTitle(),
		Slug:        req.GetTitle(), // slug normalised from title; a utility can replace this
		Description: req.GetDescription(),
		Genre:       req.GetGenre(),
		// Default status is the first phase ("development"). Must be one of the
		// values allowed by the productions_status_check CHECK constraint:
		// development | pre_production | production | post_production | released | archived.
		// "active" was used here historically and silently failed every INSERT.
		Status:    "development",
		CreatedBy: uuid.Nil, // populated by auth interceptor once wired
	}

	if err := h.svc.CreateProduction(ctx, prod); err != nil {
		return nil, grpcErr(err)
	}

	out, err := h.svc.GetProduction(ctx, tenantID, prod.ID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return productionToProto(out), nil
}

func (h *Handler) GetProduction(ctx context.Context, req *projectv1.GetProductionRequest) (*projectv1.Production, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "production:read"); err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid production ID")
	}

	prod, err := h.svc.GetProduction(ctx, tenantID, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return productionToProto(prod), nil
}

func (h *Handler) ListProductions(ctx context.Context, req *projectv1.ListProductionsRequest) (*projectv1.ListProductionsResponse, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "production:read"); err != nil {
		return nil, err
	}

	limit := int(req.GetLimit())
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Cursor-based pagination (after) is not yet implemented; always use offset=0.
	prods, err := h.svc.ListProductions(ctx, tenantID, req.GetStatus(), limit, 0)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*projectv1.Production, len(prods))
	for i := range prods {
		out[i] = productionToProto(&prods[i])
	}
	return &projectv1.ListProductionsResponse{Productions: out}, nil
}

func (h *Handler) UpdateProduction(ctx context.Context, req *projectv1.UpdateProductionRequest) (*projectv1.Production, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "production:write"); err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid production ID")
	}

	prod := &Production{
		ID:          id,
		TenantID:    tenantID,
		Title:       req.GetTitle(),
		Description: req.GetDescription(),
		// Genre and Status are not in the request; empty string triggers COALESCE
		// in the SQL, preserving existing values.
	}

	if err := h.svc.UpdateProduction(ctx, prod); err != nil {
		return nil, grpcErr(err)
	}

	updated, err := h.svc.GetProduction(ctx, tenantID, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return productionToProto(updated), nil
}

func (h *Handler) ArchiveProduction(ctx context.Context, req *projectv1.ArchiveProductionRequest) (*projectv1.Production, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "production:write"); err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid production ID")
	}

	if err := h.svc.ArchiveProduction(ctx, tenantID, id); err != nil {
		return nil, grpcErr(err)
	}

	archived, err := h.svc.GetProduction(ctx, tenantID, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return productionToProto(archived), nil
}

// --- Phases ---

func (h *Handler) CreatePhase(ctx context.Context, req *projectv1.CreatePhaseRequest) (*projectv1.Phase, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "production:write"); err != nil {
		return nil, err
	}

	productionID, err := uuid.Parse(req.GetProductionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid production ID")
	}

	phase := &Phase{
		ProductionID: productionID,
		TenantID:     tenantID,
		PhaseType:    req.GetPhaseType(),
		Status:       "pending",
	}

	if err := h.svc.CreatePhase(ctx, phase); err != nil {
		return nil, grpcErr(err)
	}

	created, err := h.svc.GetPhase(ctx, tenantID, phase.ID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return phaseToProto(created), nil
}

func (h *Handler) ListPhases(ctx context.Context, req *projectv1.ListPhasesRequest) (*projectv1.ListPhasesResponse, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "production:read"); err != nil {
		return nil, err
	}

	productionID, err := uuid.Parse(req.GetProductionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid production ID")
	}

	// Proto does not carry limit/offset yet — apply a server-side cap.
	const defaultLimit = 50 // productions rarely have more than 50 phases
	phases, err := h.svc.ListPhases(ctx, tenantID, productionID, defaultLimit, 0)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*projectv1.Phase, len(phases))
	for i := range phases {
		out[i] = phaseToProto(&phases[i])
	}
	return &projectv1.ListPhasesResponse{Phases: out}, nil
}

func (h *Handler) UpdatePhaseStatus(ctx context.Context, req *projectv1.UpdatePhaseStatusRequest) (*projectv1.Phase, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "production:write"); err != nil {
		return nil, err
	}

	phaseID, err := uuid.Parse(req.GetPhaseId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid phase ID")
	}

	if err := h.svc.UpdatePhaseStatus(ctx, tenantID, phaseID, req.GetNewPhaseType()); err != nil {
		return nil, grpcErr(err)
	}

	phase, err := h.svc.GetPhase(ctx, tenantID, phaseID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return phaseToProto(phase), nil
}

// --- Crew ---

func (h *Handler) AddCrewMember(ctx context.Context, req *projectv1.AddCrewMemberRequest) (*projectv1.CrewMember, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "resource:manage"); err != nil {
		return nil, err
	}

	productionID, err := uuid.Parse(req.GetProductionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid production ID")
	}

	dayRate, err := decimal.NewFromString(req.GetDayRate())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid day_rate: must be a decimal string")
	}

	c := &CrewMember{
		ProductionID: productionID,
		TenantID:     tenantID,
		Name:         req.GetName(),
		Role:         req.GetRole(),
		Department:   req.GetDepartment(),
		DayRate:      dayRate,
		Currency:     "INR", // default; can be extended via request field
	}

	if err := h.svc.AddCrewMember(ctx, c); err != nil {
		return nil, grpcErr(err)
	}

	// Fetch the persisted crew member to return accurate DB-assigned fields.
	// Use a generous limit — we only need to find the one just inserted.
	const addLookupLimit = 200
	members, err := h.svc.ListCrewMembers(ctx, tenantID, productionID, addLookupLimit, 0)
	if err != nil {
		return nil, grpcErr(err)
	}
	for i := range members {
		if members[i].ID == c.ID {
			return crewToProto(&members[i]), nil
		}
	}
	return crewToProto(c), nil
}

func (h *Handler) ListCrewMembers(ctx context.Context, req *projectv1.ListCrewMembersRequest) (*projectv1.ListCrewMembersResponse, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "production:read"); err != nil {
		return nil, err
	}

	productionID, err := uuid.Parse(req.GetProductionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid production ID")
	}

	// Proto does not carry limit/offset yet; apply a server-side default.
	// Update the proto and pass through req fields when pagination is added.
	const defaultLimit = 50
	members, err := h.svc.ListCrewMembers(ctx, tenantID, productionID, defaultLimit, 0)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*projectv1.CrewMember, len(members))
	for i := range members {
		out[i] = crewToProto(&members[i])
	}
	return &projectv1.ListCrewMembersResponse{Members: out}, nil
}

func (h *Handler) RemoveCrewMember(ctx context.Context, req *projectv1.RemoveCrewMemberRequest) (*projectv1.RemoveCrewMemberResponse, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "resource:manage"); err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid crew member ID")
	}

	if err := h.svc.RemoveCrewMember(ctx, tenantID, id); err != nil {
		return nil, grpcErr(err)
	}
	return &projectv1.RemoveCrewMemberResponse{}, nil
}

// --- Vertical metadata ---

func (h *Handler) GetEntityLabels(ctx context.Context, _ *projectv1.GetEntityLabelsRequest) (*projectv1.EntityLabels, error) {
	labels := h.svc.GetEntityLabels(ctx)
	return &projectv1.EntityLabels{
		Project:          labels.Project,
		ProjectPlural:    labels.ProjectPlural,
		Phase:            labels.Phase,
		PhasePlural:      labels.PhasePlural,
		TeamMember:       labels.TeamMember,
		TeamMemberPlural: labels.TeamMemberPlural,
		RateLabel:        labels.RateLabel,
		RateUnit:         labels.RateUnit,
	}, nil
}

func (h *Handler) GetPhaseTypes(ctx context.Context, _ *projectv1.GetPhaseTypesRequest) (*projectv1.GetPhaseTypesResponse, error) {
	types := h.svc.GetPhaseTypes(ctx)
	out := make([]*projectv1.PhaseTypeInfo, len(types))
	for i, pt := range types {
		out[i] = &projectv1.PhaseTypeInfo{
			Id:                 pt.ID,
			Label:              pt.Label,
			Order:              int32(pt.Order),
			IsBillable:         pt.IsBillable,
			AllowedTransitions: pt.AllowedTransitions,
		}
	}
	return &projectv1.GetPhaseTypesResponse{PhaseTypes: out}, nil
}

// --- proto conversion helpers ---

func productionToProto(p *Production) *projectv1.Production {
	out := &projectv1.Production{
		Id:          p.ID.String(),
		TenantId:    p.TenantID.String(),
		Title:       p.Title,
		Slug:        p.Slug,
		Description: p.Description,
		Genre:       p.Genre,
		Language:    p.Language,
		Status:      p.Status,
		CreatedBy:   p.CreatedBy.String(),
		CreatedAt:   timestamppb.New(p.CreatedAt),
		UpdatedAt:   timestamppb.New(p.UpdatedAt),
	}
	if p.StartDate != nil {
		out.StartDate = *p.StartDate
	}
	if p.EndDate != nil {
		out.EndDate = *p.EndDate
	}
	return out
}

func phaseToProto(p *Phase) *projectv1.Phase {
	out := &projectv1.Phase{
		Id:           p.ID.String(),
		ProductionId: p.ProductionID.String(),
		PhaseType:    p.PhaseType,
		Status:       p.Status,
	}
	if p.StartDate != nil {
		out.StartDate = *p.StartDate
	}
	if p.EndDate != nil {
		out.EndDate = *p.EndDate
	}
	return out
}

func crewToProto(c *CrewMember) *projectv1.CrewMember {
	out := &projectv1.CrewMember{
		Id:           c.ID.String(),
		ProductionId: c.ProductionID.String(),
		Name:         c.Name,
		Role:         c.Role,
		Department:   c.Department,
		DayRate:      c.DayRate.StringFixed(2), // Rule #1: money as string with 2dp
		Currency:     c.Currency,
	}
	if c.UserID != nil {
		out.UserId = c.UserID.String()
	}
	return out
}

// grpcErr maps domain sentinel errors to the appropriate gRPC status codes.
// Unknown errors are returned as opaque Internal so internal details are never
// leaked to callers (Rule #11 — validate at the boundary).
func grpcErr(err error) error {
	switch {
	case errors.Is(err, ErrProductionNotFound):
		return status.Error(codes.NotFound, "production not found")
	case errors.Is(err, ErrPhaseNotFound):
		return status.Error(codes.NotFound, "phase not found")
	case errors.Is(err, ErrCrewNotFound):
		return status.Error(codes.NotFound, "crew member not found")
	case errors.Is(err, ErrDuplicateSlug):
		return status.Error(codes.AlreadyExists, "production with this slug already exists")
	case errors.Is(err, ErrInvalidPhaseType):
		return status.Error(codes.InvalidArgument, "invalid phase type for this vertical")
	case errors.Is(err, ErrInvalidTransition):
		return status.Error(codes.FailedPrecondition, "phase transition not allowed")
	case errors.Is(err, ErrProductionArchived):
		return status.Error(codes.FailedPrecondition, "production is archived")
	case errors.Is(err, vertical.ErrNotFound):
		return status.Error(codes.FailedPrecondition, "vertical config not found for tenant")
	}
	return status.Error(codes.Internal, "internal error")
}
