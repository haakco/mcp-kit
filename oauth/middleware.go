package oauth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ory/fosite"
)

type contextKey string

const (
	contextKeyUserID      contextKey = "oauth.user_id"
	contextKeyScopes      contextKey = "oauth.scopes"
	contextKeyScopeType   contextKey = "oauth.scope_type"
	contextKeyScopeTarget contextKey = "oauth.scope_target"
	contextKeyAuthSource  contextKey = "oauth.source"

	// AuthSourceOAuth2 indicates a Fosite-issued bearer token authenticated the request.
	AuthSourceOAuth2 = "oauth2"
	// AuthSourcePAT indicates a personal access token authenticated the request.
	AuthSourcePAT = "pat"
)

type authDisabledKey struct{}

// Target identifies a consumer resource protected by a target-scoped PAT.
type Target struct {
	ScopeType string
	Target    string
}

// TargetResolver resolves the target resource for the current request.
type TargetResolver func(r *http.Request) (Target, error)

// BearerConfig configures Bearer middleware.
type BearerConfig struct {
	Introspector         TokenIntrospector
	SessionFactory       func() fosite.Session
	ResourceMetadataURL  string
	TokenValidator       TokenValidator
	Now                  func() time.Time
	RecordUsageTimeout   time.Duration
	MaxUsageRecorders    int
	AllowUnauthenticated bool
	ExpectedAudience     string
}

// Bearer validates OAuth bearer tokens and optional PATs.
func Bearer(cfg BearerConfig) func(http.Handler) http.Handler {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.SessionFactory == nil {
		cfg.SessionFactory = func() fosite.Session { return NewEmptySession() }
	}
	if cfg.RecordUsageTimeout == 0 {
		cfg.RecordUsageTimeout = 2 * time.Second
	}
	if cfg.MaxUsageRecorders == 0 {
		cfg.MaxUsageRecorders = 32
	}
	usageLimiter := make(chan struct{}, cfg.MaxUsageRecorders)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.Introspector == nil && cfg.TokenValidator == nil && cfg.AllowUnauthenticated {
				next.ServeHTTP(w, r.WithContext(WithAuthDisabled(r.Context())))
				return
			}

			token := extractBearerToken(r)
			if token == "" {
				writeBearerChallenge(w, cfg.ResourceMetadataURL, "", http.StatusUnauthorized)
				return
			}

			if cfg.TokenValidator != nil {
				if result, ok := validatePAT(r.Context(), cfg.TokenValidator, token); ok {
					recordPATUsage(cfg.TokenValidator, result.TokenID, cfg.RecordUsageTimeout, usageLimiter)
					next.ServeHTTP(w, r.WithContext(contextWithPAT(r.Context(), result)))
					return
				}
			}

			if cfg.Introspector == nil {
				writeBearerChallenge(w, cfg.ResourceMetadataURL, "", http.StatusUnauthorized)
				return
			}
			if cfg.ExpectedAudience == "" {
				writeBearerChallenge(w, cfg.ResourceMetadataURL, "", http.StatusUnauthorized)
				return
			}

			_, request, err := cfg.Introspector.IntrospectToken(r.Context(), token, fosite.AccessToken, cfg.SessionFactory())
			if err != nil {
				writeBearerChallenge(w, cfg.ResourceMetadataURL, "", http.StatusUnauthorized)
				return
			}
			if !request.GetGrantedAudience().Has(cfg.ExpectedAudience) {
				writeBearerChallenge(w, cfg.ResourceMetadataURL, "", http.StatusUnauthorized)
				return
			}
			if expiresAt := request.GetSession().GetExpiresAt(fosite.AccessToken); !expiresAt.IsZero() && expiresAt.Before(cfg.Now()) {
				writeBearerChallenge(w, cfg.ResourceMetadataURL, "", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r.WithContext(contextWithOAuth(r.Context(), request)))
		})
	}
}

// RequireScope denies requests that lack scope.
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if IsAuthDisabled(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}
			if !GetScopes(r.Context()).Has(scope) {
				status := http.StatusForbidden
				if len(GetScopes(r.Context())) == 0 {
					status = http.StatusUnauthorized
				}
				writeBearerChallenge(w, "", scope, status)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireScopeForTarget denies requests that lack scope or whose PAT boundary
// does not cover the resolved target. OAuth bearer tokens are checked by scope
// only; PATs with an empty boundary are treated as full access.
func RequireScopeForTarget(scope string, resolve TargetResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return RequireScope(scope)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if GetAuthSource(r.Context()) != AuthSourcePAT {
				next.ServeHTTP(w, r)
				return
			}
			target, err := resolve(r)
			if err != nil || !patCoversTarget(r.Context(), target) {
				writeBearerChallenge(w, "", scope, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		}))
	}
}

// WithAuthDisabled marks a request as intentionally unauthenticated because auth is disabled.
func WithAuthDisabled(ctx context.Context) context.Context {
	return context.WithValue(ctx, authDisabledKey{}, true)
}

// IsAuthDisabled reports whether auth is disabled for this request.
func IsAuthDisabled(ctx context.Context) bool {
	value, _ := ctx.Value(authDisabledKey{}).(bool)
	return value
}

// WithScopes stores granted scopes in context.
func WithScopes(ctx context.Context, scopes fosite.Arguments) context.Context {
	return context.WithValue(ctx, contextKeyScopes, append(fosite.Arguments{}, scopes...))
}

// GetUserID returns the authenticated subject.
func GetUserID(ctx context.Context) string {
	value, _ := ctx.Value(contextKeyUserID).(string)
	return value
}

// GetScopes returns granted scopes.
func GetScopes(ctx context.Context) fosite.Arguments {
	value, _ := ctx.Value(contextKeyScopes).(fosite.Arguments)
	return value
}

// GetScopeType returns the PAT scope type, when present.
func GetScopeType(ctx context.Context) string {
	value, _ := ctx.Value(contextKeyScopeType).(string)
	return value
}

// GetScopeTarget returns the PAT scope target, when present.
func GetScopeTarget(ctx context.Context) string {
	value, _ := ctx.Value(contextKeyScopeTarget).(string)
	return value
}

// GetAuthSource returns the auth mechanism that authenticated the request.
func GetAuthSource(ctx context.Context) string {
	value, _ := ctx.Value(contextKeyAuthSource).(string)
	return value
}

func validatePAT(ctx context.Context, validator TokenValidator, token string) (*PATAuthResult, bool) {
	result, err := validator.ValidateAndResolve(ctx, token)
	return result, err == nil && result != nil
}

func recordPATUsage(validator TokenValidator, tokenID string, timeout time.Duration, limiter chan struct{}) {
	select {
	case limiter <- struct{}{}:
	default:
		return
	}

	go func() {
		defer func() { <-limiter }()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		validator.RecordUsage(ctx, tokenID)
	}()
}

func contextWithPAT(ctx context.Context, result *PATAuthResult) context.Context {
	ctx = context.WithValue(ctx, contextKeyUserID, result.UserID)
	ctx = context.WithValue(ctx, contextKeyScopeType, result.ScopeType)
	ctx = context.WithValue(ctx, contextKeyScopeTarget, result.ScopeTarget)
	ctx = context.WithValue(ctx, contextKeyAuthSource, AuthSourcePAT)
	return WithScopes(ctx, fosite.Arguments(result.Scopes))
}

func contextWithOAuth(ctx context.Context, request fosite.AccessRequester) context.Context {
	ctx = context.WithValue(ctx, contextKeyUserID, request.GetSession().GetSubject())
	ctx = context.WithValue(ctx, contextKeyAuthSource, AuthSourceOAuth2)
	return WithScopes(ctx, request.GetGrantedScopes())
}

func patCoversTarget(ctx context.Context, target Target) bool {
	scopeType := GetScopeType(ctx)
	scopeTarget := GetScopeTarget(ctx)
	if scopeType == "" && scopeTarget == "" {
		return true
	}
	return scopeType == target.ScopeType && scopeTarget == target.Target
}

func extractBearerToken(r *http.Request) string {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func writeBearerChallenge(w http.ResponseWriter, resourceMetadataURL string, requiredScope string, status int) {
	challenge := `Bearer realm="mcp-kit"`
	if resourceMetadataURL != "" {
		challenge += `, resource_metadata="` + resourceMetadataURL + `"`
	}
	if requiredScope != "" {
		challenge += `, scope="` + requiredScope + `"`
	}

	w.Header().Set("WWW-Authenticate", challenge)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if status == http.StatusForbidden {
		writeOAuthErrorBody(w, "insufficient_scope", fmt.Sprintf("requires scope: %s", requiredScope))
		return
	}
	writeOAuthErrorBody(w, "invalid_token", "Bearer token required")
}
