package ledger

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ledgerv1 "github.com/wegofwd2020/thittam/gen/ledger/v1"
)

func TestHandler_CreateParty_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(NewService(&mockRepo{}), allowAllPerm{})
	resp, err := h.CreateParty(callerCtx(tid), &ledgerv1.CreatePartyRequest{
		TenantId:        tid.String(),
		Name:            "Acme Studios",
		PartyType:       "customer",
		Email:           "ap@acme.test",
		DefaultCurrency: "usd",
	})
	require.NoError(t, err)
	assert.Equal(t, "Acme Studios", resp.GetName())
	assert.Equal(t, "USD", resp.GetDefaultCurrency())
	assert.True(t, resp.GetIsActive())
	assert.NotEmpty(t, resp.GetId())
}

func TestHandler_CreateParty_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateParty(callerCtx(uuid.New()), &ledgerv1.CreatePartyRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateParty_ValidationIsInvalidArgument(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().CreateParty(callerCtx(tid), &ledgerv1.CreatePartyRequest{
		TenantId: tid.String(), Name: "Acme", PartyType: "vendor",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateParty_DuplicateIsAlreadyExists(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(NewService(&mockRepo{createPartyFn: func(_ context.Context, _ *Party) error {
		return ErrPartyNameExists
	}}), allowAllPerm{})
	_, err := h.CreateParty(callerCtx(tid), &ledgerv1.CreatePartyRequest{
		TenantId: tid.String(), Name: "Acme", PartyType: "customer",
	})
	assert.Equal(t, codes.AlreadyExists, status.Code(err))
}

func TestHandler_GetParty_InvalidID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().GetParty(callerCtx(tid), &ledgerv1.GetPartyRequest{TenantId: tid.String(), Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetParty_NotFound(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(NewService(&mockRepo{getPartyFn: func(_ context.Context, _, _ uuid.UUID) (*Party, error) {
		return nil, ErrPartyNotFound
	}}), allowAllPerm{})
	_, err := h.GetParty(callerCtx(tid), &ledgerv1.GetPartyRequest{TenantId: tid.String(), Id: uuid.New().String()})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestHandler_ListParties_FiltersAndMaps(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(NewService(&mockRepo{listPartiesFn: func(_ context.Context, _ uuid.UUID, _ string, _ bool) ([]Party, error) {
		return []Party{{ID: uuid.New(), TenantID: tid, Name: "Acme", PartyType: "both", IsActive: true}}, nil
	}}), allowAllPerm{})
	resp, err := h.ListParties(callerCtx(tid), &ledgerv1.ListPartiesRequest{TenantId: tid.String(), PartyType: "customer"})
	require.NoError(t, err)
	require.Len(t, resp.GetParties(), 1)
	assert.Equal(t, "both", resp.GetParties()[0].GetPartyType())
}

func TestHandler_ListParties_InvalidTypeIsInvalidArgument(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().ListParties(callerCtx(tid), &ledgerv1.ListPartiesRequest{TenantId: tid.String(), PartyType: "vendor"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_UpdateParty_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(NewService(&mockRepo{}), allowAllPerm{})
	id := uuid.New()
	resp, err := h.UpdateParty(callerCtx(tid), &ledgerv1.UpdatePartyRequest{
		TenantId: tid.String(), Id: id.String(), Name: "Acme", PartyType: "supplier", IsActive: false,
	})
	require.NoError(t, err)
	assert.Equal(t, id.String(), resp.GetId())
	assert.False(t, resp.GetIsActive())
}

func TestHandler_UpdateParty_InvalidID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().UpdateParty(callerCtx(tid), &ledgerv1.UpdatePartyRequest{TenantId: tid.String(), Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_Party_WriteDeniedForReadOnly(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := NewHandler(NewService(&mockRepo{}), denyPerm{}).CreateParty(callerCtx(tid), &ledgerv1.CreatePartyRequest{
		TenantId: tid.String(), Name: "Acme", PartyType: "customer",
	})
	assert.Error(t, err)
}
