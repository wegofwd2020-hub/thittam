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
	ID              uuid.UUID      `json:"id"`
	TenantID        uuid.UUID      `json:"tenant_id"`
	Name            string         `json:"name"`
	PartyType       string         `json:"party_type"`
	Email           string         `json:"email"`
	Phone           string         `json:"phone"`
	TaxID           string         `json:"tax_id"`
	Address         string         `json:"address"`
	DefaultCurrency string         `json:"default_currency"`
	IsActive        bool           `json:"is_active"`
	Contacts        []PartyContact `json:"contacts"`
}

// PartyContact is a person at a party, to reach about a specific matter.
type PartyContact struct {
	ID          uuid.UUID `json:"id"`
	PartyID     uuid.UUID `json:"party_id"`
	Name        string    `json:"name"`
	Designation string    `json:"designation"`
	Email       string    `json:"email"`
	Phone       string    `json:"phone"`
	IsPrimary   bool      `json:"is_primary"`
}

const (
	partyContactsMax   = 20
	contactNameMaxLen  = 200
	contactDesigMaxLen = 100
	contactPhoneMaxLen = 30
)

// CreateParty validates and stores a new party.
func (s *Service) CreateParty(ctx context.Context, p *Party) (*Party, error) {
	if err := normalizeParty(p); err != nil {
		return nil, err
	}
	p.ID = uuid.New()
	p.IsActive = true
	assignContactIDs(p)
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
	// Contacts are replaced as a set, so every row gets a fresh ID.
	assignContactIDs(p)
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
	return normalizeContacts(p.Contacts)
}

// normalizeContacts trims each contact, requires a name and at least one way
// to reach the person, and enforces one primary contact. When contacts exist
// but none is marked primary, the first one becomes primary.
func normalizeContacts(contacts []PartyContact) error {
	if len(contacts) > partyContactsMax {
		return fmt.Errorf("%w: at most %d contacts per party", ErrContactInvalid, partyContactsMax)
	}
	primaries := 0
	for i := range contacts {
		c := &contacts[i]
		n := i + 1
		c.Name = strings.TrimSpace(c.Name)
		c.Designation = strings.TrimSpace(c.Designation)
		c.Email = strings.TrimSpace(c.Email)
		c.Phone = strings.TrimSpace(c.Phone)

		switch {
		case c.Name == "" || utf8.RuneCountInString(c.Name) > contactNameMaxLen:
			return fmt.Errorf("%w: contact %d needs a name of at most %d characters", ErrContactInvalid, n, contactNameMaxLen)
		case utf8.RuneCountInString(c.Designation) > contactDesigMaxLen:
			return fmt.Errorf("%w: contact %d designation is longer than %d characters", ErrContactInvalid, n, contactDesigMaxLen)
		case utf8.RuneCountInString(c.Phone) > contactPhoneMaxLen:
			return fmt.Errorf("%w: contact %d phone is longer than %d characters", ErrContactInvalid, n, contactPhoneMaxLen)
		case c.Email == "" && c.Phone == "":
			return fmt.Errorf("%w: contact %d needs an email or a phone number", ErrContactInvalid, n)
		}
		if c.Email != "" {
			addr, err := mail.ParseAddress(c.Email)
			if err != nil || addr.Address != c.Email || len(c.Email) > partyEmailMaxLen {
				return fmt.Errorf("%w: contact %d email is not a valid address", ErrContactInvalid, n)
			}
		}
		if c.IsPrimary {
			primaries++
		}
	}
	if primaries > 1 {
		return fmt.Errorf("%w: only one contact can be primary", ErrContactInvalid)
	}
	if len(contacts) > 0 && primaries == 0 {
		contacts[0].IsPrimary = true
	}
	return nil
}

// assignContactIDs gives each contact an ID and links it to its party.
func assignContactIDs(p *Party) {
	for i := range p.Contacts {
		p.Contacts[i].ID = uuid.New()
		p.Contacts[i].PartyID = p.ID
	}
}
