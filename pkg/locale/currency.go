// Package locale provides static ISO country/currency lookups.
//
// The map is deliberately a pure lookup table — no runtime fetches, no
// external deps. Add entries as tenants in new countries onboard.
//
// Some countries have multiple official currencies (PR/USD, SM/EUR);
// callers can override the result at the tenant level via
// primary_currency_code. This lookup returns the most commonly expected
// business currency.
package locale

import (
	"fmt"
	"strings"
)

// countryCurrency maps ISO 3166-1 alpha-2 country codes → ISO 4217 currency.
//
// Coverage aims for the long tail of SaaS customers rather than every
// sovereign state — extend as needed.
var countryCurrency = map[string]string{
	"AE": "AED", "AR": "ARS", "AT": "EUR", "AU": "AUD", "BD": "BDT",
	"BE": "EUR", "BG": "BGN", "BH": "BHD", "BR": "BRL", "CA": "CAD",
	"CH": "CHF", "CL": "CLP", "CN": "CNY", "CO": "COP", "CY": "EUR",
	"CZ": "CZK", "DE": "EUR", "DK": "DKK", "EE": "EUR", "EG": "EGP",
	"ES": "EUR", "FI": "EUR", "FR": "EUR", "GB": "GBP", "GR": "EUR",
	"HK": "HKD", "HR": "EUR", "HU": "HUF", "ID": "IDR", "IE": "EUR",
	"IL": "ILS", "IN": "INR", "IS": "ISK", "IT": "EUR", "JP": "JPY",
	"KE": "KES", "KR": "KRW", "KW": "KWD", "LK": "LKR", "LT": "EUR",
	"LU": "EUR", "LV": "EUR", "MA": "MAD", "MT": "EUR", "MX": "MXN",
	"MY": "MYR", "NG": "NGN", "NL": "EUR", "NO": "NOK", "NZ": "NZD",
	"OM": "OMR", "PE": "PEN", "PH": "PHP", "PK": "PKR", "PL": "PLN",
	"PT": "EUR", "QA": "QAR", "RO": "RON", "RU": "RUB", "SA": "SAR",
	"SE": "SEK", "SG": "SGD", "SI": "EUR", "SK": "EUR", "TH": "THB",
	"TR": "TRY", "TW": "TWD", "UA": "UAH", "US": "USD", "VN": "VND",
	"ZA": "ZAR",
}

// CurrencyForCountry returns the default ISO 4217 currency for a country.
// Input is case-insensitive; returns an error if the country is unknown.
func CurrencyForCountry(countryCode string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(countryCode))
	if len(code) != 2 {
		return "", fmt.Errorf("locale: country_code must be 2 letters, got %q", countryCode)
	}
	cur, ok := countryCurrency[code]
	if !ok {
		return "", fmt.Errorf("locale: no default currency mapping for country %q — supply primary_currency_code explicitly", code)
	}
	return cur, nil
}

// IsKnownCountry reports whether the 2-letter code has a currency mapping.
func IsKnownCountry(countryCode string) bool {
	_, err := CurrencyForCountry(countryCode)
	return err == nil
}
