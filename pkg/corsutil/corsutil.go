// Package corsutil provides shared CORS helpers for service wiring.
//
// Production CORS is handled by Kong at the edge. This package exists for two
// interim states where Kong is not yet in front of the services:
//
//   1. Local dev — `go run ./cmd/<svc>` loaded from localhost or a LAN IP
//      (e.g. 192.168.x.x) during a customer demo.
//   2. Pre-Kong cloud deploys — services hosted directly on a public VM
//      under a real hostname (e.g. https://thittam.demo.example.com) so
//      early-access users can try the app before the edge layer lands.
//
// The helper returned by OriginFunc accepts: (a) any loopback / RFC-1918 host
// on the Next.js dev-server ports, and (b) any exact origin string passed in
// the `extra` list (typically loaded from CORS_EXTRA_ORIGINS at startup).
package corsutil

import (
	"net"
	"net/url"
	"os"
	"strings"
)

// webPorts are the ports the Next.js dev server is allowed to run on.
// :3100 is Thittam's current port; :3000 is kept because StudyBuddy uses it
// and some contributors still run the Thittam web there historically.
var webPorts = map[string]bool{
	"3100": true,
	"3000": true,
}

// OriginFunc returns a CORS AllowOriginFunc that accepts:
//   - any http://<host>:<port> where host is a loopback or RFC-1918 address
//     and port is one of the Next.js dev-server ports (3100, 3000), and
//   - any origin whose exact string matches an entry in `extra`.
//
// Use `extra` to admit public demo hostnames (e.g. https://demo.example.com)
// in cloud deploys where Kong is not yet fronting the services. Entries are
// compared as exact strings — no wildcards — so misconfiguration fails closed.
func OriginFunc(extra ...string) func(origin string) bool {
	extras := make(map[string]bool, len(extra))
	for _, o := range extra {
		if o = strings.TrimSpace(o); o != "" {
			extras[o] = true
		}
	}
	return func(origin string) bool {
		if extras[origin] {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "http" {
			return false
		}
		host, port := u.Hostname(), u.Port()
		if !webPorts[port] {
			return false
		}
		if host == "localhost" {
			return true
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return false
		}
		return ip.IsLoopback() || ip.IsPrivate()
	}
}

// LocalDevOriginFunc is OriginFunc with no extras — local-dev-only.
// Retained as a thin alias so callers that don't need cloud origins can
// read as "this is local dev only" at the call site.
func LocalDevOriginFunc() func(origin string) bool {
	return OriginFunc()
}

// ExtraOriginsFromEnv parses CORS_EXTRA_ORIGINS (comma-separated exact
// origins) and returns the list. Empty entries and surrounding whitespace
// are trimmed out. When the env var is unset, returns nil.
func ExtraOriginsFromEnv() []string {
	raw := os.Getenv("CORS_EXTRA_ORIGINS")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
