package cliauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultScope = "mcp.read"

// LoginOptions configures a CLI OAuth helper.
type LoginOptions struct {
	Issuer     string
	CredPath   string
	Store      *CredStore
	HTTPClient *http.Client
	OpenURL    func(url string) error
	Endpoints  Endpoints
}

// Endpoints contains OAuth endpoints discovered from the issuer or supplied by callers.
type Endpoints struct {
	Authorization string
	Token         string
	Revocation    string
	Registration  string
}

// NewLogin returns a Login configured for issuer.
func NewLogin(opts LoginOptions) (*Login, error) {
	if _, err := HostFromIssuer(opts.Issuer); err != nil {
		return nil, err
	}
	store := opts.Store
	if store == nil {
		path := opts.CredPath
		if path == "" {
			var err error
			path, err = DefaultCredPath(opts.Issuer)
			if err != nil {
				return nil, err
			}
		}
		store = NewCredStore(path)
	}
	return &Login{
		Issuer:     strings.TrimRight(opts.Issuer, "/"),
		Store:      store,
		HTTPClient: opts.HTTPClient,
		OpenURL:    opts.OpenURL,
		Endpoints:  opts.Endpoints,
	}, nil
}

// Login orchestrates the OAuth/PKCE flow against an MCP server.
type Login struct {
	Issuer     string
	Store      *CredStore
	HTTPClient *http.Client
	OpenURL    func(url string) error
	Endpoints  Endpoints
}

// LoopbackOptions configures the auto-browser PKCE loopback flow.
type LoopbackOptions struct {
	Port  int
	Scope string
}

// PasteOptions configures the paste-redirect PKCE flow.
type PasteOptions struct {
	Port  int
	Scope string
	In    io.Reader
	Out   io.Writer
}

type dcrResponse struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret,omitempty"`
	RedirectURIs []string `json:"redirect_uris"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope,omitempty"`
}

func (l *Login) httpClient() *http.Client {
	if l.HTTPClient != nil {
		return l.HTTPClient
	}
	return http.DefaultClient
}

func (l *Login) openURL(u string) error {
	if l.OpenURL != nil {
		return l.OpenURL(u)
	}
	return openBrowser(u)
}

func (l *Login) endpoints(ctx context.Context) (Endpoints, error) {
	if l.Endpoints.complete() {
		if err := l.Endpoints.validate(l.Issuer); err != nil {
			return Endpoints{}, err
		}
		return l.Endpoints, nil
	}
	discovered, err := discoverEndpoints(ctx, l.httpClient(), l.Issuer)
	if err != nil {
		return Endpoints{}, err
	}
	l.Endpoints = l.Endpoints.merge(discovered)
	if !l.Endpoints.complete() {
		return Endpoints{}, errors.New("issuer metadata missing required OAuth endpoints")
	}
	if err := l.Endpoints.validate(l.Issuer); err != nil {
		return Endpoints{}, err
	}
	return l.Endpoints, nil
}

// RunLoopback performs the auto-browser PKCE loopback flow.
func (l *Login) RunLoopback(ctx context.Context, opts LoopbackOptions) error {
	scope := normalizeScope(opts.Scope)
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", opts.Port))
	if err != nil {
		return fmt.Errorf("bind callback listener: %w", err)
	}
	defer func() { _ = listener.Close() }()

	port := listener.Addr().(*net.TCPAddr).Port
	authRequest, err := l.prepareAuthRequest(ctx, port, scope)
	if err != nil {
		return err
	}
	resCh, srv := startLoopbackCallback(listener)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx) //nolint:errcheck // local callback server is already no longer needed
	}()
	if err := l.openURL(authRequest.URL); err != nil {
		fmt.Printf("Open this URL to authorize: %s\n", authRequest.URL)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case r := <-resCh:
		if r.err != nil {
			return r.err
		}
		if r.state != authRequest.State {
			return fmt.Errorf("state mismatch: got %q want %q", r.state, authRequest.State)
		}
		return l.exchangeAndSave(ctx, exchangeParams{
			ClientID:    authRequest.ClientID,
			RedirectURI: authRequest.RedirectURI,
			Code:        r.code,
			Verifier:    authRequest.Verifier,
			Scope:       scope,
		})
	}
}

// RunPasteRedirect performs a PKCE flow where the user pastes the redirect URL.
func (l *Login) RunPasteRedirect(ctx context.Context, opts PasteOptions) error {
	scope := normalizeScope(opts.Scope)
	if opts.In == nil {
		return errors.New("PasteOptions.In is required")
	}
	if opts.Out == nil {
		return errors.New("PasteOptions.Out is required")
	}
	port := opts.Port
	if port == 0 {
		port = 8765
	}
	authRequest, err := l.prepareAuthRequest(ctx, port, scope)
	if err != nil {
		return err
	}
	if err := writePasteInstructions(opts.Out, authRequest.URL); err != nil {
		return err
	}
	_ = l.openURL(authRequest.URL) //nolint:errcheck // courtesy browser open; paste flow remains usable on failure

	code, err := readPastedRedirect(opts.In, authRequest.State)
	if err != nil {
		return err
	}
	return l.exchangeAndSave(ctx, exchangeParams{
		ClientID:    authRequest.ClientID,
		RedirectURI: authRequest.RedirectURI,
		Code:        code,
		Verifier:    authRequest.Verifier,
		Scope:       scope,
	})
}

type authRequest struct {
	ClientID    string
	RedirectURI string
	State       string
	URL         string
	Verifier    string
}

func (l *Login) prepareAuthRequest(ctx context.Context, port int, scope string) (*authRequest, error) {
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	endpoints, err := l.endpoints(ctx)
	if err != nil {
		return nil, err
	}
	clientID, err := l.ensureRegistered(ctx, endpoints, redirectURI, scope)
	if err != nil {
		return nil, err
	}
	pkce, err := newPKCEPair()
	if err != nil {
		return nil, err
	}
	state, err := randomState()
	if err != nil {
		return nil, err
	}
	return &authRequest{
		ClientID:    clientID,
		RedirectURI: redirectURI,
		State:       state,
		URL:         buildAuthorizeURL(endpoints.Authorization, clientID, redirectURI, scope, state, pkce.Challenge),
		Verifier:    pkce.Verifier,
	}, nil
}

// GetAccessToken returns a valid access token, refreshing first if needed.
func (l *Login) GetAccessToken(ctx context.Context) (string, error) {
	host, err := HostFromIssuer(l.Issuer)
	if err != nil {
		return "", err
	}
	creds, err := l.Store.LoadHost(host)
	if err != nil {
		return "", err
	}
	if creds.Issuer != l.Issuer {
		return "", ErrNotLoggedIn
	}
	if !creds.NeedsRefresh() {
		return creds.AccessToken, nil
	}
	if creds.RefreshToken == "" {
		return "", errors.New("access token expired and no refresh token available; log in again")
	}
	endpoints, err := l.endpoints(ctx)
	if err != nil {
		return "", err
	}
	tok, err := l.refresh(ctx, endpoints, host, creds)
	if err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

// Logout removes the stored credentials for l.Issuer's host.
func (l *Login) Logout(ctx context.Context) error {
	host, err := HostFromIssuer(l.Issuer)
	if err != nil {
		return err
	}
	creds, err := l.Store.LoadHost(host)
	if errors.Is(err, ErrNotLoggedIn) {
		return nil
	}
	if err != nil {
		return err
	}
	if creds.Issuer != l.Issuer {
		return nil
	}
	if creds.RefreshToken != "" {
		if endpoints, err := l.endpoints(ctx); err == nil && endpoints.Revocation != "" {
			_ = revokeToken(ctx, l.httpClient(), endpoints.Revocation, creds.ClientID, creds.RefreshToken) //nolint:errcheck // logout proceeds even if server-side revoke fails
		}
	}
	return l.Store.DeleteHost(host)
}

func (l *Login) ensureRegistered(ctx context.Context, endpoints Endpoints, redirectURI, scope string) (string, error) {
	host, err := HostFromIssuer(l.Issuer)
	if err != nil {
		return "", err
	}
	creds, err := l.Store.LoadHost(host)
	if err == nil && creds.ClientID != "" && creds.Issuer == l.Issuer && containsString(creds.RedirectURIs, redirectURI) {
		return creds.ClientID, nil
	}
	if err != nil && !errors.Is(err, ErrNotLoggedIn) {
		return "", err
	}
	body, err := json.Marshal(map[string]any{
		"redirect_uris":              []string{redirectURI},
		"client_name":                "mcp-kit CLI",
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
		"scope":                      scope,
	})
	if err != nil {
		return "", fmt.Errorf("marshal dcr: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoints.Registration, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build dcr request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("dcr request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("dcr failed: %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
	var dcr dcrResponse
	if err := json.NewDecoder(resp.Body).Decode(&dcr); err != nil {
		return "", fmt.Errorf("decode dcr: %w", err)
	}
	if dcr.ClientID == "" {
		return "", errors.New("dcr response missing client_id")
	}
	stored, _ := l.Store.LoadHost(host)
	stored.Issuer = l.Issuer
	stored.ClientID = dcr.ClientID
	stored.RedirectURIs = append([]string{}, dcr.RedirectURIs...)
	if err := l.Store.SaveHost(host, stored); err != nil {
		return "", err
	}
	return dcr.ClientID, nil
}

func writePasteInstructions(out io.Writer, authorizeURL string) error {
	lines := []string{
		"Open this URL in any browser:",
		"",
		"    " + authorizeURL,
		"",
		"After approving, paste the full redirect URL here:",
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(out, line); err != nil {
			return fmt.Errorf("write paste instructions: %w", err)
		}
	}
	return nil
}

type exchangeParams struct {
	ClientID    string
	RedirectURI string
	Code        string
	Verifier    string
	Scope       string
}

func (l *Login) exchangeAndSave(ctx context.Context, p exchangeParams) error {
	host, err := HostFromIssuer(l.Issuer)
	if err != nil {
		return err
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", p.Code)
	form.Set("redirect_uri", p.RedirectURI)
	form.Set("client_id", p.ClientID)
	form.Set("code_verifier", p.Verifier)

	endpoints, err := l.endpoints(ctx)
	if err != nil {
		return err
	}
	tok, err := postToken(ctx, l.httpClient(), endpoints.Token, form)
	if err != nil {
		return err
	}
	stored, _ := l.Store.LoadHost(host)
	stored.Issuer = l.Issuer
	stored.ClientID = p.ClientID
	stored.AccessToken = tok.AccessToken
	if tok.RefreshToken != "" {
		stored.RefreshToken = tok.RefreshToken
	}
	stored.Scope = p.Scope
	if tok.ExpiresIn > 0 {
		stored.ExpiresAt = time.Now().UTC().Add(time.Duration(tok.ExpiresIn) * time.Second)
	}
	return l.Store.SaveHost(host, stored)
}

func (l *Login) refresh(ctx context.Context, endpoints Endpoints, host string, creds Credentials) (*tokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", creds.RefreshToken)
	form.Set("client_id", creds.ClientID)
	tok, err := postToken(ctx, l.httpClient(), endpoints.Token, form)
	if err != nil {
		return nil, err
	}
	creds.AccessToken = tok.AccessToken
	if tok.RefreshToken != "" {
		creds.RefreshToken = tok.RefreshToken
	}
	if tok.ExpiresIn > 0 {
		creds.ExpiresAt = time.Now().UTC().Add(time.Duration(tok.ExpiresIn) * time.Second)
	}
	if err := l.Store.SaveHost(host, creds); err != nil {
		return nil, err
	}
	return tok, nil
}

func normalizeScope(scope string) string {
	if strings.TrimSpace(scope) == "" {
		return defaultScope
	}
	return scope
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
