// Package corsutil provides shared CORS helpers for local-dev service wiring.
//
// Production CORS is handled by Kong at the edge; this package exists so that
// `go run ./cmd/<svc>` is demo-ready from any host on the local network —
// localhost for single-machine dev, a LAN IP (192.168.x.x etc.) when presenting
// on someone else's network. Prior approach hardcoded "http://localhost:3100"
// in each service's AllowedOrigins, which broke every time the presenting
// laptop moved to a different WiFi.
package corsutil

import (
	"net"
	"net/url"
)

// webPorts are the ports the Next.js dev server is allowed to run on.
// :3100 is Thittam's current port; :3000 is kept because StudyBuddy uses it
// and some contributors still run the Thittam web there historically.
var webPorts = map[string]bool{
	"3100": true,
	"3000": true,
}

// LocalDevOriginFunc returns a CORS AllowOriginFunc that accepts any
// http://<host>:<port> where the host is a loopback address or an RFC-1918
// private range (10.x, 172.16–31.x, 192.168.x) and the port is one of the
// Next.js dev-server ports. It rejects public hosts and https origins — this
// is a local-dev convenience, not an edge policy.
func LocalDevOriginFunc() func(origin string) bool {
	return func(origin string) bool {
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
