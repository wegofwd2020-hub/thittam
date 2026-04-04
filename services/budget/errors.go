package budget

import "errors"

var (
	ErrBudgetNotFound   = errors.New("budget: budget not found")
	ErrLineItemNotFound = errors.New("budget: line item not found")
	ErrBudgetLocked     = errors.New("budget: budget is locked")
	ErrBudgetNotApproved = errors.New("budget: budget is not approved")
	ErrInvalidCategory  = errors.New("budget: invalid category for this vertical")
	ErrInsufficientFunds = errors.New("budget: insufficient funds on line item")
	ErrAlreadyApproved  = errors.New("budget: budget is already approved")
)
