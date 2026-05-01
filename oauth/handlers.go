package oauth

import (
	"log/slog"
	"net/http"
)

// SubjectResolver returns the authenticated subject for an authorize request.
type SubjectResolver func(r *http.Request) (Subject, error)

// AuthorizeHandler returns an OAuth authorization endpoint. Consumers are
// responsible for authenticating the browser request and collecting consent
// before this handler grants requested scopes.
func (p *Provider) AuthorizeHandler(resolve SubjectResolver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if state := r.URL.Query().Get("state"); len(state) < 8 {
			http.Error(w, "invalid_state", http.StatusBadRequest)
			return
		}

		requester, err := p.oauth.NewAuthorizeRequest(ctx, r)
		if err != nil {
			p.oauth.WriteAuthorizeError(ctx, w, requester, err)
			return
		}

		subject, err := resolve(r)
		if err != nil {
			p.oauth.WriteAuthorizeError(ctx, w, requester, err)
			return
		}
		for _, scope := range requester.GetRequestedScopes() {
			requester.GrantScope(scope)
		}

		response, err := p.oauth.NewAuthorizeResponse(ctx, requester, NewSession(subject))
		if err != nil {
			p.oauth.WriteAuthorizeError(ctx, w, requester, err)
			return
		}
		p.oauth.WriteAuthorizeResponse(ctx, w, requester, response)
	})
}

// TokenHandler returns the OAuth token endpoint.
func (p *Provider) TokenHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		requester, err := p.oauth.NewAccessRequest(ctx, r, NewEmptySession())
		if err != nil {
			p.oauth.WriteAccessError(ctx, w, requester, err)
			return
		}
		for _, scope := range requester.GetRequestedScopes() {
			requester.GrantScope(scope)
		}

		response, err := p.oauth.NewAccessResponse(ctx, requester)
		if err != nil {
			p.oauth.WriteAccessError(ctx, w, requester, err)
			return
		}
		p.oauth.WriteAccessResponse(ctx, w, requester, response)
	})
}

// RevokeHandler returns the OAuth token revocation endpoint.
func (p *Provider) RevokeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := p.oauth.NewRevocationRequest(ctx, r); err != nil {
			p.oauth.WriteRevocationResponse(ctx, w, err)
			return
		}
		p.oauth.WriteRevocationResponse(ctx, w, nil)
	})
}

// RegisterRoutes mounts OAuth handlers on mux with prefix, for example
// prefix "/oauth" mounts "/oauth/authorize", "/oauth/token", "/oauth/revoke",
// and "/oauth/register".
func (p *Provider) RegisterRoutes(mux *http.ServeMux, prefix string, resolve SubjectResolver) {
	if prefix == "" {
		prefix = "/oauth"
	}
	mux.Handle(prefix+"/authorize", p.AuthorizeHandler(resolve))
	mux.Handle(prefix+"/token", p.TokenHandler())
	mux.Handle(prefix+"/revoke", p.RevokeHandler())
	mux.Handle(prefix+"/register", p.RegisterHandler())
}

func logHandlerError(message string, err error) {
	if err != nil {
		slog.Error(message, "error", err)
	}
}
