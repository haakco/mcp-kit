package consent

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
)

func oauthAuthorizeValues(values url.Values) url.Values {
	clean := make(url.Values, len(values))
	for key, vals := range values {
		switch key {
		case "username", "password", "action", "approval_token", "challenge_id", "challenge_response":
			continue
		}
		for _, value := range vals {
			clean.Add(key, value)
		}
	}
	return clean
}

// ParamsDigest returns a stable digest of credential-free OAuth params.
func ParamsDigest(values url.Values) string {
	clean := oauthAuthorizeValues(values)
	sum := sha256.Sum256([]byte(clean.Encode()))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func normalizeOAuthStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
