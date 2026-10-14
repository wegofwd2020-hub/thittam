package ledger

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validParty() *Party {
	return &Party{TenantID: fixedTenantID, Name: "  Acme Studios  ", PartyType: PartyTypeCustomer, Email: "ap@acme.test", DefaultCurrency: "usd"}
}

func TestCreateParty_NormalizesAndAssignsID(t *testing.T) {
	t.Parallel()
	var saved *Party
	svc := NewService(&mockRepo{createPartyFn: func(_ context.Context, p *Party) error {
		saved = p
		return nil
	}})

	p, err := svc.CreateParty(context.Background(), validParty())
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, p.ID)
	assert.True(t, p.IsActive)
	assert.Equal(t, "Acme Studios", p.Name)
	assert.Equal(t, "USD", p.DefaultCurrency)
	require.NotNil(t, saved)
}

func TestCreateParty_Validation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		mutate  func(*Party)
		wantErr error
	}{
		{"empty name", func(p *Party) { p.Name = "   " }, ErrPartyNameInvalid},
		{"name too long", func(p *Party) { p.Name = string(make([]rune, 201)) }, ErrPartyNameInvalid},
		{"unknown type", func(p *Party) { p.PartyType = "vendor" }, ErrPartyTypeInvalid},
		{"bad email", func(p *Party) { p.Email = "not-an-email" }, ErrPartyEmailInvalid},
		{"email with display name", func(p *Party) { p.Email = "Bob <bob@acme.test>" }, ErrPartyEmailInvalid},
		{"phone too long", func(p *Party) { p.Phone = string(make([]rune, 31)) }, ErrPartyFieldTooLong},
		{"tax id too long", func(p *Party) { p.TaxID = string(make([]rune, 51)) }, ErrPartyFieldTooLong},
		{"address too long", func(p *Party) { p.Address = string(make([]rune, 501)) }, ErrPartyFieldTooLong},
		{"currency not 3 letters", func(p *Party) { p.DefaultCurrency = "US" }, ErrPartyCurrencyInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := NewService(&mockRepo{})
			p := validParty()
			tc.mutate(p)
			_, err := svc.CreateParty(context.Background(), p)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestCreateParty_OptionalFieldsMayBeEmpty(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})
	p := validParty()
	p.Email = ""
	p.DefaultCurrency = ""
	got, err := svc.CreateParty(context.Background(), p)
	require.NoError(t, err)
	assert.Equal(t, "", got.DefaultCurrency)
}

func TestCreateParty_DuplicateNameSurfaces(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{createPartyFn: func(_ context.Context, _ *Party) error {
		return ErrPartyNameExists
	}})
	_, err := svc.CreateParty(context.Background(), validParty())
	assert.ErrorIs(t, err, ErrPartyNameExists)
}

func TestListParties_TypeFilter(t *testing.T) {
	t.Parallel()
	var gotType string
	var gotInactive bool
	svc := NewService(&mockRepo{listPartiesFn: func(_ context.Context, _ uuid.UUID, pt string, inactive bool) ([]Party, error) {
		gotType, gotInactive = pt, inactive
		return nil, nil
	}})

	_, err := svc.ListParties(context.Background(), fixedTenantID, PartyTypeCustomer, true)
	require.NoError(t, err)
	assert.Equal(t, PartyTypeCustomer, gotType)
	assert.True(t, gotInactive)

	_, err = svc.ListParties(context.Background(), fixedTenantID, "vendor", false)
	assert.ErrorIs(t, err, ErrPartyTypeInvalid)
}

func TestGetParty_NotFound(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{getPartyFn: func(_ context.Context, _, _ uuid.UUID) (*Party, error) {
		return nil, ErrPartyNotFound
	}})
	_, err := svc.GetParty(context.Background(), fixedTenantID, uuid.New())
	assert.ErrorIs(t, err, ErrPartyNotFound)
}

func TestUpdateParty_NotFound(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{updatePartyFn: func(_ context.Context, _ *Party) error {
		return ErrPartyNotFound
	}})
	p := validParty()
	p.ID = uuid.New()
	_, err := svc.UpdateParty(context.Background(), p)
	assert.ErrorIs(t, err, ErrPartyNotFound)
}
