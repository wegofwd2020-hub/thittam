package expense

import "errors"

var (
	ErrExpenseNotFound       = errors.New("expense: expense not found")
	ErrPONotFound            = errors.New("expense: purchase order not found")
	ErrAlreadyApproved       = errors.New("expense: expense already approved")
	ErrAlreadyRejected       = errors.New("expense: expense already rejected")
	ErrAlreadySettled        = errors.New("expense: petty cash advance already settled")
	ErrUnspentExceedsAdvance = errors.New("expense: unspent amount exceeds advance amount")
	ErrApprovalLimitExceeded = errors.New("expense: amount exceeds approval limit for role")
	ErrDualApprovalRequired  = errors.New("expense: amount exceeds dual approval threshold")
	ErrPORequired            = errors.New("expense: purchase order required for this category")
	ErrInvalidCategory       = errors.New("expense: invalid expense category for this vertical")
	ErrInsufficientBudget    = errors.New("expense: insufficient budget remaining")
)
