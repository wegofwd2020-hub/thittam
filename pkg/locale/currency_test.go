package locale

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrencyForCountry(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"IN", "INR", false},
		{"in", "INR", false}, // case-insensitive
		{" US ", "USD", false},
		{"DE", "EUR", false},
		{"FR", "EUR", false},
		{"GB", "GBP", false},
		{"XX", "", true}, // unknown
		{"I", "", true},  // too short
		{"", "", true},
	}
	for _, tc := range cases {
		got, err := CurrencyForCountry(tc.in)
		if tc.wantErr {
			assert.Error(t, err, "input %q should error", tc.in)
			continue
		}
		require.NoError(t, err)
		assert.Equal(t, tc.want, got)
	}
}

func TestIsKnownCountry(t *testing.T) {
	t.Parallel()
	assert.True(t, IsKnownCountry("IN"))
	assert.True(t, IsKnownCountry("us"))
	assert.False(t, IsKnownCountry("ZZ"))
	assert.False(t, IsKnownCountry(""))
}
