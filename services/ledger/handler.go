package ledger

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	ledgerv1 "github.com/wegofwd2020/thittam/gen/ledger/v1"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Ledger permissions encode separation of duties: drafting an entry (permLedgerWrite),
// committing it (permLedgerPost), and controlling the books (permLedgerAdmin) are three
// distinct grants.
//
// These duplicate the unexported constants in services/iam/service.go, where systemRoles
// grants them. Go cannot share unexported identifiers across packages; both packages'
// tests assert the string literals, which is the only guard against drift.
const (
	permLedgerRead  = "ledger:read"
	permLedgerWrite = "ledger:write"
	permLedgerPost  = "ledger:post"
	permLedgerAdmin = "ledger:admin"
)

// Handler implements the gRPC LedgerService.
// The general-ledger is a universal service — it does not load vertical config
// per request. The tenant is always taken from the caller's verified token
// (interceptor.TenantFromRequest), never from the request body; a request that
// names a different tenant is rejected. Internal callers that need to act
// across tenants (seeders, cross-tenant reporting) must go through the Service
// layer directly — there is no caller-less path into these handlers.
type Handler struct {
	ledgerv1.UnimplementedLedgerServiceServer
	svc  *Service
	perm interceptor.PermissionChecker
}

// NewHandler creates a Handler wrapping the given Service.
//
// perm is required, not optional. The four other permission-gated services take it via
// a WithPermissionChecker setter, which compiles fine when nobody calls it — yielding a
// handler whose every RPC returns Internal. Here, forgetting the checker is a build error.
// cmd/general-ledger refuses to start when the checker is nil.
func NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler {
	return &Handler{svc: svc, perm: perm}
}

// Compile-time interface check.
var _ ledgerv1.LedgerServiceServer = (*Handler)(nil)

// --- Accounts ---

func (h *Handler) CreateAccount(ctx context.Context, req *ledgerv1.CreateAccountRequest) (*ledgerv1.Account, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}

	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerWrite); err != nil {
		return nil, err
	}

	if req.GetBankName() != "" || req.GetAccountNumber() != "" || req.GetBankCurrency() != "" {
		if req.GetCode() != "" || req.GetParentId() != "" ||
			(req.GetAccountType() != "" && req.GetAccountType() != "asset") {
			return nil, grpcErr(ErrBankAccountNotAsset)
		}
		out, err := h.svc.CreateBankAccount(ctx, tenantID, BankAccountInput{
			BankName:      req.GetBankName(),
			AccountNumber: req.GetAccountNumber(),
			Currency:      req.GetBankCurrency(),
		})
		if err != nil {
			return nil, grpcErr(err)
		}
		return accountToProto(out), nil
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
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}

	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerRead); err != nil {
		return nil, err
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
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}

	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerRead); err != nil {
		return nil, err
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

func (h *Handler) ListAccountingPeriods(ctx context.Context, req *ledgerv1.ListAccountingPeriodsRequest) (*ledgerv1.ListAccountingPeriodsResponse, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}

	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerRead); err != nil {
		return nil, err
	}

	periods, err := h.svc.ListAccountingPeriods(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*ledgerv1.AccountingPeriod, len(periods))
	for i := range periods {
		out[i] = periodToProto(&periods[i])
	}
	return &ledgerv1.ListAccountingPeriodsResponse{Periods: out}, nil
}

func (h *Handler) SeedChartOfAccounts(ctx context.Context, req *ledgerv1.SeedChartOfAccountsRequest) (*ledgerv1.SeedChartOfAccountsResponse, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}

	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerAdmin); err != nil {
		return nil, err
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
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}

	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerAdmin); err != nil {
		return nil, err
	}

	if err := h.svc.OpenAccountingPeriod(ctx, tenantID, int(req.GetYear()), time.Month(req.GetMonth())); err != nil {
		return nil, grpcErr(err)
	}

	// The service is idempotent; we cannot distinguish "created" from "already existed"
	// without an extra round-trip, so AlreadyExisted is always false here.
	return &ledgerv1.OpenAccountingPeriodResponse{AlreadyExisted: false}, nil
}

func (h *Handler) CloseAccountingPeriod(ctx context.Context, req *ledgerv1.CloseAccountingPeriodRequest) (*ledgerv1.AccountingPeriod, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}

	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerAdmin); err != nil {
		return nil, err
	}

	periodID, err := uuid.Parse(req.GetPeriodId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid period_id")
	}

	closedBy, err := interceptor.ActorFromRequest(ctx, req.GetClosedBy())
	if err != nil {
		return nil, err
	}

	period, err := h.svc.CloseAccountingPeriod(ctx, tenantID, periodID, closedBy)
	if err != nil {
		return nil, grpcErr(err)
	}
	return periodToProto(period), nil
}

// --- Journal Entries ---

func (h *Handler) CreateJournalEntry(ctx context.Context, req *ledgerv1.CreateJournalEntryRequest) (*ledgerv1.JournalEntry, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}

	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerWrite); err != nil {
		return nil, err
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
		if s := l.GetPartyId(); s != "" {
			pid, err := uuid.Parse(s)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "line %d: invalid party_id", i)
			}
			lines[i].PartyID = &pid
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
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}

	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerPost); err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid journal entry ID")
	}

	postedBy, err := interceptor.ActorFromRequest(ctx, req.GetPostedBy())
	if err != nil {
		return nil, err
	}

	je, err := h.svc.PostJournalEntry(ctx, tenantID, id, postedBy)
	if err != nil {
		return nil, grpcErr(err)
	}
	return journalEntryToProto(je), nil
}

func (h *Handler) GetJournalEntry(ctx context.Context, req *ledgerv1.GetJournalEntryRequest) (*ledgerv1.JournalEntry, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}

	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerRead); err != nil {
		return nil, err
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
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}

	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerRead); err != nil {
		return nil, err
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
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}

	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerPost); err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid journal entry ID")
	}

	voidedBy, err := interceptor.ActorFromRequest(ctx, req.GetVoidedBy())
	if err != nil {
		return nil, err
	}

	reversing, err := h.svc.VoidJournalEntry(ctx, tenantID, id, voidedBy)
	if err != nil {
		return nil, grpcErr(err)
	}
	return journalEntryToProto(reversing), nil
}

// --- Trial Balance ---

func (h *Handler) GetTrialBalance(ctx context.Context, req *ledgerv1.GetTrialBalanceRequest) (*ledgerv1.GetTrialBalanceResponse, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}

	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerRead); err != nil {
		return nil, err
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

// SetAccountCostBehavior is admin-only: the tag changes how spend is reported,
// so it is treated like other chart-of-accounts configuration.
func (h *Handler) SetAccountCostBehavior(ctx context.Context, req *ledgerv1.SetAccountCostBehaviorRequest) (*ledgerv1.Account, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}

	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerAdmin); err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid account ID")
	}

	account, err := h.svc.SetAccountCostBehavior(ctx, tenantID, id, req.GetCostBehavior())
	if err != nil {
		return nil, grpcErr(err)
	}
	return accountToProto(account), nil
}

// --- Monthly cost behavior ---

func (h *Handler) GetMonthlyCostBehavior(ctx context.Context, req *ledgerv1.GetMonthlyCostBehaviorRequest) (*ledgerv1.GetMonthlyCostBehaviorResponse, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}

	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerRead); err != nil {
		return nil, err
	}

	rows, err := h.svc.GetMonthlyCostBehavior(ctx, tenantID, req.GetFromMonth(), req.GetToMonth())
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*ledgerv1.MonthlyCostBehavior, len(rows))
	for i, r := range rows {
		out[i] = &ledgerv1.MonthlyCostBehavior{
			Month:        r.Month,
			Fixed:        r.Fixed.StringFixed(2),
			Variable:     r.Variable.StringFixed(2),
			Unclassified: r.Unclassified.StringFixed(2),
			Total:        r.Total.StringFixed(2),
		}
	}
	return &ledgerv1.GetMonthlyCostBehaviorResponse{Months: out}, nil
}

// --- proto conversion helpers ---

// --- Parties ---

func (h *Handler) CreateParty(ctx context.Context, req *ledgerv1.CreatePartyRequest) (*ledgerv1.Party, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerWrite); err != nil {
		return nil, err
	}
	out, err := h.svc.CreateParty(ctx, &Party{
		TenantID:        tenantID,
		Name:            req.GetName(),
		PartyType:       req.GetPartyType(),
		Email:           req.GetEmail(),
		Phone:           req.GetPhone(),
		TaxID:           req.GetTaxId(),
		Address:         req.GetAddress(),
		DefaultCurrency: req.GetDefaultCurrency(),
		Contacts:        contactsFromProto(req.GetContacts()),
	})
	if err != nil {
		return nil, grpcErr(err)
	}
	return partyToProto(out), nil
}

func (h *Handler) GetParty(ctx context.Context, req *ledgerv1.GetPartyRequest) (*ledgerv1.Party, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerRead); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid party ID")
	}
	out, err := h.svc.GetParty(ctx, tenantID, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return partyToProto(out), nil
}

func (h *Handler) ListParties(ctx context.Context, req *ledgerv1.ListPartiesRequest) (*ledgerv1.ListPartiesResponse, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerRead); err != nil {
		return nil, err
	}
	parties, err := h.svc.ListParties(ctx, tenantID, req.GetPartyType(), req.GetIncludeInactive())
	if err != nil {
		return nil, grpcErr(err)
	}
	out := &ledgerv1.ListPartiesResponse{Parties: make([]*ledgerv1.Party, 0, len(parties))}
	for i := range parties {
		out.Parties = append(out.Parties, partyToProto(&parties[i]))
	}
	return out, nil
}

func (h *Handler) UpdateParty(ctx context.Context, req *ledgerv1.UpdatePartyRequest) (*ledgerv1.Party, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerWrite); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid party ID")
	}
	out, err := h.svc.UpdateParty(ctx, &Party{
		ID:              id,
		TenantID:        tenantID,
		Name:            req.GetName(),
		PartyType:       req.GetPartyType(),
		Email:           req.GetEmail(),
		Phone:           req.GetPhone(),
		TaxID:           req.GetTaxId(),
		Address:         req.GetAddress(),
		DefaultCurrency: req.GetDefaultCurrency(),
		IsActive:        req.GetIsActive(),
		Contacts:        contactsFromProto(req.GetContacts()),
	})
	if err != nil {
		return nil, grpcErr(err)
	}
	return partyToProto(out), nil
}

func partyToProto(p *Party) *ledgerv1.Party {
	return &ledgerv1.Party{
		Id:              p.ID.String(),
		TenantId:        p.TenantID.String(),
		Name:            p.Name,
		PartyType:       p.PartyType,
		Email:           p.Email,
		Phone:           p.Phone,
		TaxId:           p.TaxID,
		Address:         p.Address,
		DefaultCurrency: p.DefaultCurrency,
		IsActive:        p.IsActive,
		Contacts:        contactsToProto(p.Contacts),
	}
}

func contactsFromProto(in []*ledgerv1.PartyContact) []PartyContact {
	out := make([]PartyContact, 0, len(in))
	for _, c := range in {
		out = append(out, PartyContact{
			Name:        c.GetName(),
			Designation: c.GetDesignation(),
			Email:       c.GetEmail(),
			Phone:       c.GetPhone(),
			IsPrimary:   c.GetIsPrimary(),
		})
	}
	return out
}

func contactsToProto(in []PartyContact) []*ledgerv1.PartyContact {
	out := make([]*ledgerv1.PartyContact, 0, len(in))
	for _, c := range in {
		out = append(out, &ledgerv1.PartyContact{
			Id:          c.ID.String(),
			PartyId:     c.PartyID.String(),
			Name:        c.Name,
			Designation: c.Designation,
			Email:       c.Email,
			Phone:       c.Phone,
			IsPrimary:   c.IsPrimary,
		})
	}
	return out
}

func accountToProto(a *Account) *ledgerv1.Account {
	out := &ledgerv1.Account{
		Id:           a.ID.String(),
		TenantId:     a.TenantID.String(),
		Code:         a.Code,
		Name:         a.Name,
		AccountType:  a.AccountType,
		IsActive:     a.IsActive,
		BankName:     a.BankName,
		AccountLast4: a.AccountLast4,
		BankCurrency: a.BankCurrency,
		CostBehavior: a.CostBehavior,
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
				PartyId:      partyIDString(l.PartyID),
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
	case errors.Is(err, ErrBankNameInvalid),
		errors.Is(err, ErrAccountNumberInvalid),
		errors.Is(err, ErrBankCurrencyInvalid),
		errors.Is(err, ErrBankAccountNotAsset):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ErrCostBehaviorInvalid),
		errors.Is(err, ErrCostBehaviorNotExpense),
		errors.Is(err, ErrMonthRangeInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ErrNoBankCodeAvailable):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, ErrPartyNotFound):
		return status.Error(codes.NotFound, "party not found")
	case errors.Is(err, ErrPartyNameExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, ErrPartyNameInvalid),
		errors.Is(err, ErrPartyTypeInvalid),
		errors.Is(err, ErrPartyEmailInvalid),
		errors.Is(err, ErrPartyFieldTooLong),
		errors.Is(err, ErrPartyCurrencyInvalid),
		errors.Is(err, ErrContactInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
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

// --- Party transactions (view only) ---

func (h *Handler) ListPartyTransactions(ctx context.Context, req *ledgerv1.ListPartyTransactionsRequest) (*ledgerv1.ListPartyTransactionsResponse, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerRead); err != nil {
		return nil, err
	}
	partyID, err := uuid.Parse(req.GetPartyId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid party ID")
	}
	out, err := h.svc.ListPartyTransactions(ctx, tenantID, partyID)
	if err != nil {
		return nil, grpcErr(err)
	}
	resp := &ledgerv1.ListPartyTransactionsResponse{
		Lines:       make([]*ledgerv1.PartyLedgerLine, 0, len(out.Lines)),
		TotalDebit:  out.TotalDebit.StringFixed(2),
		TotalCredit: out.TotalCredit.StringFixed(2),
	}
	for _, l := range out.Lines {
		resp.Lines = append(resp.Lines, &ledgerv1.PartyLedgerLine{
			EntryId:      l.EntryID.String(),
			EntryNumber:  l.EntryNumber,
			EntryDate:    l.EntryDate.UTC().Format(time.RFC3339),
			Narration:    l.Narration,
			AccountCode:  l.AccountCode,
			AccountName:  l.AccountName,
			DebitAmount:  l.DebitAmount.StringFixed(2),
			CreditAmount: l.CreditAmount.StringFixed(2),
			Currency:     l.Currency,
			Description:  l.Description,
		})
	}
	return resp, nil
}

func partyIDString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}
