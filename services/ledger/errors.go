package ledger

import "errors"

var (
	ErrAccountNotFound      = errors.New("ledger: account not found")
	ErrAccountInactive      = errors.New("ledger: account is inactive")
	ErrAccountCodeExists    = errors.New("ledger: account code already exists for tenant")
	ErrInvalidAccountType   = errors.New("ledger: invalid account type; must be asset, liability, equity, revenue, or expense")
	ErrPeriodNotFound       = errors.New("ledger: accounting period not found")
	ErrPeriodClosed         = errors.New("ledger: accounting period is closed")
	ErrPeriodAlreadyExists  = errors.New("ledger: accounting period already exists")
	ErrJournalNotFound      = errors.New("ledger: journal entry not found")
	ErrJournalAlreadyPosted = errors.New("ledger: journal entry is already posted")
	ErrJournalVoid          = errors.New("ledger: journal entry is void")
	ErrJournalNotBalanced   = errors.New("ledger: journal entry is not balanced: sum(debit) ≠ sum(credit)")
	ErrJournalTooFewLines   = errors.New("ledger: journal entry must have at least two lines")
)
