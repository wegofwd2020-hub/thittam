package corsutil

import "testing"

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
