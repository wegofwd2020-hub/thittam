package ledger

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validBankInput() BankAccountInput {
	return BankAccountInput{BankName: "HDFC Bank", AccountNumber: "1234-5678 9012", Currency: "usd"}
}

func TestCreateBankAccount_AssignsNextCodeAndStoresLast4(t *testing.T) {
	t.Parallel()
	var saved *Account
	svc := NewService(&mockRepo{
		nextBankCodeFn: func(_ context.Context, _ uuid.UUID) (int, error) { return 1010, nil },
		createAccountFn: func(_ context.Context, a *Account) error {
			saved = a
			return nil
		},
	})

	acct, err := svc.CreateBankAccount(context.Background(), fixedTenantID, validBankInput())
	require.NoError(t, err)
	assert.Equal(t, "1020", acct.Code)
	assert.Equal(t, "asset", acct.AccountType)
	assert.Equal(t, "HDFC Bank", acct.BankName)
	assert.Equal(t, "9012", acct.AccountLast4)
	assert.Equal(t, "USD", acct.BankCurrency)
	assert.Equal(t, "HDFC Bank (ending 9012)", acct.Name)
	assert.True(t, acct.IsActive)
	require.NotNil(t, saved)
	assert.Equal(t, "9012", saved.AccountLast4, "only last 4 digits reach the repository")
}

func TestCreateBankAccount_FirstAccountUsesRangeStart(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})
	acct, err := svc.CreateBankAccount(context.Background(), fixedTenantID, validBankInput())
	require.NoError(t, err)
	assert.Equal(t, "1000", acct.Code)
}

func TestCreateBankAccount_Validation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		mutate  func(*BankAccountInput)
		wantErr error
	}{
		{"empty bank name", func(i *BankAccountInput) { i.BankName = "  " }, ErrBankNameInvalid},
		{"bank name too long", func(i *BankAccountInput) { i.BankName = string(make([]byte, 101)) }, ErrBankNameInvalid},
		{"account number too short", func(i *BankAccountInput) { i.AccountNumber = "123" }, ErrAccountNumberInvalid},
		{"account number has letters", func(i *BankAccountInput) { i.AccountNumber = "12AB5678" }, ErrAccountNumberInvalid},
		{"currency not 3 letters", func(i *BankAccountInput) { i.Currency = "US" }, ErrBankCurrencyInvalid},
		{"currency has digits", func(i *BankAccountInput) { i.Currency = "U5D" }, ErrBankCurrencyInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := NewService(&mockRepo{})
			in := validBankInput()
			tc.mutate(&in)
			_, err := svc.CreateBankAccount(context.Background(), fixedTenantID, in)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestCreateBankAccount_NoCodeLeftInRange(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		nextBankCodeFn: func(_ context.Context, _ uuid.UUID) (int, error) { return 1990, nil },
	})
	_, err := svc.CreateBankAccount(context.Background(), fixedTenantID, validBankInput())
	assert.ErrorIs(t, err, ErrNoBankCodeAvailable)
}

func TestCreateBankAccount_RetriesOnCodeCollision(t *testing.T) {
	t.Parallel()
	attempts := 0
	svc := NewService(&mockRepo{
		createAccountFn: func(_ context.Context, a *Account) error {
			attempts++
			if attempts < 3 {
				return ErrAccountCodeExists
			}
			return nil
		},
	})
	acct, err := svc.CreateBankAccount(context.Background(), fixedTenantID, validBankInput())
	require.NoError(t, err)
	assert.Equal(t, 3, attempts)
	assert.NotEmpty(t, acct.Code)
}

func TestCreateBankAccount_GivesUpAfterRepeatedCollisions(t *testing.T) {
	t.Parallel()
	attempts := 0
	svc := NewService(&mockRepo{
		createAccountFn: func(_ context.Context, _ *Account) error {
			attempts++
			return ErrAccountCodeExists
		},
	})
	_, err := svc.CreateBankAccount(context.Background(), fixedTenantID, validBankInput())
	assert.ErrorIs(t, err, ErrAccountCodeExists)
	assert.Equal(t, bankCreateAttempts, attempts)
}
