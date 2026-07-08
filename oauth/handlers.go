package oauth

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/haakco/mcp-kit/audit"
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
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}

		grantType := r.PostForm.Get("grant_type")
		clientID := r.PostForm.Get("client_id")
		if grantType == "refresh_token" && p.replayWindow > 0 {
			p.handleReplayableRefresh(w, r, clientID)
			return
		}

		recorder := p.executeTokenRequest(ctx, r, grantType, clientID, false)
		writeReplay(w, recorder.replay())
	})
}

func (p *Provider) handleReplayableRefresh(w http.ResponseWriter, r *http.Request, clientID string) {
	refreshToken := r.PostForm.Get("refresh_token")
	if refreshToken == "" {
		recorder := p.executeTokenRequest(r.Context(), r, "refresh_token", clientID, false)
		writeReplay(w, recorder.replay())
		return
	}
	key := refreshReplayKey(clientID, refreshToken)
	now := p.now().UTC()
	if replay, ok := p.replayCache.get(key, now); ok {
		writeReplay(w, replay)
		p.emitTokenAudit(r.Context(), "oauth_refresh", "replayed", nil, "refresh_token", clientID, nil, true)
		return
	}

	value, _, _ := p.replayCache.group.Do(key, func() (any, error) {
		if replay, ok := p.replayCache.get(key, p.now().UTC()); ok {
			return replay, nil
		}
		recorder := p.executeTokenRequest(r.Context(), r, "refresh_token", clientID, false)
		replay := recorder.replay()
		p.storeRefreshReplay(clientID, refreshToken, replay)
		return replay, nil
	})
	replay, ok := value.(replayedTokenResponse)
	if !ok || replay.status == 0 {
		http.Error(w, "token exchange failed", http.StatusInternalServerError)
		return
	}
	writeReplay(w, replay)
}

func (p *Provider) executeTokenRequest(
	ctx context.Context,
	r *http.Request,
	grantType string,
	clientID string,
	replayed bool,
) *tokenResponseRecorder {
	recorder := newTokenResponseRecorder()
	requester, err := p.oauth.NewAccessRequest(ctx, r, NewEmptySession())
	if err != nil {
		p.emitTokenAudit(ctx, "oauth_token_exchange", "failed", requester, grantType, clientID, err, replayed)
		p.oauth.WriteAccessError(ctx, recorder, requester, err)
		return recorder
	}
	clientID = requester.GetClient().GetID()
	p.grantDefaultAudience(requester)

	response, err := p.oauth.NewAccessResponse(ctx, requester)
	if err != nil {
		p.emitTokenAudit(ctx, "oauth_token_exchange", "failed", requester, grantType, clientID, err, replayed)
		p.oauth.WriteAccessError(ctx, recorder, requester, err)
		return recorder
	}
	p.oauth.WriteAccessResponse(ctx, recorder, requester, response)
	if grantType == "refresh_token" {
		p.emitTokenAudit(ctx, "oauth_refresh", "rotated", requester, grantType, clientID, nil, replayed)
	} else {
		p.emitTokenAudit(ctx, "oauth_token", "issued", requester, grantType, clientID, nil, replayed)
	}
	return recorder
}

func (p *Provider) storeRefreshReplay(clientID string, refreshToken string, replay replayedTokenResponse) {
	if replay.status < http.StatusOK || replay.status >= http.StatusMultipleChoices {
		return
	}
	if refreshToken == "" {
		return
	}
	p.replayCache.set(refreshReplayKey(clientID, refreshToken), replayedTokenResponse{
		body:      replay.body,
		header:    replay.header,
		status:    replay.status,
		expiresAt: p.now().UTC().Add(p.replayWindow),
	})
}

func writeReplay(w http.ResponseWriter, replay replayedTokenResponse) {
	for key, values := range replay.header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(replay.status)
	_, _ = w.Write(replay.body) //nolint:errcheck // replay response already committed; caller cannot recover from write errors
}

func (p *Provider) emitTokenAudit(
	ctx context.Context,
	entityType string,
	action string,
	requester fosite.AccessRequester,
	grantType string,
	clientID string,
	err error,
	replayed bool,
) {
	if p.auditEmitter == nil {
		return
	}
	scope := ""
	if requester != nil {
		scope = strings.Join(requester.GetGrantedScopes(), " ")
		if clientID == "" && requester.GetClient() != nil {
			clientID = requester.GetClient().GetID()
		}
	}
	event := audit.Event{
		EntityType: entityType,
		Action:     action,
		ClientID:   clientID,
		Scope:      scope,
		Metadata: map[string]any{
			"grant_type": grantType,
			"replayed":   replayed,
		},
		Timestamp: p.now().UTC(),
	}
	if err != nil {
		event.Metadata["error_code"] = oauthErrorCode(err)
	}
	_ = p.auditEmitter.Emit(ctx, event) //nolint:errcheck // audit is non-blocking for token endpoint availability
}

func oauthErrorCode(err error) string {
	var oauthErr *fosite.RFC6749Error
	if errors.As(err, &oauthErr) && oauthErr.ErrorField != "" {
		return oauthErr.ErrorField
	}
	return "unknown"
}

type tokenResponseRecorder struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newTokenResponseRecorder() *tokenResponseRecorder {
	return &tokenResponseRecorder{header: http.Header{}}
}

func (r *tokenResponseRecorder) Header() http.Header {
	return r.header
}

func (r *tokenResponseRecorder) WriteHeader(status int) {
	r.status = status
}

func (r *tokenResponseRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(data)
}

func (r *tokenResponseRecorder) replay() replayedTokenResponse {
	status := r.status
	if status == 0 {
		status = http.StatusOK
	}
	return replayedTokenResponse{
		body:   r.body.Bytes(),
		header: r.header,
		status: status,
	}
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
