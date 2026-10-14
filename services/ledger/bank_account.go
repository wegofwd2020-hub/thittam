package ledger

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const (
	bankCodeMin        = 1000
	bankCodeMax        = 1999
	bankCodeStep       = 10
	bankCreateAttempts = 3
	bankNameMaxLen     = 100
)

var (
	accountNumberRE = regexp.MustCompile(`^[0-9]{4,}$`)
	bankCurrencyRE  = regexp.MustCompile(`^[A-Z]{3}$`)
)

// BankAccountInput is a request to create a bank account. AccountNumber is used
// only to derive the last four digits. It is never stored or logged.
type BankAccountInput struct {
	BankName      string
	AccountNumber string
	Currency      string
}

// CreateBankAccount creates an asset account for a bank. The service assigns
// the next free code in the 1000-1999 range and stores only the last four
// digits of the account number.
func (s *Service) CreateBankAccount(ctx context.Context, tenantID uuid.UUID, in BankAccountInput) (*Account, error) {
	bankName := strings.TrimSpace(in.BankName)
	if bankName == "" || len(bankName) > bankNameMaxLen {
		return nil, ErrBankNameInvalid
	}
	digits := strings.NewReplacer(" ", "", "-", "").Replace(in.AccountNumber)
	if !accountNumberRE.MatchString(digits) {
		return nil, ErrAccountNumberInvalid
	}
	currency := strings.ToUpper(strings.TrimSpace(in.Currency))
	if !bankCurrencyRE.MatchString(currency) {
		return nil, ErrBankCurrencyInvalid
	}
	last4 := digits[len(digits)-4:]

	// Another request can take the same code between the read and the insert.
	// The unique (tenant, code) constraint rejects the loser, so retry with a
	// fresh code.
	var lastErr error
	for range bankCreateAttempts {
		highest, err := s.repo.NextBankAccountCode(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("ledger: next bank code: %w", err)
		}
		code, err := nextBankCode(highest)
		if err != nil {
			return nil, err
		}
		account := &Account{
			ID:           uuid.New(),
			TenantID:     tenantID,
			Code:         code,
			Name:         fmt.Sprintf("%s (ending %s)", bankName, last4),
			AccountType:  "asset",
			IsActive:     true,
			BankName:     bankName,
			AccountLast4: last4,
			BankCurrency: currency,
		}
		err = s.repo.CreateAccount(ctx, account)
		if err == nil {
			return account, nil
		}
		if !errors.Is(err, ErrAccountCodeExists) {
			return nil, fmt.Errorf("ledger: create bank account: %w", err)
		}
		lastErr = err
	}
	return nil, lastErr
}

// nextBankCode returns the code after the highest existing 1xxx code. Zero
// means the tenant has none in range yet, so the range start is used.
func nextBankCode(highest int) (string, error) {
	next := bankCodeMin
	if highest > 0 {
		next = highest + bankCodeStep
	}
	if next > bankCodeMax {
		return "", ErrNoBankCodeAvailable
	}
	return strconv.Itoa(next), nil
}
