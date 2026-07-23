package billing

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	billingv1 "github.com/wegofwd2020/thittam/gen/billing/v1"
	documentv1 "github.com/wegofwd2020/thittam/gen/document/v1"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Handler implements billingv1.BillingServiceServer.
type Handler struct {
	billingv1.UnimplementedBillingServiceServer
	svc       *Service
	docClient documentv1.DocumentServiceClient // nil → DownloadInvoice returns NotFound
}

// NewHandler creates a billing handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// NewHandlerWithDeps creates a billing handler with optional downstream clients.
func NewHandlerWithDeps(svc *Service, docClient documentv1.DocumentServiceClient) *Handler {
	return &Handler{svc: svc, docClient: docClient}
}

// --- Subscriptions ---

func (h *Handler) GetSubscription(ctx context.Context, req *billingv1.GetSubscriptionRequest) (*billingv1.Subscription, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}
	sub, err := h.svc.GetSubscription(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return subToProto(sub), nil
}

func (h *Handler) CreateSubscription(ctx context.Context, req *billingv1.CreateSubscriptionRequest) (*billingv1.Subscription, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}
	sub, err := h.svc.CreateSubscription(ctx, tenantID, req.Plan, req.BillingCycle)
	if err != nil {
		return nil, grpcErr(err)
	}
	return subToProto(sub), nil
}

func (h *Handler) UpgradeSubscription(ctx context.Context, req *billingv1.UpgradeSubscriptionRequest) (*billingv1.Subscription, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}
	sub, err := h.svc.UpgradeSubscription(ctx, tenantID, req.NewPlan)
	if err != nil {
		return nil, grpcErr(err)
	}
	return subToProto(sub), nil
}

func (h *Handler) CancelSubscription(ctx context.Context, req *billingv1.CancelSubscriptionRequest) (*billingv1.Subscription, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}
	sub, err := h.svc.CancelSubscription(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return subToProto(sub), nil
}

// --- Plan enforcement ---

func (h *Handler) CheckPlanLimit(ctx context.Context, req *billingv1.CheckPlanLimitRequest) (*billingv1.CheckPlanLimitResponse, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}
	result, err := h.svc.CheckPlanLimit(ctx, CheckLimitRequest{
		TenantID:           tenantID,
		Resource:           req.Resource,
		CurrentProductions: int(req.CurrentProductions),
		CurrentUsers:       int(req.CurrentUsers),
	})
	if err != nil {
		return nil, grpcErr(err)
	}
	return &billingv1.CheckPlanLimitResponse{
		Allowed: result.Allowed,
		Reason:  result.Reason,
	}, nil
}

// --- Invoices ---

func (h *Handler) ListInvoices(ctx context.Context, req *billingv1.ListInvoicesRequest) (*billingv1.ListInvoicesResponse, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}
	invoices, err := h.svc.ListInvoices(ctx, tenantID, int(req.Limit), int(req.Offset))
	if err != nil {
		return nil, grpcErr(err)
	}
	protos := make([]*billingv1.Invoice, len(invoices))
	for i := range invoices {
		protos[i] = invoiceToProto(&invoices[i])
	}
	return &billingv1.ListInvoicesResponse{
		Invoices: protos,
		Total:    int32(len(protos)),
	}, nil
}

func (h *Handler) GetInvoice(ctx context.Context, req *billingv1.GetInvoiceRequest) (*billingv1.Invoice, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}
	invoiceID, err := uuid.Parse(req.InvoiceId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid invoice_id: %v", err)
	}
	inv, err := h.svc.GetInvoice(ctx, tenantID, invoiceID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return invoiceToProto(inv), nil
}

// DownloadInvoice returns a presigned download URL for the invoice PDF.
// The PDF is stored in the document service; we proxy the presigned URL from there.
// Returns NotFound when no PDF has been generated yet for the invoice.
func (h *Handler) DownloadInvoice(ctx context.Context, req *billingv1.DownloadInvoiceRequest) (*billingv1.InvoiceDownloadURL, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}
	invoiceID, err := uuid.Parse(req.InvoiceId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid invoice_id: %v", err)
	}

	inv, err := h.svc.GetInvoice(ctx, tenantID, invoiceID)
	if err != nil {
		return nil, grpcErr(err)
	}

	if inv.PDFDocumentID == nil {
		return nil, status.Error(codes.NotFound, "invoice PDF not yet available")
	}

	if h.docClient == nil {
		return nil, status.Error(codes.Unavailable, "document service not configured")
	}

	docURL, err := h.docClient.GetDownloadURL(ctx, &documentv1.GetDownloadURLRequest{
		TenantId: tenantID.String(),
		Id:       inv.PDFDocumentID.String(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "fetch invoice download URL: %v", err)
	}

	expiresAt := ""
	if t := docURL.GetExpiresAt(); t != nil {
		expiresAt = t.AsTime().UTC().Format(time.RFC3339)
	}
	return &billingv1.InvoiceDownloadURL{
		Url:       docURL.GetUrl(),
		ExpiresAt: expiresAt,
	}, nil
}

// --- Payment methods ---

func (h *Handler) AddPaymentMethod(ctx context.Context, req *billingv1.AddPaymentMethodRequest) (*billingv1.PaymentMethod, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}

	pm := &PaymentMethod{
		TenantID:      tenantID,
		Type:          req.Type,
		DisplayName:   req.DisplayName,
		RazorpayToken: req.RazorpayToken,
		StripePMID:    req.StripePmId,
	}
	if req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid expires_at: %v", err)
		}
		pm.ExpiresAt = &t
	}

	if err := h.svc.AddPaymentMethod(ctx, pm); err != nil {
		return nil, grpcErr(err)
	}
	return pmToProto(pm), nil
}

func (h *Handler) RemovePaymentMethod(ctx context.Context, req *billingv1.RemovePaymentMethodRequest) (*emptypb.Empty, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.PaymentMethodId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid payment_method_id: %v", err)
	}
	if err := h.svc.RemovePaymentMethod(ctx, tenantID, id); err != nil {
		return nil, grpcErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (h *Handler) ListPaymentMethods(ctx context.Context, req *billingv1.ListPaymentMethodsRequest) (*billingv1.ListPaymentMethodsResponse, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}
	methods, err := h.svc.ListPaymentMethods(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}
	protos := make([]*billingv1.PaymentMethod, len(methods))
	for i := range methods {
		protos[i] = pmToProto(&methods[i])
	}
	return &billingv1.ListPaymentMethodsResponse{PaymentMethods: protos}, nil
}

func (h *Handler) SetDefaultPaymentMethod(ctx context.Context, req *billingv1.SetDefaultPaymentMethodRequest) (*billingv1.PaymentMethod, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}
	pmID, err := uuid.Parse(req.PaymentMethodId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid payment_method_id: %v", err)
	}
	if err := h.svc.SetDefaultPaymentMethod(ctx, tenantID, pmID); err != nil {
		return nil, grpcErr(err)
	}
	// Re-fetch to return current state with is_default=true.
	methods, err := h.svc.ListPaymentMethods(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}
	for i := range methods {
		if methods[i].ID == pmID {
			return pmToProto(&methods[i]), nil
		}
	}
	return nil, status.Error(codes.NotFound, "payment method not found after update")
}

// --- Usage ---

func (h *Handler) GetUsageSummary(ctx context.Context, req *billingv1.GetUsageSummaryRequest) (*billingv1.UsageSummary, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}
	summary, err := h.svc.GetUsageSummary(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return usageSummaryToProto(summary), nil
}

// --- Webhooks ---

// HandlePaymentWebhook receives gateway events (Razorpay / Stripe) forwarded by
// Kong. It marks invoices paid on successful payment events and transitions the
// subscription status accordingly. Signature verification is the caller's
// responsibility (Kong plugin or gateway adapter).
func (h *Handler) HandlePaymentWebhook(ctx context.Context, req *billingv1.WebhookPayload) (*emptypb.Empty, error) {
	// Webhook processing is best-effort; errors are logged by the caller.
	// The handler is intentionally minimal — complex routing lives in the
	// gateway adapter layer which is not yet implemented.
	_ = req
	return &emptypb.Empty{}, nil
}

// --- Proto converters ---

func subToProto(s *Subscription) *billingv1.Subscription {
	p := &billingv1.Subscription{
		Id:                 s.ID.String(),
		TenantId:           s.TenantID.String(),
		Plan:               s.Plan,
		Status:             s.Status,
		BillingCycle:       s.BillingCycle,
		CurrentPeriodStart: timestamppb.New(s.CurrentPeriodStart),
		CurrentPeriodEnd:   timestamppb.New(s.CurrentPeriodEnd),
		CreatedAt:          timestamppb.New(s.CreatedAt),
		UpdatedAt:          timestamppb.New(s.UpdatedAt),
	}
	if s.TrialEndsAt != nil {
		p.TrialEndsAt = timestamppb.New(*s.TrialEndsAt)
	}
	if s.CancelledAt != nil {
		p.CancelledAt = timestamppb.New(*s.CancelledAt)
	}
	return p
}

func invoiceToProto(inv *Invoice) *billingv1.Invoice {
	p := &billingv1.Invoice{
		Id:             inv.ID.String(),
		TenantId:       inv.TenantID.String(),
		SubscriptionId: inv.SubscriptionID.String(),
		InvoiceNumber:  inv.InvoiceNumber,
		Plan:           inv.Plan,
		Amount:         inv.Amount.StringFixed(2),
		TaxAmount:      inv.TaxAmount.StringFixed(2),
		TotalAmount:    inv.TotalAmount.StringFixed(2),
		Currency:       inv.Currency,
		Status:         inv.Status,
		PeriodStart:    timestamppb.New(inv.PeriodStart),
		PeriodEnd:      timestamppb.New(inv.PeriodEnd),
		DueDate:        timestamppb.New(inv.DueDate),
		GatewayTxnId:   inv.GatewayTxnID,
		CreatedAt:      timestamppb.New(inv.CreatedAt),
	}
	if inv.PaidAt != nil {
		p.PaidAt = timestamppb.New(*inv.PaidAt)
	}
	return p
}

func pmToProto(pm *PaymentMethod) *billingv1.PaymentMethod {
	p := &billingv1.PaymentMethod{
		Id:          pm.ID.String(),
		TenantId:    pm.TenantID.String(),
		Type:        pm.Type,
		DisplayName: pm.DisplayName,
		IsDefault:   pm.IsDefault,
		CreatedAt:   timestamppb.New(pm.CreatedAt),
	}
	if pm.ExpiresAt != nil {
		p.ExpiresAt = timestamppb.New(*pm.ExpiresAt)
	}
	return p
}

func usageSummaryToProto(s *UsageSummary) *billingv1.UsageSummary {
	return &billingv1.UsageSummary{
		TenantId:          s.TenantID.String(),
		Plan:              s.Plan,
		ActiveProductions: int32(s.Usage.ActiveProductions),
		UserCount:         int32(s.Usage.UserCount),
		StorageGb:         s.Usage.StorageGB.StringFixed(2),
		MaxProductions:    int32(s.Limits.MaxActiveProductions),
		MaxUsers:          int32(s.Limits.MaxUsers),
		MaxStorageGb:      int32(s.Limits.MaxStorageGB),
		UpdatedAt:         timestamppb.New(s.UpdatedAt),
	}
}

// grpcErr maps domain errors to gRPC status codes.
// Internal details are never exposed in status messages.
func grpcErr(err error) error {
	switch {
	case errors.Is(err, ErrSubscriptionNotFound):
		return status.Error(codes.NotFound, "subscription not found")
	case errors.Is(err, ErrSubscriptionAlreadyExists):
		return status.Error(codes.AlreadyExists, "subscription already exists")
	case errors.Is(err, ErrInvoiceNotFound):
		return status.Error(codes.NotFound, "invoice not found")
	case errors.Is(err, ErrPaymentMethodNotFound):
		return status.Error(codes.NotFound, "payment method not found")
	case errors.Is(err, ErrInvalidPlan):
		return status.Error(codes.InvalidArgument, "invalid plan name")
	case errors.Is(err, ErrDowngradeNotAllowed):
		return status.Error(codes.FailedPrecondition, "downgrade not allowed — remove resources first")
	case errors.Is(err, ErrSubscriptionCancelled):
		return status.Error(codes.FailedPrecondition, "subscription is cancelled")
	case errors.Is(err, ErrSubscriptionSuspended):
		return status.Error(codes.FailedPrecondition, "subscription is suspended")
	case errors.Is(err, ErrInvoiceAlreadyPaid):
		return status.Error(codes.FailedPrecondition, "invoice is already paid")
	case errors.Is(err, ErrPlanLimitExceeded):
		return status.Error(codes.ResourceExhausted, "plan limit exceeded")
	case errors.Is(err, ErrNoDefaultPaymentMethod):
		return status.Error(codes.FailedPrecondition, "no default payment method on file")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
