package consent

import (
	"log/slog"
	"net/http"
	"net/url"

	"github.com/ory/fosite"
)

func validateResourceIndicators(r *http.Request, params url.Values, resourceURL ResourceURLFunc, logger *slog.Logger) error {
	resources := normalizeOAuthStringSlice(params["resource"])
	if len(resources) == 0 {
		return nil
	}
	expected, err := resourceURL(r)
	if err != nil {
		return fosite.ErrInvalidRequest.WithHint("invalid mcp host")
	}
	if len(resources) != 1 || resources[0] != expected {
		logger.Warn("consent: authorize rejected: resource indicator mismatch",
			"client_id", params.Get("client_id"),
			"provided_resources", resources,
			"expected_resource", expected,
		)
		return fosite.ErrInvalidRequest.WithHint("authorization request must target the current MCP resource")
	}
	return nil
}
