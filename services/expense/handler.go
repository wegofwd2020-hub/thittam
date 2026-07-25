package expense

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	expensev1 "github.com/wegofwd2020/thittam/gen/expense/v1"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/tenant"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Handler implements the gRPC ExpenseService.
type Handler struct {
	expensev1.UnimplementedExpenseServiceServer
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
var _ expensev1.ExpenseServiceServer = (*Handler)(nil)

// --- Purchase Orders ---

func (h *Handler) CreatePurchaseOrder(ctx context.Context, req *expensev1.CreatePurchaseOrderRequest) (*expensev1.PurchaseOrder, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "expense:submit"); err != nil {
		return nil, err
	}

	productionID, err := uuid.Parse(req.GetProductionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid production ID")
	}

	amount, err := decimal.NewFromString(req.GetAmount())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid amount: must be a decimal string")
	}

	currency := req.GetCurrency()
	if currency == "" {
		currency = "INR"
	}

	po := &PurchaseOrder{
		ProductionID: productionID,
		TenantID:     tenantID,
		PONumber:     req.GetPoNumber(),
		VendorName:   req.GetVendorName(),
		VendorGSTIN:  req.GetVendorGstin(),
		Description:  req.GetDescription(),
		Amount:       amount,
		Currency:     currency,
		RaisedBy:     uuid.Nil, // populated by auth interceptor once wired
	}

	if s := req.GetBudgetLineId(); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid budget_line_id")
		}
		po.BudgetLineID = &id
	}

	if err := h.svc.CreatePurchaseOrder(ctx, po); err != nil {
		return nil, grpcErr(err)
	}

	out, err := h.svc.GetPurchaseOrder(ctx, tenantID, po.ID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return purchaseOrderToProto(out), nil
}

func (h *Handler) GetPurchaseOrder(ctx context.Context, req *expensev1.GetPurchaseOrderRequest) (*expensev1.PurchaseOrder, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "expense:read"); err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid purchase order ID")
	}

	po, err := h.svc.GetPurchaseOrder(ctx, tenantID, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return purchaseOrderToProto(po), nil
}

func (h *Handler) ListPurchaseOrders(ctx context.Context, req *expensev1.ListPurchaseOrdersRequest) (*expensev1.ListPurchaseOrdersResponse, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "expense:read"); err != nil {
		return nil, err
	}

	var productionID uuid.UUID
	if s := req.GetProductionId(); s != "" {
		var err error
		productionID, err = uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid production ID")
		}
	}

	pos, err := h.svc.ListPurchaseOrders(ctx, tenantID, productionID, req.GetStatus(), int(req.GetLimit()), 0)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*expensev1.PurchaseOrder, len(pos))
	for i := range pos {
		out[i] = purchaseOrderToProto(&pos[i])
	}
	return &expensev1.ListPurchaseOrdersResponse{PurchaseOrders: out}, nil
}

func (h *Handler) ApprovePurchaseOrder(ctx context.Context, req *expensev1.ApprovePurchaseOrderRequest) (*expensev1.PurchaseOrder, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "expense:approve"); err != nil {
		return nil, err
	}

	poID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}

	// approverID is populated by the auth interceptor once wired.
	if err := h.svc.ApprovePurchaseOrder(ctx, tenantID, poID, uuid.Nil); err != nil {
		return nil, grpcErr(err)
	}

	po, err := h.svc.GetPurchaseOrder(ctx, tenantID, poID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return purchaseOrderToProto(po), nil
}

// --- Expenses ---

func (h *Handler) SubmitExpense(ctx context.Context, req *expensev1.SubmitExpenseRequest) (*expensev1.Expense, error) {
	if err := interceptor.RequirePermission(ctx, h.perm, "expense:submit"); err != nil {
		return nil, err
	}

	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	productionID, err := uuid.Parse(req.GetProductionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid production ID")
	}

	amount, err := decimal.NewFromString(req.GetAmount())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid amount: must be a decimal string")
	}

	// tax_amount is not exposed in the current proto; defaults to zero.
	taxAmount := decimal.Zero

	currency := req.GetCurrency()
	if currency == "" {
		currency = "INR"
	}

	e := &Expense{
		ProductionID: productionID,
		TenantID:     tenantID,
		CategoryID:   req.GetCategoryId(),
		Description:  req.GetDescription(),
		Amount:       amount,
		Currency:     currency,
		TaxAmount:    taxAmount,
		SubmittedBy:  uuid.Nil, // populated by auth interceptor once wired
	}

	if s := req.GetBudgetLineId(); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid budget_line_id")
		}
		e.BudgetLineID = &id
	}

	if s := req.GetPurchaseOrderId(); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid purchase_order_id")
		}
		e.PurchaseOrderID = &id
	}

	if err := h.svc.SubmitExpense(ctx, e); err != nil {
		return nil, grpcErr(err)
	}

	out, err := h.svc.GetExpense(ctx, tenantID, e.ID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return expenseToProto(out), nil
}

func (h *Handler) GetExpense(ctx context.Context, req *expensev1.GetExpenseRequest) (*expensev1.Expense, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "expense:read"); err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid expense ID")
	}

	e, err := h.svc.GetExpense(ctx, tenantID, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return expenseToProto(e), nil
}

func (h *Handler) ListExpenses(ctx context.Context, req *expensev1.ListExpensesRequest) (*expensev1.ListExpensesResponse, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "expense:read"); err != nil {
		return nil, err
	}

	var productionID uuid.UUID
	if s := req.GetProductionId(); s != "" {
		var err error
		productionID, err = uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid production ID")
		}
	}

	expenses, err := h.svc.ListExpenses(ctx, tenantID, productionID, req.GetStatus(), int(req.GetLimit()), 0)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*expensev1.Expense, len(expenses))
	for i := range expenses {
		out[i] = expenseToProto(&expenses[i])
	}
	return &expensev1.ListExpensesResponse{Expenses: out}, nil
}

func (h *Handler) ApproveExpense(ctx context.Context, req *expensev1.ApproveExpenseRequest) (*expensev1.Expense, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "expense:approve"); err != nil {
		return nil, err
	}

	expenseID, err := uuid.Parse(req.GetExpenseId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid expense_id")
	}

	// approverID and approverRole are populated by the auth interceptor once wired.
	if err := h.svc.ApproveExpense(ctx, tenantID, expenseID, uuid.Nil, ""); err != nil {
		return nil, grpcErr(err)
	}

	e, err := h.svc.GetExpense(ctx, tenantID, expenseID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return expenseToProto(e), nil
}

func (h *Handler) RejectExpense(ctx context.Context, req *expensev1.RejectExpenseRequest) (*expensev1.Expense, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "expense:approve"); err != nil {
		return nil, err
	}

	expenseID, err := uuid.Parse(req.GetExpenseId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid expense_id")
	}

	if err := h.svc.RejectExpense(ctx, tenantID, expenseID, req.GetReason()); err != nil {
		return nil, grpcErr(err)
	}

	e, err := h.svc.GetExpense(ctx, tenantID, expenseID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return expenseToProto(e), nil
}

// --- Petty Cash ---

func (h *Handler) CreatePettyCashAdvance(ctx context.Context, req *expensev1.CreatePettyCashAdvanceRequest) (*expensev1.PettyCashAdvance, error) {
	if err := interceptor.RequirePermission(ctx, h.perm, "expense:submit"); err != nil {
		return nil, err
	}

	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	productionID, err := uuid.Parse(req.GetProductionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid production ID")
	}

	issuedTo, err := uuid.Parse(req.GetIssuedTo())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid issued_to: must be a valid UUID")
	}

	amount, err := decimal.NewFromString(req.GetAmount())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid amount: must be a decimal string")
	}

	pc := &PettyCashAdvance{
		ProductionID: productionID,
		TenantID:     tenantID,
		IssuedTo:     issuedTo,
		Amount:       amount,
		Purpose:      req.GetPurpose(),
	}

	if err := h.svc.CreatePettyCashAdvance(ctx, pc); err != nil {
		return nil, grpcErr(err)
	}

	out, err := h.svc.GetPettyCashAdvance(ctx, tenantID, pc.ID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return pettyCashToProto(out), nil
}

func (h *Handler) GetPettyCashAdvance(ctx context.Context, req *expensev1.GetPettyCashAdvanceRequest) (*expensev1.PettyCashAdvance, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "expense:read"); err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid petty cash advance ID")
	}

	pc, err := h.svc.GetPettyCashAdvance(ctx, tenantID, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return pettyCashToProto(pc), nil
}

func (h *Handler) ListPettyCashAdvances(ctx context.Context, req *expensev1.ListPettyCashAdvancesRequest) (*expensev1.ListPettyCashAdvancesResponse, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "expense:read"); err != nil {
		return nil, err
	}

	var productionID uuid.UUID
	if s := req.GetProductionId(); s != "" {
		var err error
		productionID, err = uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid production ID")
		}
	}

	advances, err := h.svc.ListPettyCashAdvances(ctx, tenantID, productionID, req.GetStatus(), int(req.GetLimit()), 0)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*expensev1.PettyCashAdvance, len(advances))
	for i := range advances {
		out[i] = pettyCashToProto(&advances[i])
	}
	return &expensev1.ListPettyCashAdvancesResponse{Advances: out}, nil
}

func (h *Handler) SettlePettyCash(ctx context.Context, req *expensev1.SettlePettyCashRequest) (*expensev1.PettyCashAdvance, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "expense:submit"); err != nil {
		return nil, err
	}

	advanceID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}

	unspent, err := decimal.NewFromString(req.GetUnspentAmount())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid unspent_amount: must be a decimal string")
	}

	if err := h.svc.SettlePettyCash(ctx, tenantID, advanceID, unspent); err != nil {
		return nil, grpcErr(err)
	}

	pc, err := h.svc.GetPettyCashAdvance(ctx, tenantID, advanceID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return pettyCashToProto(pc), nil
}

// --- Vertical metadata ---

func (h *Handler) GetExpenseCategories(ctx context.Context, _ *expensev1.GetExpenseCategoriesRequest) (*expensev1.GetExpenseCategoriesResponse, error) {
	cats := h.svc.GetExpenseCategories(ctx)
	out := make([]*expensev1.ExpenseCategoryInfo, len(cats))
	for i, c := range cats {
		out[i] = &expensev1.ExpenseCategoryInfo{
			Id:                 c.ID,
			Label:              c.Label,
			TaxTreatment:       c.TaxTreatment,
			DefaultAccountCode: c.DefaultAccountCode,
			RequiresPo:         c.RequiresPO,
		}
	}
	return &expensev1.GetExpenseCategoriesResponse{Categories: out}, nil
}

func (h *Handler) GetApprovalLimits(ctx context.Context, _ *expensev1.GetApprovalLimitsRequest) (*expensev1.GetApprovalLimitsResponse, error) {
	wf := h.svc.GetApprovalLimits(ctx)
	limits := make([]*expensev1.ApprovalLimitInfo, len(wf.Limits))
	for i, l := range wf.Limits {
		limits[i] = &expensev1.ApprovalLimitInfo{
			Role:      l.Role,
			MaxAmount: l.MaxAmount.StringFixed(2),
		}
	}
	return &expensev1.GetApprovalLimitsResponse{
		Limits:            limits,
		DualApprovalAbove: wf.DualApprovalAbove.StringFixed(2),
	}, nil
}

// --- proto conversion helpers ---

func purchaseOrderToProto(po *PurchaseOrder) *expensev1.PurchaseOrder {
	out := &expensev1.PurchaseOrder{
		Id:           po.ID.String(),
		ProductionId: po.ProductionID.String(),
		TenantId:     po.TenantID.String(),
		PoNumber:     po.PONumber,
		VendorName:   po.VendorName,
		VendorGstin:  po.VendorGSTIN,
		Description:  po.Description,
		Amount:       po.Amount.StringFixed(2),
		Currency:     po.Currency,
		Status:       po.Status,
		RaisedBy:     po.RaisedBy.String(),
		RaisedAt:     timestamppb.New(po.RaisedAt),
		CreatedAt:    timestamppb.New(po.CreatedAt),
	}
	if po.BudgetLineID != nil {
		out.BudgetLineId = po.BudgetLineID.String()
	}
	if po.ApprovedBy != nil {
		out.ApprovedBy = po.ApprovedBy.String()
	}
	if po.ApprovedAt != nil {
		out.ApprovedAt = timestamppb.New(*po.ApprovedAt)
	}
	return out
}

func expenseToProto(e *Expense) *expensev1.Expense {
	out := &expensev1.Expense{
		Id:           e.ID.String(),
		ProductionId: e.ProductionID.String(),
		TenantId:     e.TenantID.String(),
		CategoryId:   e.CategoryID,
		Description:  e.Description,
		Amount:       e.Amount.StringFixed(2),
		Currency:     e.Currency,
		TaxAmount:    e.TaxAmount.StringFixed(2),
		Status:       e.Status,
		SubmittedBy:  e.SubmittedBy.String(),
		CreatedAt:    timestamppb.New(e.CreatedAt),
	}
	if e.BudgetLineID != nil {
		out.BudgetLineId = e.BudgetLineID.String()
	}
	if e.PurchaseOrderID != nil {
		out.PurchaseOrderId = e.PurchaseOrderID.String()
	}
	if e.ApprovedBy != nil {
		out.ApprovedBy = e.ApprovedBy.String()
	}
	if e.SubmittedAt != nil {
		out.SubmittedAt = timestamppb.New(*e.SubmittedAt)
	}
	if e.ApprovedAt != nil {
		out.ApprovedAt = timestamppb.New(*e.ApprovedAt)
	}
	return out
}

func pettyCashToProto(pc *PettyCashAdvance) *expensev1.PettyCashAdvance {
	out := &expensev1.PettyCashAdvance{
		Id:            pc.ID.String(),
		ProductionId:  pc.ProductionID.String(),
		TenantId:      pc.TenantID.String(),
		IssuedTo:      pc.IssuedTo.String(),
		Amount:        pc.Amount.StringFixed(2),
		Purpose:       pc.Purpose,
		Status:        pc.Status,
		IssuedAt:      timestamppb.New(pc.IssuedAt),
		UnspentAmount: pc.UnspentAmount.StringFixed(2),
	}
	if pc.SettledAt != nil {
		out.SettledAt = timestamppb.New(*pc.SettledAt)
	}
	return out
}

// grpcErr maps domain sentinel errors to gRPC status codes.
func grpcErr(err error) error {
	switch {
	case errors.Is(err, ErrExpenseNotFound):
		return status.Error(codes.NotFound, "expense not found")
	case errors.Is(err, ErrPONotFound):
		return status.Error(codes.NotFound, "purchase order not found")
	case errors.Is(err, ErrAlreadyApproved), errors.Is(err, ErrAlreadyRejected):
		return status.Error(codes.FailedPrecondition, "expense is already approved")
	case errors.Is(err, ErrApprovalLimitExceeded):
		return status.Error(codes.FailedPrecondition, "amount exceeds approval limit for role")
	case errors.Is(err, ErrDualApprovalRequired):
		return status.Error(codes.FailedPrecondition, "amount requires dual approval")
	case errors.Is(err, ErrPORequired):
		return status.Error(codes.FailedPrecondition, "purchase order required for this category")
	case errors.Is(err, ErrInvalidCategory):
		return status.Error(codes.InvalidArgument, "invalid expense category for this vertical")
	case errors.Is(err, ErrInsufficientBudget):
		return status.Error(codes.FailedPrecondition, "insufficient budget remaining")
	case errors.Is(err, vertical.ErrNotFound):
		return status.Error(codes.FailedPrecondition, "vertical config not found for tenant")
	}
	return status.Error(codes.Internal, "internal error")
}
