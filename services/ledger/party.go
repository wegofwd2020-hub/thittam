package ledger

import (
	"context"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Party types. A "both" party is returned by both customer and supplier filters.
const (
	PartyTypeCustomer = "customer"
	PartyTypeSupplier = "supplier"
	PartyTypeBoth     = "both"
)

const (
	partyNameMaxLen    = 200
	partyEmailMaxLen   = 254
	partyPhoneMaxLen   = 30
	partyTaxIDMaxLen   = 50
	partyAddressMaxLen = 500
)

var partyCurrencyRE = regexp.MustCompile(`^[A-Z]{3}$`)

// Party is a customer, a supplier, or both.
type Party struct {
	ID              uuid.UUID `json:"id"`
	TenantID        uuid.UUID `json:"tenant_id"`
	Name            string    `json:"name"`
	PartyType       string    `json:"party_type"`
	Email           string    `json:"email"`
	Phone           string    `json:"phone"`
	TaxID           string    `json:"tax_id"`
	Address         string    `json:"address"`
	DefaultCurrency string    `json:"default_currency"`
	IsActive        bool      `json:"is_active"`
}

// CreateParty validates and stores a new party.
func (s *Service) CreateParty(ctx context.Context, p *Party) (*Party, error) {
	if err := normalizeParty(p); err != nil {
		return nil, err
	}
	p.ID = uuid.New()
	p.IsActive = true
	if err := s.repo.CreateParty(ctx, p); err != nil {
		return nil, fmt.Errorf("ledger: create party: %w", err)
	}
	return p, nil
}

// GetParty returns one party for the tenant.
func (s *Service) GetParty(ctx context.Context, tenantID, id uuid.UUID) (*Party, error) {
	p, err := s.repo.GetPartyByID(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("ledger: get party %s: %w", id, err)
	}
	return p, nil
}

// ListParties returns the tenant's parties. partyType is optional; when set it
// must be customer or supplier, and "both" parties match either filter.
func (s *Service) ListParties(ctx context.Context, tenantID uuid.UUID, partyType string, includeInactive bool) ([]Party, error) {
	if partyType != "" && partyType != PartyTypeCustomer && partyType != PartyTypeSupplier {
		return nil, ErrPartyTypeInvalid
	}
	parties, err := s.repo.ListParties(ctx, tenantID, partyType, includeInactive)
	if err != nil {
		return nil, fmt.Errorf("ledger: list parties: %w", err)
	}
	return parties, nil
}

// UpdateParty replaces the editable fields of an existing party.
func (s *Service) UpdateParty(ctx context.Context, p *Party) (*Party, error) {
	if err := normalizeParty(p); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateParty(ctx, p); err != nil {
		return nil, fmt.Errorf("ledger: update party %s: %w", p.ID, err)
	}
	return p, nil
}

// normalizeParty trims fields, checks them, and uppercases the currency.
func normalizeParty(p *Party) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || utf8.RuneCountInString(p.Name) > partyNameMaxLen {
		return ErrPartyNameInvalid
	}
	switch p.PartyType {
	case PartyTypeCustomer, PartyTypeSupplier, PartyTypeBoth:
	default:
		return ErrPartyTypeInvalid
	}

	p.Email = strings.TrimSpace(p.Email)
	if p.Email != "" {
		addr, err := mail.ParseAddress(p.Email)
		if err != nil || addr.Address != p.Email || len(p.Email) > partyEmailMaxLen {
			return ErrPartyEmailInvalid
		}
	}

	p.Phone = strings.TrimSpace(p.Phone)
	if utf8.RuneCountInString(p.Phone) > partyPhoneMaxLen {
		return ErrPartyFieldTooLong
	}
	p.TaxID = strings.TrimSpace(p.TaxID)
	if utf8.RuneCountInString(p.TaxID) > partyTaxIDMaxLen {
		return ErrPartyFieldTooLong
	}
	p.Address = strings.TrimSpace(p.Address)
	if utf8.RuneCountInString(p.Address) > partyAddressMaxLen {
		return ErrPartyFieldTooLong
	}

	p.DefaultCurrency = strings.ToUpper(strings.TrimSpace(p.DefaultCurrency))
	if p.DefaultCurrency != "" && !partyCurrencyRE.MatchString(p.DefaultCurrency) {
		return ErrPartyCurrencyInvalid
	}
	return nil
}
