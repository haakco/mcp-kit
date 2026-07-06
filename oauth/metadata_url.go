package oauth

import (
	"fmt"
	"net/url"
	"strings"
)

const protectedResourceMetadataBasePath = "/.well-known/oauth-protected-resource"

// ProtectedResourceMetadataPathFor returns the RFC 9728 well-known metadata
// path for an OAuth protected resource URL.
func ProtectedResourceMetadataPathFor(resourceURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(resourceURL))
	if err != nil {
		return "", fmt.Errorf("parse protected resource URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("protected resource URL must be absolute")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("protected resource URL must not include query or fragment")
	}

	resourcePath := strings.TrimRight(parsed.EscapedPath(), "/")
	if resourcePath == "" {
		return protectedResourceMetadataBasePath, nil
	}
	return protectedResourceMetadataBasePath + resourcePath, nil
}

// ProtectedResourceMetadataURLFor returns the absolute RFC 9728 well-known
// metadata URL for an OAuth protected resource URL.
func ProtectedResourceMetadataURLFor(resourceURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(resourceURL))
	if err != nil {
		return "", fmt.Errorf("parse protected resource URL: %w", err)
	}
	metadataPath, err := ProtectedResourceMetadataPathFor(resourceURL)
	if err != nil {
		return "", err
	}

	return parsed.Scheme + "://" + parsed.Host + metadataPath, nil
}
