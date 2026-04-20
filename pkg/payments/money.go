package payments

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// currencyDecimals holds ISO 4217 currencies whose minor-unit scale is
// NOT the default 2. Anything not listed here is assumed 2dp. Data per
// ISO 4217; re-check when adding a new currency.
var currencyDecimals = map[string]int32{
	// Zero-decimal — minor unit == major unit
	"BIF": 0, "CLP": 0, "DJF": 0, "GNF": 0, "ISK": 0, "JPY": 0,
	"KMF": 0, "KRW": 0, "PYG": 0, "RWF": 0, "UGX": 0, "UYI": 0,
	"VND": 0, "VUV": 0, "XAF": 0, "XOF": 0, "XPF": 0,

	// Three-decimal
	"BHD": 3, "IQD": 3, "JOD": 3, "KWD": 3, "LYD": 3, "OMR": 3,
	"TND": 3,

	// Four-decimal (rare; only UYW currently)
	"UYW": 4,
}

// CurrencyDecimals returns the number of decimal places a currency's
// minor unit carries. Unknown currencies default to 2 (matches the
// majority of ISO 4217 allocations).
func CurrencyDecimals(code string) int32 {
	if n, ok := currencyDecimals[code]; ok {
		return n
	}
	return 2
}

// ToMinorUnits converts a major-unit Decimal amount ("150.00") into the
// integer minor-unit representation ("15000" for INR, "150" for JPY,
// "150000" for BHD) that PSPs almost universally expect on the wire.
//
// Returns ErrInvalidAmount if:
//   - amount is negative or zero
//   - amount has more decimal places than the currency allows
//     (e.g. 1.005 INR — would silently round with a lesser implementation)
func ToMinorUnits(amount decimal.Decimal, currency string) (int64, error) {
	if !amount.IsPositive() {
		return 0, fmt.Errorf("%w: must be positive, got %s", ErrInvalidAmount, amount.String())
	}
	decimals := CurrencyDecimals(currency)
	// Reject extra precision explicitly — we'd rather 400 than round away
	// fractions of a paise.
	if amount.Exponent() < -decimals {
		return 0, fmt.Errorf("%w: %s has more than %d decimal places for %s",
			ErrInvalidAmount, amount.String(), decimals, currency)
	}
	scaled := amount.Shift(decimals)
	n := scaled.IntPart()
	// Guard against overflow on tiny (improbable) currencies at huge amounts.
	if scaled.Cmp(decimal.NewFromInt(n)) != 0 {
		return 0, fmt.Errorf("%w: amount %s overflows int64 minor units",
			ErrInvalidAmount, amount.String())
	}
	return n, nil
}

// FromMinorUnits is the inverse of ToMinorUnits.
func FromMinorUnits(minor int64, currency string) decimal.Decimal {
	decimals := CurrencyDecimals(currency)
	return decimal.New(minor, -decimals)
}
