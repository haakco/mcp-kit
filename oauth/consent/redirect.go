package consent

import (
	"net/url"
	"strings"

	"github.com/ory/fosite"
)

// ClientNameFromRequester returns the client's display name.
func ClientNameFromRequester(req fosite.AuthorizeRequester) string {
	if req == nil || req.GetClient() == nil {
		return "your MCP client"
	}
	client := req.GetClient()
	if provider, ok := client.(interface{ GetName() string }); ok {
		if name := strings.TrimSpace(provider.GetName()); name != "" {
			return name
		}
	}
	if id := strings.TrimSpace(client.GetID()); id != "" {
		return id
	}
	return "your MCP client"
}

// RedirectURLFromResponder builds the client redirect URL with OAuth response params.
func RedirectURLFromResponder(req fosite.AuthorizeRequester, resp fosite.AuthorizeResponder) string {
	redirectURI := req.GetRedirectURI()
	if redirectURI == nil {
		return ""
	}
	out := *redirectURI
	query := out.Query()
	for key, values := range resp.GetParameters() {
		delete(query, key)
		for _, value := range values {
			query.Add(key, value)
		}
	}
	out.RawQuery = query.Encode()
	return out.String()
}

// HiddenInputs converts URL values to renderer-friendly hidden inputs.
func HiddenInputs(values url.Values) []HiddenInput {
	out := make([]HiddenInput, 0, len(values))
	for key, vals := range values {
		for _, value := range vals {
			out = append(out, HiddenInput{Name: key, Value: value})
		}
	}
	return out
}

// ResourcesFromValues returns normalized resource indicator values.
func ResourcesFromValues(values url.Values) []string {
	out := normalizeOAuthStringSlice(values["resource"])
	if len(out) == 0 {
		return nil
	}
	return out
}

// ArgumentsToStrings copies fosite arguments to a string slice.
func ArgumentsToStrings(args fosite.Arguments) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, len(args))
	copy(out, args)
	return out
}
