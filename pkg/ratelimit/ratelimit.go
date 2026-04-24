// Package ratelimit provides an HTTP middleware that caps the number of
// requests a single client IP can make against an endpoint in a fixed time
// window. Intended for credential-stuffing protection on /auth/* endpoints
// before Kong's native rate-limiting lands.
//
// Algorithm: Redis fixed-window counter — INCR the key, set TTL on first hit,
// return 429 once the count exceeds Limit within Window. Fixed windows drift
// by up to 2x at window boundaries (adjacent windows can burst 2*Limit in
// quick succession); acceptable trade-off for login protection and much
// simpler than a sliding log.
//
// The middleware fails *open* if Redis is unreachable. The rate limiter is a
// safety net, not an authorization layer — denying users because of a Redis
// hiccup would be worse than letting an attacker through for 30 seconds.
package ratelimit

import (
	"context"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config configures a rate-limit middleware.
type Config struct {
	// Limit is the maximum number of requests per Window per key.
	Limit int
	// Window is the reset interval.
	Window time.Duration
	// KeyPrefix is prepended to the Redis key so multiple middlewares on the
	// same Redis don't collide. E.g. "iam:auth".
	KeyPrefix string
}

// Middleware returns an http.Handler middleware that rate-limits requests by
// client IP using rdb for counter storage.
func Middleware(rdb *redis.Client, cfg Config) func(http.Handler) http.Handler {
	if cfg.Limit <= 0 || cfg.Window <= 0 {
		panic("ratelimit: Config.Limit and Config.Window must be > 0")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if ip == "" {
				// No usable IP — pass through. Better than failing open on
				// everyone; a misconfigured proxy shouldn't take the site down.
				next.ServeHTTP(w, r)
				return
			}
			key := cfg.KeyPrefix + ":" + ip

			ctx, cancel := context.WithTimeout(r.Context(), 200*time.Millisecond)
			defer cancel()

			count, err := rdb.Incr(ctx, key).Result()
			if err != nil {
				log.Printf("ratelimit: redis INCR failed, passing through: %v", err)
				next.ServeHTTP(w, r)
				return
			}
			if count == 1 {
				// Best-effort TTL on the first hit of a new window.
				if err := rdb.Expire(ctx, key, cfg.Window).Err(); err != nil {
					log.Printf("ratelimit: redis EXPIRE failed: %v", err)
				}
			}
			if count > int64(cfg.Limit) {
				ttl, _ := rdb.TTL(ctx, key).Result()
				retryAfter := int(ttl.Seconds())
				if retryAfter < 1 {
					retryAfter = int(cfg.Window.Seconds())
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"code":8,"message":"Too many requests","details":[]}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP returns the best-guess client IP, preferring proxy headers that
// originate from known reverse-proxy vendors (Cloudflare, Kong/nginx) and
// falling back to r.RemoteAddr for direct (no-proxy) deployments.
//
// In production behind Cloudflare, CF-Connecting-IP is the authoritative
// value — Cloudflare overwrites it per request so it cannot be spoofed by
// clients. If you change proxies, revisit this order.
func clientIP(r *http.Request) string {
	if v := r.Header.Get("CF-Connecting-IP"); v != "" {
		return strings.TrimSpace(v)
	}
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return strings.TrimSpace(v)
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		// XFF is "client, proxy1, proxy2" — first hop is the originating client.
		if i := strings.Index(v, ","); i >= 0 {
			v = v[:i]
		}
		return strings.TrimSpace(v)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
