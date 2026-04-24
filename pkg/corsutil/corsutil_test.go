package corsutil

import (
	"reflect"
	"testing"
)

func TestLocalDevOriginFunc(t *testing.T) {
	t.Parallel()

	allow := LocalDevOriginFunc()

	cases := []struct {
		origin string
		want   bool
	}{
		{"http://localhost:3100", true},
		{"http://localhost:3000", true},
		{"http://127.0.0.1:3100", true},
		{"http://192.168.68.55:3100", true},
		{"http://10.0.0.5:3000", true},
		{"http://172.16.4.2:3100", true},
		{"http://172.31.255.1:3100", true},

		// Wrong port.
		{"http://localhost:3200", false},
		{"http://192.168.1.1:8080", false},

		// Wrong scheme — https is explicitly out of scope for local dev.
		{"https://localhost:3100", false},

		// Public IPs and non-RFC-1918 ranges.
		{"http://8.8.8.8:3100", false},
		{"http://172.32.0.1:3100", false}, // just outside 172.16-31
		{"http://example.com:3100", false},

		// Garbage.
		{"", false},
		{"not-a-url", false},
		{"http://:3100", false},
	}

	for _, tc := range cases {
		if got := allow(tc.origin); got != tc.want {
			t.Errorf("LocalDevOriginFunc()(%q) = %v, want %v", tc.origin, got, tc.want)
		}
	}
}

func TestOriginFunc_WithExtras(t *testing.T) {
	t.Parallel()

	allow := OriginFunc(
		"https://thittam.demo.example.com",
		"https://preview.example.com",
		"  https://whitespace.example.com  ", // trimmed
		"",                                   // skipped
	)

	cases := []struct {
		origin string
		want   bool
	}{
		// Extras — exact match wins regardless of scheme/port rules.
		{"https://thittam.demo.example.com", true},
		{"https://preview.example.com", true},
		{"https://whitespace.example.com", true},

		// Local dev still works alongside extras.
		{"http://localhost:3100", true},
		{"http://192.168.1.50:3100", true},

		// Not in extras and doesn't pass local-dev check.
		{"https://other.example.com", false},
		{"https://thittam.demo.example.com:8443", false}, // different port = different origin
		{"http://thittam.demo.example.com", false},       // different scheme
		{"", false},
	}

	for _, tc := range cases {
		if got := allow(tc.origin); got != tc.want {
			t.Errorf("OriginFunc(...)(%q) = %v, want %v", tc.origin, got, tc.want)
		}
	}
}

// Not t.Parallel() — uses t.Setenv.
func TestExtraOriginsFromEnv(t *testing.T) {
	cases := []struct {
		name  string
		value string
		set   bool
		want  []string
	}{
		{"unset returns nil", "", false, nil},
		{"empty string returns nil", "", true, nil},
		{"single origin", "https://a.example.com", true, []string{"https://a.example.com"}},
		{
			"comma-separated list",
			"https://a.example.com,https://b.example.com",
			true,
			[]string{"https://a.example.com", "https://b.example.com"},
		},
		{
			"trims whitespace and drops empties",
			" https://a.example.com , ,https://b.example.com  ",
			true,
			[]string{"https://a.example.com", "https://b.example.com"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv("CORS_EXTRA_ORIGINS", tc.value)
			} else {
				// Ensure unset even if the ambient env has it.
				t.Setenv("CORS_EXTRA_ORIGINS", "")
				_ = tc.value
			}
			// The "unset" case above is modelled as empty-string via t.Setenv,
			// which ExtraOriginsFromEnv treats the same (returns nil).
			got := ExtraOriginsFromEnv()
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ExtraOriginsFromEnv() = %v, want %v", got, tc.want)
			}
		})
	}
}
