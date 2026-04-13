package budget

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	budgetv1 "github.com/wegofwd2020/thittam/gen/budget/v1"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/tenant"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Handler implements the gRPC BudgetService.
type Handler struct {
	budgetv1.UnimplementedBudgetServiceServer
	svc  *Service
	perm interceptor.PermissionChecker // optional; nil skips RequirePermission (backwards compatible)
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
var _ budgetv1.BudgetServiceServer = (*Handler)(nil)

// --- Budgets ---

func (h *Handler) CreateBudget(ctx context.Context, req *budgetv1.CreateBudgetRequest) (*budgetv1.Budget, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if h.perm != nil {
		if err := interceptor.RequirePermission(ctx, h.perm, "budget:write"); err != nil {
			return nil, err
		}
	}

	productionID, err := uuid.Parse(req.GetProductionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid production ID")
	}

	currency := req.GetCurrency()
	if currency == "" {
		currency = "INR"
	}

	b := &Budget{
		ProductionID: productionID,
		TenantID:     tenantID,
		Label:        req.GetLabel(),
		Currency:     currency,
		CreatedBy:    uuid.Nil, // populated by auth interceptor once wired
	}

	if err := h.svc.CreateBudget(ctx, b); err != nil {
		return nil, grpcErr(err)
	}

	out, err := h.svc.GetBudget(ctx, tenantID, b.ID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return budgetToProto(out), nil
}

func (h *Handler) GetBudget(ctx context.Context, req *budgetv1.GetBudgetRequest) (*budgetv1.Budget, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid budget ID")
	}

	b, err := h.svc.GetBudget(ctx, tenantID, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return budgetToProto(b), nil
}

func (h *Handler) ListBudgets(ctx context.Context, req *budgetv1.ListBudgetsRequest) (*budgetv1.ListBudgetsResponse, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	var productionID uuid.UUID
	if s := req.GetProductionId(); s != "" {
		var err error
		productionID, err = uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid production ID")
		}
	}

	limit := int(req.GetLimit())
	// Always use offset=0 for now; cursor (after) is a follow-up.
	budgets, err := h.svc.ListBudgets(ctx, tenantID, productionID, req.GetStatus(), limit, 0)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*budgetv1.Budget, len(budgets))
	for i := range budgets {
		out[i] = budgetToProto(&budgets[i])
	}
	return &budgetv1.ListBudgetsResponse{Budgets: out}, nil
}

func (h *Handler) SubmitBudget(ctx context.Context, req *budgetv1.SubmitBudgetRequest) (*budgetv1.Budget, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if h.perm != nil {
		if err := interceptor.RequirePermission(ctx, h.perm, "budget:write"); err != nil {
			return nil, err
		}
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid budget ID")
	}

	if err := h.svc.SubmitBudget(ctx, tenantID, id, uuid.Nil); err != nil {
		return nil, grpcErr(err)
	}

	b, err := h.svc.GetBudget(ctx, tenantID, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return budgetToProto(b), nil
}

func (h *Handler) ApproveBudget(ctx context.Context, req *budgetv1.ApproveBudgetRequest) (*budgetv1.Budget, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if h.perm != nil {
		if err := interceptor.RequirePermission(ctx, h.perm, "budget:approve"); err != nil {
			return nil, err
		}
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid budget ID")
	}

	if err := h.svc.ApproveBudget(ctx, tenantID, id, uuid.Nil); err != nil {
		return nil, grpcErr(err)
	}

	b, err := h.svc.GetBudget(ctx, tenantID, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return budgetToProto(b), nil
}

func (h *Handler) CreateBudgetFromTemplate(ctx context.Context, req *budgetv1.CreateBudgetFromTemplateRequest) (*budgetv1.Budget, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if h.perm != nil {
		if err := interceptor.RequirePermission(ctx, h.perm, "budget:write"); err != nil {
			return nil, err
		}
	}

	productionID, err := uuid.Parse(req.GetProductionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid production ID")
	}

	currency := req.GetCurrency()
	if currency == "" {
		currency = "INR"
	}

	b := &Budget{
		ProductionID: productionID,
		TenantID:     tenantID,
		Label:        req.GetLabel(),
		Currency:     currency,
		CreatedBy:    uuid.Nil,
	}

	if err := h.svc.CreateBudgetFromTemplate(ctx, b, req.GetTemplateName()); err != nil {
		return nil, grpcErr(err)
	}

	out, err := h.svc.GetBudget(ctx, tenantID, b.ID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return budgetToProto(out), nil
}

// --- Line Items ---

func (h *Handler) CreateLineItem(ctx context.Context, req *budgetv1.CreateLineItemRequest) (*budgetv1.BudgetLineItem, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if h.perm != nil {
		if err := interceptor.RequirePermission(ctx, h.perm, "budget:write"); err != nil {
			return nil, err
		}
	}

	budgetID, err := uuid.Parse(req.GetBudgetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid budget ID")
	}

	budgetedAmount, err := decimal.NewFromString(req.GetBudgetedAmount())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid budgeted_amount: must be a decimal string")
	}

	li := &BudgetLineItem{
		BudgetID:       budgetID,
		TenantID:       tenantID,
		CategoryID:     req.GetCategoryId(),
		Description:    req.GetDescription(),
		AccountCode:    req.GetAccountCode(),
		BudgetedAmount: budgetedAmount,
	}

	if err := h.svc.CreateLineItem(ctx, li); err != nil {
		return nil, grpcErr(err)
	}

	created, err := h.svc.GetLineItem(ctx, li.ID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return lineItemToProto(created), nil
}

func (h *Handler) GetLineItem(ctx context.Context, req *budgetv1.GetLineItemRequest) (*budgetv1.BudgetLineItem, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid line item ID")
	}

	li, err := h.svc.GetLineItem(ctx, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return lineItemToProto(li), nil
}

func (h *Handler) ListLineItems(ctx context.Context, req *budgetv1.ListLineItemsRequest) (*budgetv1.ListLineItemsResponse, error) {
	budgetID, err := uuid.Parse(req.GetBudgetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid budget ID")
	}

	// Proto does not yet carry limit/offset — apply a server-side cap to prevent
	// unbounded queries. Callers wanting a different page size should use the
	// batch-export endpoint or wait for the proto to be updated.
	const defaultLimit = 100
	items, err := h.svc.ListLineItems(ctx, budgetID, defaultLimit, 0)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*budgetv1.BudgetLineItem, len(items))
	for i := range items {
		out[i] = lineItemToProto(&items[i])
	}
	return &budgetv1.ListLineItemsResponse{LineItems: out}, nil
}

func (h *Handler) UpdateLineItemActuals(ctx context.Context, req *budgetv1.UpdateLineItemActualsRequest) (*budgetv1.BudgetLineItem, error) {
	if h.perm != nil {
		if err := interceptor.RequirePermission(ctx, h.perm, "budget:write"); err != nil {
			return nil, err
		}
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid line item ID")
	}

	actual, err := decimal.NewFromString(req.GetActualAmount())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid actual_amount")
	}

	committed, err := decimal.NewFromString(req.GetCommittedAmount())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid committed_amount")
	}

	if err := h.svc.UpdateLineItemActuals(ctx, id, actual, committed); err != nil {
		return nil, grpcErr(err)
	}

	li, err := h.svc.GetLineItem(ctx, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return lineItemToProto(li), nil
}

func (h *Handler) CheckLineAvailability(ctx context.Context, req *budgetv1.CheckLineAvailabilityRequest) (*budgetv1.CheckLineAvailabilityResponse, error) {
	id, err := uuid.Parse(req.GetLineItemId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid line_item_id")
	}

	available, err := h.svc.CheckLineAvailability(ctx, id)
	if err != nil {
		return nil, grpcErr(err)
	}

	return &budgetv1.CheckLineAvailabilityResponse{
		AvailableAmount: available.StringFixed(2),
	}, nil
}

// --- Vertical metadata ---

func (h *Handler) GetBudgetCategories(ctx context.Context, _ *budgetv1.GetBudgetCategoriesRequest) (*budgetv1.GetBudgetCategoriesResponse, error) {
	cats := h.svc.GetBudgetCategories(ctx)
	out := make([]*budgetv1.BudgetCategoryInfo, len(cats))
	for i, c := range cats {
		out[i] = &budgetv1.BudgetCategoryInfo{
			Id:                 c.ID,
			Label:              c.Label,
			Description:        c.Description,
			DefaultAccountCode: c.DefaultAccountCode,
		}
	}
	return &budgetv1.GetBudgetCategoriesResponse{Categories: out}, nil
}

func (h *Handler) GetBudgetTemplates(ctx context.Context, _ *budgetv1.GetBudgetTemplatesRequest) (*budgetv1.GetBudgetTemplatesResponse, error) {
	tmpls := h.svc.GetBudgetTemplates(ctx)
	out := make([]*budgetv1.BudgetTemplateInfo, len(tmpls))
	for i, t := range tmpls {
		lis := make([]*budgetv1.TemplateLineItemInfo, len(t.LineItems))
		for j, tli := range t.LineItems {
			lis[j] = &budgetv1.TemplateLineItemInfo{
				CategoryId:    tli.CategoryID,
				Description:   tli.Description,
				AccountCode:   tli.AccountCode,
				DefaultAmount: tli.DefaultAmount.StringFixed(2),
				IsRequired:    tli.IsRequired,
			}
		}
		out[i] = &budgetv1.BudgetTemplateInfo{
			Name:        t.Name,
			Description: t.Description,
			LineItems:   lis,
		}
	}
	return &budgetv1.GetBudgetTemplatesResponse{Templates: out}, nil
}

// --- proto conversion helpers ---

func budgetToProto(b *Budget) *budgetv1.Budget {
	out := &budgetv1.Budget{
		Id:           b.ID.String(),
		TenantId:     b.TenantID.String(),
		ProductionId: b.ProductionID.String(),
		Label:        b.Label,
		Status:       b.Status,
		Currency:     b.Currency,
		TotalAmount:  b.TotalAmount.StringFixed(2), // Rule #1: money as string
		CreatedBy:    b.CreatedBy.String(),
		CreatedAt:    timestamppb.New(b.CreatedAt),
		UpdatedAt:    timestamppb.New(b.UpdatedAt),
	}
	if b.SubmittedBy != nil {
		out.SubmittedBy = b.SubmittedBy.String()
	}
	if b.ApprovedBy != nil {
		out.ApprovedBy = b.ApprovedBy.String()
	}
	if b.SubmittedAt != nil {
		out.SubmittedAt = timestamppb.New(*b.SubmittedAt)
	}
	if b.ApprovedAt != nil {
		out.ApprovedAt = timestamppb.New(*b.ApprovedAt)
	}
	return out
}

func lineItemToProto(li *BudgetLineItem) *budgetv1.BudgetLineItem {
	return &budgetv1.BudgetLineItem{
		Id:              li.ID.String(),
		BudgetId:        li.BudgetID.String(),
		TenantId:        li.TenantID.String(),
		CategoryId:      li.CategoryID,
		Description:     li.Description,
		AccountCode:     li.AccountCode,
		BudgetedAmount:  li.BudgetedAmount.StringFixed(2),
		ActualAmount:    li.ActualAmount.StringFixed(2),
		CommittedAmount: li.CommittedAmount.StringFixed(2),
		IsLocked:        li.IsLocked,
		CreatedAt:       timestamppb.New(li.CreatedAt),
		UpdatedAt:       timestamppb.New(li.UpdatedAt),
	}
}

// grpcErr maps domain sentinel errors to gRPC status codes.
func grpcErr(err error) error {
	switch {
	case errors.Is(err, ErrBudgetNotFound):
		return status.Error(codes.NotFound, "budget not found")
	case errors.Is(err, ErrLineItemNotFound):
		return status.Error(codes.NotFound, "line item not found")
	case errors.Is(err, ErrBudgetLocked):
		return status.Error(codes.FailedPrecondition, "budget is locked")
	case errors.Is(err, ErrBudgetNotApproved):
		return status.Error(codes.FailedPrecondition, "budget is not in submitted status")
	case errors.Is(err, ErrAlreadyApproved):
		return status.Error(codes.FailedPrecondition, "budget is already approved or locked")
	case errors.Is(err, ErrInvalidCategory):
		return status.Error(codes.InvalidArgument, "invalid budget category for this vertical")
	case errors.Is(err, ErrInsufficientFunds):
		return status.Error(codes.FailedPrecondition, "insufficient funds on line item")
	case errors.Is(err, vertical.ErrNotFound):
		return status.Error(codes.FailedPrecondition, "vertical config not found for tenant")
	}
	return status.Error(codes.Internal, "internal error")
}
