package ledger

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ledgerv1 "github.com/wegofwd2020/thittam/gen/ledger/v1"
)

func twoLineEntry(partyID *uuid.UUID) *JournalEntry {
	return &JournalEntry{
		TenantID:  fixedTenantID,
		PeriodID:  uuid.New(),
		Narration: "Catering invoice",
		Lines: []JournalLine{
			{AccountID: uuid.New(), DebitAmount: decimal.RequireFromString("100.00"), PartyID: partyID},
			{AccountID: uuid.New(), CreditAmount: decimal.RequireFromString("100.00")},
		},
	}
}

func TestCreateJournalEntry_PartyMustExist(t *testing.T) {
	t.Parallel()
	pid := uuid.New()
	svc := NewService(&mockRepo{getPartyFn: func(_ context.Context, _, _ uuid.UUID) (*Party, error) {
		return nil, ErrPartyNotFound
	}})
	_, err := svc.CreateJournalEntry(context.Background(), twoLineEntry(&pid))
	assert.ErrorIs(t, err, ErrPartyNotFound)
}

func TestCreateJournalEntry_PartyTaggedLinePasses(t *testing.T) {
	t.Parallel()
	pid := uuid.New()
	svc := NewService(&mockRepo{})
	je, err := svc.CreateJournalEntry(context.Background(), twoLineEntry(&pid))
	require.NoError(t, err)
	require.NotNil(t, je.Lines[0].PartyID)
	assert.Equal(t, pid, *je.Lines[0].PartyID)
}

func TestListPartyTransactions_PartyNotFound(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{getPartyFn: func(_ context.Context, _, _ uuid.UUID) (*Party, error) {
		return nil, ErrPartyNotFound
	}})
	_, err := svc.ListPartyTransactions(context.Background(), fixedTenantID, uuid.New())
	assert.ErrorIs(t, err, ErrPartyNotFound)
}

func TestListPartyTransactions_TotalsAreExact(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{listPartyLinesFn: func(_ context.Context, _, _ uuid.UUID) ([]PartyLedgerLine, error) {
		return []PartyLedgerLine{
			{EntryDate: time.Now(), DebitAmount: decimal.RequireFromString("0.10"), CreditAmount: decimal.Zero},
			{EntryDate: time.Now(), DebitAmount: decimal.RequireFromString("0.20"), CreditAmount: decimal.Zero},
			{EntryDate: time.Now(), DebitAmount: decimal.Zero, CreditAmount: decimal.RequireFromString("0.30")},
		}, nil
	}})
	out, err := svc.ListPartyTransactions(context.Background(), fixedTenantID, uuid.New())
	require.NoError(t, err)
	assert.Len(t, out.Lines, 3)
	assert.Equal(t, "0.30", out.TotalDebit.StringFixed(2), "decimal sum, no float drift")
	assert.Equal(t, "0.30", out.TotalCredit.StringFixed(2))
}

func TestHandler_ListPartyTransactions_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	pid := uuid.New()
	when := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	h := NewHandler(NewService(&mockRepo{listPartyLinesFn: func(_ context.Context, _, _ uuid.UUID) ([]PartyLedgerLine, error) {
		return []PartyLedgerLine{{EntryID: uuid.New(), EntryNumber: "JE-2026-0001", EntryDate: when,
			AccountCode: "1010", AccountName: "Business Checking",
			DebitAmount: decimal.RequireFromString("500.00"), CreditAmount: decimal.Zero, Currency: "USD"}}, nil
	}}), allowAllPerm{})
	resp, err := h.ListPartyTransactions(callerCtx(tid), &ledgerv1.ListPartyTransactionsRequest{TenantId: tid.String(), PartyId: pid.String()})
	require.NoError(t, err)
	require.Len(t, resp.GetLines(), 1)
	assert.Equal(t, "2026-10-20T09:00:00Z", resp.GetLines()[0].GetEntryDate())
	assert.Equal(t, "500.00", resp.GetLines()[0].GetDebitAmount())
	assert.Equal(t, "500.00", resp.GetTotalDebit())
	assert.Equal(t, "0.00", resp.GetTotalCredit())
}

func TestHandler_ListPartyTransactions_InvalidID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().ListPartyTransactions(callerCtx(tid), &ledgerv1.ListPartyTransactionsRequest{TenantId: tid.String(), PartyId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateJournalEntry_InvalidLinePartyID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().CreateJournalEntry(callerCtx(tid), &ledgerv1.CreateJournalEntryRequest{
		TenantId:  tid.String(),
		PeriodId:  uuid.New().String(),
		Narration: "x",
		Lines: []*ledgerv1.JournalLineInput{
			{AccountId: uuid.New().String(), DebitAmount: "10.00", CreditAmount: "0.00", Currency: "USD", PartyId: "bad"},
			{AccountId: uuid.New().String(), DebitAmount: "0.00", CreditAmount: "10.00", Currency: "USD"},
		},
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
