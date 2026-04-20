package payments

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrencyDecimals(t *testing.T) {
	t.Parallel()
	cases := []struct {
		code string
		want int32
	}{
		{"INR", 2}, {"USD", 2}, {"EUR", 2}, {"GBP", 2},
		{"JPY", 0}, {"KRW", 0}, {"VND", 0}, {"ISK", 0},
		{"BHD", 3}, {"KWD", 3}, {"OMR", 3},
		{"UYW", 4},
		{"ZZZ", 2}, // unknown → default
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, CurrencyDecimals(tc.code), "code %s", tc.code)
	}
}

func TestToMinorUnits_Success(t *testing.T) {
	t.Parallel()
	cases := []struct {
		amount   string
		currency string
		want     int64
	}{
		{"150.00", "INR", 15000},
		{"1.50", "USD", 150},
		{"150", "JPY", 150},           // zero-decimal: major == minor
		{"150.000", "BHD", 150000},    // three-decimal
		{"0.001", "BHD", 1},
		{"999999.99", "USD", 99999999},
	}
	for _, tc := range cases {
		amt := decimal.RequireFromString(tc.amount)
		got, err := ToMinorUnits(amt, tc.currency)
		require.NoError(t, err, "%s %s", tc.amount, tc.currency)
		assert.Equal(t, tc.want, got, "%s %s", tc.amount, tc.currency)
	}
}

func TestToMinorUnits_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name             string
		amount, currency string
	}{
		{"zero", "0.00", "USD"},
		{"negative", "-1.00", "USD"},
		{"extra precision for 2dp currency", "1.005", "USD"},
		{"extra precision for 0dp currency", "1.50", "JPY"},
		{"extra precision for 3dp currency", "1.0001", "BHD"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			amt := decimal.RequireFromString(tc.amount)
			_, err := ToMinorUnits(amt, tc.currency)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidAmount)
		})
	}
}

func TestFromMinorUnits(t *testing.T) {
	t.Parallel()
	cases := []struct {
		minor    int64
		currency string
		want     string
	}{
		{15000, "INR", "150"}, // note: Decimal drops trailing zeros
		{150, "JPY", "150"},
		{150000, "BHD", "150"},
		{1, "BHD", "0.001"},
	}
	for _, tc := range cases {
		got := FromMinorUnits(tc.minor, tc.currency)
		assert.Equal(t, tc.want, got.String(), "minor=%d currency=%s", tc.minor, tc.currency)
	}
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	// 150.00 USD → 15000 minor → 150 major (trailing zeros normalised away
	// but numerically equal).
	start := decimal.RequireFromString("150.00")
	minor, err := ToMinorUnits(start, "USD")
	require.NoError(t, err)
	back := FromMinorUnits(minor, "USD")
	assert.True(t, start.Equal(back), "expected %s, got %s", start, back)
}
