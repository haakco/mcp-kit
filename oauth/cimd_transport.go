package oauth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// errBlockedTarget reports that a client metadata URL resolved to a network
// location the authorization server must not fetch from.
var errBlockedTarget = errors.New("client metadata: blocked network target")

const (
	clientMetadataFetchTimeout  = 5 * time.Second
	clientMetadataMaxBytes      = 64 << 10
	clientMetadataMaxRedirects  = 3
	clientMetadataDialTimeout   = 5 * time.Second
	clientMetadataMaxCacheItems = 256
)

// newClientMetadataHTTPClient returns an HTTP client for fetching client
// metadata documents.
//
// The server fetches a URL supplied by an unauthenticated caller, so the client
// is bounded on every axis that matters: it refuses loopback, private,
// link-local, multicast, and unspecified addresses after DNS resolution, dials
// the address it validated rather than re-resolving it, caps redirects, and
// applies a total timeout. Response size is bounded by the caller.
func newClientMetadataHTTPClient(allowLoopback bool) *http.Client {
	dialer := &net.Dialer{Timeout: clientMetadataDialTimeout}
	resolver := &net.Resolver{}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("client metadata: split address %q: %w", address, err)
			}
			addrs, err := resolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("client metadata: resolve %q: %w", host, err)
			}
			if len(addrs) == 0 {
				return nil, fmt.Errorf("%w: %q has no addresses", errBlockedTarget, host)
			}

			var lastErr error
			for _, addr := range addrs {
				if !clientMetadataTargetAllowed(addr.IP, allowLoopback) {
					lastErr = fmt.Errorf("%w: %s", errBlockedTarget, addr.IP)
					continue
				}
				// Dial the address that was validated. Re-resolving here would let
				// a rebinding resolver swap in a blocked address between the check
				// and the connection.
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(addr.IP.String(), port))
				if dialErr != nil {
					lastErr = dialErr
					continue
				}
				return conn, nil
			}
			if lastErr == nil {
				lastErr = fmt.Errorf("%w: %q", errBlockedTarget, host)
			}
			return nil, lastErr
		},
		DisableKeepAlives:   true,
		MaxIdleConns:        0,
		TLSHandshakeTimeout: clientMetadataDialTimeout,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   clientMetadataFetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= clientMetadataMaxRedirects {
				return fmt.Errorf("client metadata: exceeded %d redirects", clientMetadataMaxRedirects)
			}
			// TLS certificate validation still uses the URL host, so a redirect
			// cannot smuggle in a different identity.
			if _, ok := clientMetadataURL(req.URL, allowLoopback); !ok {
				return fmt.Errorf("client metadata: redirect to disallowed URL %q", req.URL.Redacted())
			}
			return nil
		},
	}
}

// clientMetadataTargetAllowed reports whether the server may connect to ip.
// Loopback is permitted only when the deployment has explicitly opted into
// local development.
func clientMetadataTargetAllowed(ip net.IP, allowLoopback bool) bool {
	if ip.IsLoopback() {
		return allowLoopback
	}
	return !ip.IsPrivate() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() &&
		!ip.IsInterfaceLocalMulticast() &&
		!ip.IsMulticast() &&
		!ip.IsUnspecified()
}
