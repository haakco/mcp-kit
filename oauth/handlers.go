package oauth

import (
	"encoding/base64"
	"net/http"

	"github.com/ory/fosite"
)

// SubjectResolver returns the authenticated subject for an authorize request.
type SubjectResolver func(r *http.Request) (Subject, error)

// AuthorizeHandler returns an OAuth authorization endpoint that grants
// requested scopes immediately to whatever SubjectResolver returns.
//
// This is a demo handler. It does not authenticate the browser session, does
// not collect explicit user consent, and does not bind the canonical resource
// audience server-side per RFC 8707. Production servers should replace it with
// oauth/consent.NewHandler.
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
		if !validCodeChallenge(r.URL.Query().Get("code_challenge"), r.URL.Query().Get("code_challenge_method")) {
			http.Error(w, "invalid_code_challenge", http.StatusBadRequest)
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
		grantSubjectScopes(requester, subject.GrantedScopes)
		p.grantDefaultAudience(requester)

		response, err := p.oauth.NewAuthorizeResponse(ctx, requester, NewSession(subject))
		if err != nil {
			p.oauth.WriteAuthorizeError(ctx, w, requester, err)
			return
		}
		p.oauth.WriteAuthorizeResponse(ctx, w, requester, response)
	})
}

func validCodeChallenge(challenge string, method string) bool {
	if method != "S256" {
		return false
	}
	if len(challenge) != 43 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(challenge)
	return err == nil
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
		p.grantDefaultAudience(requester)

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

func (p *Provider) grantDefaultAudience(requester interface {
	GetRequestedAudience() fosite.Arguments
	SetRequestedAudience(fosite.Arguments)
	GrantAudience(string)
}) {
	if len(requester.GetRequestedAudience()) == 0 && p.audience != "" {
		requester.SetRequestedAudience(fosite.Arguments{p.audience})
	}
	for _, audience := range requester.GetRequestedAudience() {
		requester.GrantAudience(audience)
	}
}

func grantSubjectScopes(requester interface {
	GetRequestedScopes() fosite.Arguments
	GrantScope(string)
}, granted []string) {
	allowed := map[string]struct{}{}
	for _, scope := range granted {
		allowed[scope] = struct{}{}
	}
	for _, scope := range requester.GetRequestedScopes() {
		if _, ok := allowed[scope]; ok {
			requester.GrantScope(scope)
		}
	}
}
