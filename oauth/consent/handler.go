package consent

import (
	"crypto/rand"
	"errors"
	"net/http"
	"net/url"

	"github.com/ory/fosite"

	"github.com/haakco/mcp-kit/oauth"
)

// Handler is the kit's canonical /oauth/authorize handler.
type Handler struct {
	cfg Config
}

// NewHandler validates cfg, wires defaults, and returns a Handler.
func NewHandler(cfg Config) (*Handler, error) {
	if err := cfg.applyDefaults(); err != nil {
		return nil, err
	}
	if cfg.ApprovalStore == nil {
		if len(cfg.ApprovalSecret) == 0 {
			cfg.ApprovalSecret = make([]byte, approvalSecretLength)
			if _, err := rand.Read(cfg.ApprovalSecret); err != nil {
				return nil, err
			}
			cfg.Logger.Warn("consent: approval secret not configured; generated ephemeral secret")
		} else if err := ValidateApprovalSecret(cfg.ApprovalSecret); err != nil {
			return nil, err
		}
		cfg.ApprovalStore = newDefaultApprovalStore(cfg.ApprovalSecret, cfg.Now)
	}
	return &Handler{cfg: cfg}, nil
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.renderLogin(w, r.URL.Query(), "")
	case http.MethodPost:
		h.handlePost(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handlePost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.FormBodyLimit)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form body", http.StatusBadRequest)
		return
	}
	cleanParams := oauthAuthorizeValues(r.PostForm)
	requester, err := buildAuthorizeRequest(r.Context(), h.cfg.Provider, h.cfg.PublicURL, h.cfg.FormPath, cleanParams, r)
	if err != nil {
		WriteAuthorizeError(w, err)
		return
	}
	if err := validateResourceIndicators(r, cleanParams, h.cfg.ResourceURL, h.cfg.Logger); err != nil {
		WriteAuthorizeError(w, err)
		return
	}

	switch r.PostForm.Get("action") {
	case "login":
		h.handleLogin(w, r, requester, cleanParams)
	case "approve":
		h.handleApprove(w, r, requester, cleanParams)
	case "deny":
		_, _ = h.cfg.ApprovalStore.Consume(r.Context(), r.PostForm.Get("approval_token"), cleanParams)
		h.emitConsentEvent(r.Context(), r, ActionConsentDenied, requester, oauth.Subject{}, "denied", ResourcesFromValues(cleanParams))
		h.cfg.Provider.OAuth2Provider().WriteAuthorizeError(r.Context(), w, requester, fosite.ErrAccessDenied)
	default:
		h.renderLogin(w, cleanParams, "unknown authorization action")
	}
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request, requester fosite.AuthorizeRequester, cleanParams url.Values) {
	username := r.PostForm.Get("username")
	password := r.PostForm.Get("password")
	if username == "" || password == "" {
		h.renderLogin(w, cleanParams, "username and password are required")
		return
	}
	subject, err := h.cfg.Authenticator.Authenticate(r.Context(), username, password)
	if err != nil {
		h.renderLogin(w, cleanParams, "invalid email or password")
		return
	}
	requestedScopes := ArgumentsToStrings(requester.GetRequestedScopes())
	if err := h.cfg.ConsentPolicy.ValidateScopes(r.Context(), subject, requestedScopes); err != nil {
		h.cfg.Provider.OAuth2Provider().WriteAuthorizeError(r.Context(), w, requester, fosite.ErrAccessDenied.WithHint(err.Error()))
		return
	}
	if h.cfg.ConsentPolicy.AllowsSkip(r.Context(), requester.GetClient(), subject, requestedScopes) {
		h.completeApprove(w, r, requester, cleanParams, subject)
		return
	}
	if h.cfg.Challenge != nil {
		challengeID, err := h.cfg.Challenge.Begin(r.Context(), subject)
		if errors.Is(err, ErrChallengeRequired) {
			h.renderLoginWithChallenge(w, cleanParams, challengeID)
			return
		}
		if err != nil {
			WriteAuthorizeError(w, fosite.ErrServerError.WithWrap(err))
			return
		}
	}
	token, err := h.cfg.ApprovalStore.Issue(r.Context(), subject, cleanParams)
	if err != nil {
		WriteAuthorizeError(w, fosite.ErrServerError.WithWrap(err))
		return
	}
	withToken := cloneAndSet(cleanParams, "approval_token", token)
	h.cfg.Renderer.Render(w, PageConsent, PageData{
		Authenticated: true,
		DisplayName:   subject.Email,
		ClientName:    ClientNameFromRequester(requester),
		Scopes:        requestedScopes,
		Resources:     ResourcesFromValues(withToken),
		HiddenInputs:  HiddenInputs(withToken),
		FormAction:    h.cfg.FormPath,
	})
}

func (h *Handler) handleApprove(w http.ResponseWriter, r *http.Request, requester fosite.AuthorizeRequester, cleanParams url.Values) {
	subject, err := h.cfg.ApprovalStore.Consume(r.Context(), r.PostForm.Get("approval_token"), cleanParams)
	if err != nil {
		h.renderLogin(w, cleanParams, "approval expired; sign in again")
		return
	}
	h.completeApprove(w, r, requester, cleanParams, subject)
}

func (h *Handler) completeApprove(w http.ResponseWriter, r *http.Request, requester fosite.AuthorizeRequester, cleanParams url.Values, subject oauth.Subject) {
	requestedScopes := ArgumentsToStrings(requester.GetRequestedScopes())
	if err := h.cfg.ConsentPolicy.ValidateScopes(r.Context(), subject, requestedScopes); err != nil {
		h.cfg.Provider.OAuth2Provider().WriteAuthorizeError(r.Context(), w, requester, fosite.ErrAccessDenied.WithHint(err.Error()))
		return
	}
	for _, scope := range requestedScopes {
		requester.GrantScope(scope)
	}
	resource, err := h.cfg.ResourceURL(r)
	if err != nil {
		h.cfg.Provider.OAuth2Provider().WriteAuthorizeError(r.Context(), w, requester, fosite.ErrInvalidRequest.WithHint("invalid mcp host"))
		return
	}
	requester.GrantAudience(resource)

	response, err := h.cfg.Provider.OAuth2Provider().NewAuthorizeResponse(r.Context(), requester, oauth.NewSession(subject))
	if err != nil {
		h.cfg.Provider.OAuth2Provider().WriteAuthorizeError(r.Context(), w, requester, err)
		return
	}
	h.emitConsentEvent(r.Context(), r, ActionConsentApproved, requester, subject, "approved", ResourcesFromValues(cleanParams))

	redirectURL := RedirectURLFromResponder(requester, response)
	if redirectURL == "" {
		h.cfg.Provider.OAuth2Provider().WriteAuthorizeResponse(r.Context(), w, requester, response)
		return
	}
	tracker := &trackingWriter{ResponseWriter: w}
	h.cfg.Renderer.Render(tracker, PageRedirectBridge, PageData{
		Authenticated: true,
		ClientName:    ClientNameFromRequester(requester),
		RedirectURL:   redirectURL,
	})
	if !tracker.wrote {
		http.Redirect(w, r, redirectURL, http.StatusFound)
	}
}

func (h *Handler) renderLogin(w http.ResponseWriter, params url.Values, msg string) {
	h.cfg.Renderer.Render(w, PageLogin, PageData{
		ClientName:   "your MCP client",
		Resources:    ResourcesFromValues(params),
		HiddenInputs: HiddenInputs(params),
		FormAction:   h.cfg.FormPath,
		Error:        msg,
	})
}

func (h *Handler) renderLoginWithChallenge(w http.ResponseWriter, params url.Values, challengeID string) {
	withChallenge := cloneAndSet(params, "challenge_id", challengeID)
	h.cfg.Renderer.Render(w, PageLogin, PageData{
		HiddenInputs: HiddenInputs(withChallenge),
		FormAction:   h.cfg.FormPath,
		Error:        "additional verification required",
	})
}

func cloneAndSet(in url.Values, key string, value string) url.Values {
	out := make(url.Values, len(in)+1)
	for k, values := range in {
		out[k] = append([]string{}, values...)
	}
	out.Set(key, value)
	return out
}

type trackingWriter struct {
	http.ResponseWriter
	wrote bool
}

func (t *trackingWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		t.wrote = true
	}
	return t.ResponseWriter.Write(p)
}

func (t *trackingWriter) WriteHeader(statusCode int) {
	t.wrote = true
	t.ResponseWriter.WriteHeader(statusCode)
}
