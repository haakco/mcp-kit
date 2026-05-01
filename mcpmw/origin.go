package mcpmw

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// OriginConfig configures the Origin allowlist middleware.
type OriginConfig struct {
	// Allowed is the set of permitted Origin header values (exact match).
	// Empty means no browser clients are allowed (loopback may still be
	// permitted via AllowLoopback).
	Allowed []string

	// AllowLoopback permits Origin values that resolve to a loopback host
	// (127.0.0.0/8, ::1, or the literal "localhost"). Set true in dev.
	AllowLoopback bool

	// OnDeny, if set, is called instead of writing the default 403 response.
	// Use this to integrate with the consumer's error-envelope conventions.
	OnDeny func(w http.ResponseWriter, r *http.Request, origin string)
}

// Origin enforces an allowlist on the Origin header. Empty origin (non-browser
// client) is allowed. Allowed origins pass through. Loopback origins pass
// through when AllowLoopback is true. Anything else is rejected with 403.
func Origin(cfg OriginConfig, next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(cfg.Allowed))
	for _, o := range cfg.Allowed {
		allowed[o] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		if _, ok := allowed[origin]; ok {
			next.ServeHTTP(w, r)
			return
		}

		if cfg.AllowLoopback && isLoopbackOrigin(origin) {
			next.ServeHTTP(w, r)
			return
		}

		if cfg.OnDeny != nil {
			cfg.OnDeny(w, r, origin)
			return
		}
		http.Error(w, "origin not allowed", http.StatusForbidden)
	})
}

// isLoopbackOrigin reports whether the given Origin value is loopback (any
// scheme, hostname is 127.x, ::1, or localhost; port is irrelevant).
func isLoopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
