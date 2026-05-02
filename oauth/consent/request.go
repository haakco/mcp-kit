package consent

import (
	"context"
	"net/http"
	"net/url"

	"github.com/ory/fosite"

	"github.com/haakco/mcp-kit/oauth"
)

func buildAuthorizeRequest(
	ctx context.Context,
	provider *oauth.Provider,
	publicURL string,
	formPath string,
	cleanParams url.Values,
	original *http.Request,
) (fosite.AuthorizeRequester, error) {
	rawURL := publicURL + formPath + "?" + cleanParams.Encode()
	clone, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fosite.ErrServerError.WithWrap(err)
	}
	if original != nil {
		clone.Header = original.Header.Clone()
		clone.Header.Del("Content-Type")
		clone.Header.Del("Content-Length")
	}
	return provider.OAuth2Provider().NewAuthorizeRequest(ctx, clone)
}
