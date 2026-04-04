package expense

import "errors"

var (
	ErrExpenseNotFound      = errors.New("expense: expense not found")
	ErrPONotFound           = errors.New("expense: purchase order not found")
	ErrAlreadyApproved      = errors.New("expense: expense already approved")
	ErrApprovalLimitExceeded = errors.New("expense: amount exceeds approval limit for role")
	ErrDualApprovalRequired = errors.New("expense: amount exceeds dual approval threshold")
	ErrPORequired           = errors.New("expense: purchase order required for this category")
	ErrInvalidCategory      = errors.New("expense: invalid expense category for this vertical")
	ErrInsufficientBudget   = errors.New("expense: insufficient budget remaining")
)
