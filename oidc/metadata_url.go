package oidc

import "github.com/haakco/mcp-kit/oauth"

// ProtectedResourceMetadataPathFor returns the RFC 9728 well-known metadata
// path for an OAuth protected resource URL.
func ProtectedResourceMetadataPathFor(resourceURL string) (string, error) {
	return oauth.ProtectedResourceMetadataPathFor(resourceURL)
}

// ProtectedResourceMetadataURLFor returns the absolute RFC 9728 well-known
// metadata URL for an OAuth protected resource URL.
func ProtectedResourceMetadataURLFor(resourceURL string) (string, error) {
	return oauth.ProtectedResourceMetadataURLFor(resourceURL)
}
