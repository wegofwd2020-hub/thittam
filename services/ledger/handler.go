package ledger

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	ledgerv1 "github.com/wegofwd2020/thittam/gen/ledger/v1"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Handler implements the gRPC LedgerService.
// The general-ledger is a universal service — it does not load vertical config
// per request. Tenant IDs are taken directly from the request fields, enabling
// calls from other services (IAM seeder, reporting) without an HTTP context.
type Handler struct {
	ledgerv1.UnimplementedLedgerServiceServer
	svc *Service
}

// NewHandler creates a Handler wrapping the given Service.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Compile-time interface check.
var _ ledgerv1.LedgerServiceServer = (*Handler)(nil)

// --- Accounts ---

func (h *Handler) CreateAccount(ctx context.Context, req *ledgerv1.CreateAccountRequest) (*ledgerv1.Account, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}

	account := &Account{
		TenantID:    tenantID,
		Code:        req.GetCode(),
		Name:        req.GetName(),
		AccountType: req.GetAccountType(),
	}

	if s := req.GetParentId(); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid parent_id")
		}
		account.ParentID = &id
	}

	out, err := h.svc.CreateAccount(ctx, account)
	if err != nil {
		return nil, grpcErr(err)
	}
	return accountToProto(out), nil
}

func (h *Handler) GetAccount(ctx context.Context, req *ledgerv1.GetAccountRequest) (*ledgerv1.Account, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid account ID")
	}

	account, err := h.svc.GetAccount(ctx, tenantID, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return accountToProto(account), nil
}

func (h *Handler) ListAccounts(ctx context.Context, req *ledgerv1.ListAccountsRequest) (*ledgerv1.ListAccountsResponse, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}

	// Proto does not carry limit/offset yet — apply a server-side cap.
	const defaultLimit = 200
	accounts, err := h.svc.ListAccounts(ctx, tenantID, defaultLimit, 0)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*ledgerv1.Account, len(accounts))
	for i := range accounts {
		out[i] = accountToProto(&accounts[i])
	}
	return &ledgerv1.ListAccountsResponse{Accounts: out}, nil
}

func (h *Handler) SeedChartOfAccounts(ctx context.Context, req *ledgerv1.SeedChartOfAccountsRequest) (*ledgerv1.SeedChartOfAccountsResponse, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}

	protoEntries := req.GetEntries()
	entries := make([]vertical.ChartOfAccountEntry, len(protoEntries))
	for i, e := range protoEntries {
		entries[i] = vertical.ChartOfAccountEntry{
			Code:        e.GetCode(),
			Name:        e.GetName(),
			AccountType: e.GetAccountType(),
		}
		if pc := e.GetParentCode(); pc != "" {
			s := pc
			entries[i].ParentCode = &s
		}
	}

	if err := h.svc.SeedChartOfAccounts(ctx, tenantID, entries); err != nil {
		return nil, grpcErr(err)
	}

	// The service handles idempotent skipping internally; counts are not exposed.
	return &ledgerv1.SeedChartOfAccountsResponse{
		Created: int32(len(entries)),
		Skipped: 0,
	}, nil
}

// --- Accounting Periods ---

func (h *Handler) OpenAccountingPeriod(ctx context.Context, req *ledgerv1.OpenAccountingPeriodRequest) (*ledgerv1.OpenAccountingPeriodResponse, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}

	if err := h.svc.OpenAccountingPeriod(ctx, tenantID, int(req.GetYear()), time.Month(req.GetMonth())); err != nil {
		return nil, grpcErr(err)
	}

	// The service is idempotent; we cannot distinguish "created" from "already existed"
	// without an extra round-trip, so AlreadyExisted is always false here.
	return &ledgerv1.OpenAccountingPeriodResponse{AlreadyExisted: false}, nil
}

func (h *Handler) CloseAccountingPeriod(ctx context.Context, req *ledgerv1.CloseAccountingPeriodRequest) (*ledgerv1.AccountingPeriod, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}

	periodID, err := uuid.Parse(req.GetPeriodId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid period_id")
	}

	closedBy, err := uuid.Parse(req.GetClosedBy())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid closed_by")
	}

	period, err := h.svc.CloseAccountingPeriod(ctx, tenantID, periodID, closedBy)
	if err != nil {
		return nil, grpcErr(err)
	}
	return periodToProto(period), nil
}

// --- Journal Entries ---

func (h *Handler) CreateJournalEntry(ctx context.Context, req *ledgerv1.CreateJournalEntryRequest) (*ledgerv1.JournalEntry, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}

	periodID, err := uuid.Parse(req.GetPeriodId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid period_id")
	}

	lines := make([]JournalLine, len(req.GetLines()))
	for i, l := range req.GetLines() {
		accountID, err := uuid.Parse(l.GetAccountId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "line %d: invalid account_id", i)
		}
		debit, err := decimal.NewFromString(l.GetDebitAmount())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "line %d: invalid debit_amount", i)
		}
		credit, err := decimal.NewFromString(l.GetCreditAmount())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "line %d: invalid credit_amount", i)
		}
		lines[i] = JournalLine{
			AccountID:    accountID,
			DebitAmount:  debit,
			CreditAmount: credit,
			Currency:     l.GetCurrency(),
			Description:  l.GetDescription(),
		}
	}

	je := &JournalEntry{
		TenantID:  tenantID,
		PeriodID:  periodID,
		Reference: req.GetReference(),
		Narration: req.GetNarration(),
		Lines:     lines,
	}

	if s := req.GetProductionId(); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid production_id")
		}
		je.ProductionID = &id
	}

	out, err := h.svc.CreateJournalEntry(ctx, je)
	if err != nil {
		return nil, grpcErr(err)
	}
	return journalEntryToProto(out), nil
}

func (h *Handler) PostJournalEntry(ctx context.Context, req *ledgerv1.PostJournalEntryRequest) (*ledgerv1.JournalEntry, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid journal entry ID")
	}

	postedBy, err := uuid.Parse(req.GetPostedBy())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid posted_by")
	}

	je, err := h.svc.PostJournalEntry(ctx, tenantID, id, postedBy)
	if err != nil {
		return nil, grpcErr(err)
	}
	return journalEntryToProto(je), nil
}

func (h *Handler) GetJournalEntry(ctx context.Context, req *ledgerv1.GetJournalEntryRequest) (*ledgerv1.JournalEntry, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid journal entry ID")
	}

	je, err := h.svc.GetJournalEntry(ctx, tenantID, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return journalEntryToProto(je), nil
}

func (h *Handler) ListJournalEntries(ctx context.Context, req *ledgerv1.ListJournalEntriesRequest) (*ledgerv1.ListJournalEntriesResponse, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}

	var periodID *uuid.UUID
	if s := req.GetPeriodId(); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid period_id")
		}
		periodID = &id
	}

	entries, err := h.svc.ListJournalEntries(ctx, tenantID, periodID, req.GetStatus(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*ledgerv1.JournalEntry, len(entries))
	for i := range entries {
		out[i] = journalEntryToProto(&entries[i])
	}
	return &ledgerv1.ListJournalEntriesResponse{Entries: out}, nil
}

func (h *Handler) VoidJournalEntry(ctx context.Context, req *ledgerv1.VoidJournalEntryRequest) (*ledgerv1.JournalEntry, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid journal entry ID")
	}

	voidedBy, err := uuid.Parse(req.GetVoidedBy())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid voided_by")
	}

	reversing, err := h.svc.VoidJournalEntry(ctx, tenantID, id, voidedBy)
	if err != nil {
		return nil, grpcErr(err)
	}
	return journalEntryToProto(reversing), nil
}

// --- Trial Balance ---

func (h *Handler) GetTrialBalance(ctx context.Context, req *ledgerv1.GetTrialBalanceRequest) (*ledgerv1.GetTrialBalanceResponse, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}

	asOf := time.Now().UTC()
	if ts := req.GetAsOf(); ts != nil {
		asOf = ts.AsTime()
	}

	entries, err := h.svc.GetTrialBalance(ctx, tenantID, asOf)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*ledgerv1.TrialBalanceEntry, len(entries))
	for i, e := range entries {
		out[i] = &ledgerv1.TrialBalanceEntry{
			AccountId:   e.AccountID.String(),
			Code:        e.Code,
			Name:        e.Name,
			AccountType: e.AccountType,
			Debit:       e.Debit.StringFixed(2),
			Credit:      e.Credit.StringFixed(2),
			Balance:     e.Balance.StringFixed(2),
		}
	}
	return &ledgerv1.GetTrialBalanceResponse{Entries: out}, nil
}

// --- proto conversion helpers ---

func accountToProto(a *Account) *ledgerv1.Account {
	out := &ledgerv1.Account{
		Id:          a.ID.String(),
		TenantId:    a.TenantID.String(),
		Code:        a.Code,
		Name:        a.Name,
		AccountType: a.AccountType,
		IsActive:    a.IsActive,
	}
	if a.ParentID != nil {
		out.ParentId = a.ParentID.String()
	}
	return out
}

func periodToProto(p *AccountingPeriod) *ledgerv1.AccountingPeriod {
	out := &ledgerv1.AccountingPeriod{
		Id:       p.ID.String(),
		TenantId: p.TenantID.String(),
		Year:     int32(p.Year),
		Month:    int32(p.Month),
		Status:   p.Status,
	}
	if p.ClosedBy != nil {
		out.ClosedBy = p.ClosedBy.String()
	}
	if p.ClosedAt != nil {
		out.ClosedAt = timestamppb.New(*p.ClosedAt)
	}
	return out
}

func journalEntryToProto(je *JournalEntry) *ledgerv1.JournalEntry {
	out := &ledgerv1.JournalEntry{
		Id:          je.ID.String(),
		TenantId:    je.TenantID.String(),
		PeriodId:    je.PeriodID.String(),
		EntryNumber: je.EntryNumber,
		Reference:   je.Reference,
		Narration:   je.Narration,
		Status:      je.Status,
		CreatedAt:   timestamppb.New(je.CreatedAt),
	}
	if je.ProductionID != nil {
		out.ProductionId = je.ProductionID.String()
	}
	if je.PostedBy != nil {
		out.PostedBy = je.PostedBy.String()
	}
	if je.PostedAt != nil {
		out.PostedAt = timestamppb.New(*je.PostedAt)
	}
	if len(je.Lines) > 0 {
		out.Lines = make([]*ledgerv1.JournalLine, len(je.Lines))
		for i, l := range je.Lines {
			out.Lines[i] = &ledgerv1.JournalLine{
				Id:           l.ID.String(),
				JournalId:    l.JournalID.String(),
				AccountId:    l.AccountID.String(),
				DebitAmount:  l.DebitAmount.StringFixed(2),
				CreditAmount: l.CreditAmount.StringFixed(2),
				Currency:     l.Currency,
				Description:  l.Description,
			}
		}
	}
	return out
}

// grpcErr maps domain sentinel errors to gRPC status codes.
func grpcErr(err error) error {
	switch {
	case errors.Is(err, ErrAccountNotFound):
		return status.Error(codes.NotFound, "account not found")
	case errors.Is(err, ErrAccountInactive):
		return status.Error(codes.FailedPrecondition, "account is inactive")
	case errors.Is(err, ErrAccountCodeExists):
		return status.Error(codes.AlreadyExists, "account code already exists for tenant")
	case errors.Is(err, ErrInvalidAccountType):
		return status.Error(codes.InvalidArgument, "invalid account type")
	case errors.Is(err, ErrPeriodNotFound):
		return status.Error(codes.NotFound, "accounting period not found")
	case errors.Is(err, ErrPeriodClosed):
		return status.Error(codes.FailedPrecondition, "accounting period is closed")
	case errors.Is(err, ErrPeriodAlreadyExists):
		return status.Error(codes.AlreadyExists, "accounting period already exists")
	case errors.Is(err, ErrJournalNotFound):
		return status.Error(codes.NotFound, "journal entry not found")
	case errors.Is(err, ErrJournalAlreadyPosted):
		return status.Error(codes.FailedPrecondition, "journal entry is already posted")
	case errors.Is(err, ErrJournalVoid):
		return status.Error(codes.FailedPrecondition, "journal entry is void")
	case errors.Is(err, ErrJournalNotBalanced):
		return status.Error(codes.InvalidArgument, "journal entry is not balanced")
	case errors.Is(err, ErrJournalTooFewLines):
		return status.Error(codes.InvalidArgument, "journal entry must have at least two lines")
	}
	return status.Error(codes.Internal, "internal error")
}
