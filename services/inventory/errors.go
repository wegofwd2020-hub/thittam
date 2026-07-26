package inventory

import "errors"

var (
	ErrAssetNotFound     = errors.New("inventory: asset not found")
	ErrAssetNotAvailable = errors.New("inventory: asset is not available for checkout")
	ErrInvalidCategory   = errors.New("inventory: invalid category for this vertical")
	ErrAlreadyCheckedOut = errors.New("inventory: asset is already checked out")
	ErrNoActiveCheckout  = errors.New("inventory: no open checkout for this asset")
)
