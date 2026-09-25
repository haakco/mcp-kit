package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/haakco/mcp-kit/oauth/storage"
)

// DefaultClientMetadataCacheTTL bounds how long a fetched Client ID Metadata
// Document is reused when the document sets no HTTP cache lifetime.
const DefaultClientMetadataCacheTTL = 5 * time.Minute

// ClientIDMetadataConfig configures server-side resolution of Client ID
// Metadata Documents (SEP-991), where a client_id is an HTTPS URL whose
// document describes the client.
type ClientIDMetadataConfig struct {
	// Disabled turns the feature off, so only clients registered in the
	// consumer's store are accepted.
	Disabled bool

	// HTTPClient overrides the bounded fetch client. When set, the caller owns
	// SSRF protection, timeouts, and redirect policy.
	HTTPClient *http.Client

	// AllowLoopback permits plain-HTTP loopback client IDs for local
	// development. Never enable it in production.
	AllowLoopback bool

	// CacheTTL bounds how long a document is reused. Defaults to
	// DefaultClientMetadataCacheTTL.
	CacheTTL time.Duration
}

// supportedMetadataGrantTypes are the grant types a metadata document may
// request. Anything else would widen the client's reach beyond MCP's needs.
var supportedMetadataGrantTypes = map[string]struct{}{
	"authorization_code": {},
	"refresh_token":      {},
}

// supportedMetadataResponseTypes are the response types a metadata document may request.
var supportedMetadataResponseTypes = map[string]struct{}{
	"code": {},
}

// supportedMetadataApplicationTypes are the RFC 7591 application types.
var supportedMetadataApplicationTypes = map[string]struct{}{
	"native": {},
	"web":    {},
}

// clientMetadataDocument is the subset of RFC 7591 client metadata a Client ID
// Metadata Document may carry that affects authorization.
type clientMetadataDocument struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope"`
	ApplicationType         string   `json:"application_type"`
	LogoURI                 string   `json:"logo_uri"`
}

// ClientMetadataPolicy bounds what a fetched Client ID Metadata Document may
// claim. It mirrors the dynamic client registration policy so a document can
// never widen its own access beyond what the authorization server serves.
type ClientMetadataPolicy struct {
	// Audience is the resource the authorization server serves. Every resolved
	// client is whitelisted for it, matching registered clients.
	Audience string
	// AllowedScopes is the set of scopes the server serves.
	AllowedScopes []string
	// DefaultScopes is used when a document declares no scope.
	DefaultScopes []string
}

// ClientIDMetadataFetcher resolves clients whose client_id is an HTTPS URL by
// fetching and validating the metadata document hosted there.
//
// It implements storage.ClientMetadataResolver. It is safe for concurrent use.
type ClientIDMetadataFetcher struct {
	client        *http.Client
	allowLoopback bool
	cacheTTL      time.Duration
	audience      string
	defaultScopes []string
	allowedScopes []string
	allowed       map[string]struct{}
	now           func() time.Time

	mu    sync.Mutex
	cache map[string]cachedMetadataDocument
}

type cachedMetadataDocument struct {
	client    storage.Client
	expiresAt time.Time
}

// NewClientIDMetadataFetcher builds a fetcher bounded by policy.
func NewClientIDMetadataFetcher(cfg ClientIDMetadataConfig, policy ClientMetadataPolicy, now func() time.Time) *ClientIDMetadataFetcher {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = newClientMetadataHTTPClient(cfg.AllowLoopback)
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = DefaultClientMetadataCacheTTL
	}
	if now == nil {
		now = time.Now
	}
	return &ClientIDMetadataFetcher{
		client:        cfg.HTTPClient,
		allowLoopback: cfg.AllowLoopback,
		cacheTTL:      cfg.CacheTTL,
		audience:      policy.Audience,
		allowedScopes: append([]string{}, policy.AllowedScopes...),
		defaultScopes: append([]string{}, policy.DefaultScopes...),
		allowed:       scopeSet(policy.AllowedScopes),
		now:           now,
		cache:         map[string]cachedMetadataDocument{},
	}
}

// ResolveClient fetches and validates the metadata document for clientID.
//
// It returns storage.ErrNotAClientMetadataURL when clientID is not a URL this
// fetcher handles, so a store miss for an ordinary client stays an ordinary
// unknown-client error.
func (f *ClientIDMetadataFetcher) ResolveClient(ctx context.Context, clientID string) (storage.Client, error) {
	if _, ok := clientMetadataURLString(clientID, f.allowLoopback); !ok {
		return storage.Client{}, storage.ErrNotAClientMetadataURL
	}

	if client, ok := f.cached(clientID); ok {
		return client, nil
	}

	document, cacheTTL, err := f.fetch(ctx, clientID)
	if err != nil {
		return storage.Client{}, err
	}
	client, err := f.clientFromDocument(clientID, document)
	if err != nil {
		return storage.Client{}, err
	}
	f.store(clientID, client, cacheTTL)
	return client, nil
}

// clientMetadataURLString parses rawURL and reports whether it is an acceptable
// client ID metadata URL.
func clientMetadataURLString(rawURL string, allowLoopback bool) (*url.URL, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, false
	}
	return clientMetadataURL(parsed, allowLoopback)
}

// clientMetadataURL reports whether parsed is an acceptable client ID metadata
// URL.
//
// Credentials and fragments are rejected outright: both make the identifier
// ambiguous, and a fragment never reaches the server so the document could not
// prove ownership of it. A root path is rejected because the identifier must
// name a document, not a whole origin.
func clientMetadataURL(parsed *url.URL, allowLoopback bool) (*url.URL, bool) {
	if parsed == nil || parsed.Host == "" {
		return nil, false
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return nil, false
	}
	if parsed.Path == "" || parsed.Path == "/" {
		return nil, false
	}
	switch parsed.Scheme {
	case "https":
		return parsed, true
	case "http":
		if allowLoopback && isLoopbackHost(parsed.Hostname()) {
			return parsed, true
		}
		return nil, false
	default:
		return nil, false
	}
}

func (f *ClientIDMetadataFetcher) fetch(ctx context.Context, clientID string) (clientMetadataDocument, time.Duration, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, clientID, nil)
	if err != nil {
		return clientMetadataDocument{}, 0, fmt.Errorf("client metadata: build request: %w", err)
	}
	request.Header.Set("Accept", "application/json")

	response, err := f.client.Do(request)
	if err != nil {
		return clientMetadataDocument{}, 0, fmt.Errorf("client metadata: fetch %s: %w", clientID, err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return clientMetadataDocument{}, 0, fmt.Errorf("client metadata: %s returned status %d", clientID, response.StatusCode)
	}

	limited := http.MaxBytesReader(nil, response.Body, clientMetadataMaxBytes)
	var document clientMetadataDocument
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return clientMetadataDocument{}, 0, fmt.Errorf("client metadata: decode %s: %w", clientID, err)
	}

	return document, cacheLifetime(response.Header, f.cacheTTL, f.now()), nil
}

func (f *ClientIDMetadataFetcher) clientFromDocument(clientID string, document clientMetadataDocument) (storage.Client, error) {
	// The document must claim exactly the URL it was served from. Without this
	// check one host could serve metadata for another's client_id.
	if document.ClientID != clientID {
		return storage.Client{}, fmt.Errorf("client metadata: document client_id %q does not match %q", document.ClientID, clientID)
	}
	if len(document.RedirectURIs) == 0 {
		return storage.Client{}, errors.New("client metadata: redirect_uris is required")
	}
	for _, uri := range document.RedirectURIs {
		if err := validateRedirectURI(uri); err != nil {
			return storage.Client{}, fmt.Errorf("client metadata: %w", err)
		}
	}
	if err := validateMetadataGrantTypes(document.GrantTypes); err != nil {
		return storage.Client{}, err
	}
	if err := validateMetadataResponseTypes(document.ResponseTypes); err != nil {
		return storage.Client{}, err
	}
	if err := validateMetadataApplicationType(document.ApplicationType); err != nil {
		return storage.Client{}, err
	}
	if err := validateMetadataAuthMethod(document.TokenEndpointAuthMethod); err != nil {
		return storage.Client{}, err
	}
	if err := validateLogoURI(document.LogoURI); err != nil {
		return storage.Client{}, fmt.Errorf("client metadata: %w", err)
	}

	// Scope claims are intersected with what the server serves, so a document
	// cannot widen its own access.
	scopes, _, err := normalizeScopesWithDefault(document.Scope, f.defaultScopes, f.allowedScopes, f.allowed)
	if err != nil {
		return storage.Client{}, fmt.Errorf("client metadata: %w", err)
	}

	name := strings.TrimSpace(document.ClientName)
	if len(name) > maxClientNameLen {
		name = name[:maxClientNameLen]
	}
	if name == "" {
		name = defaultClientName
	}

	grantTypes := document.GrantTypes
	if len(grantTypes) == 0 {
		grantTypes = []string{"authorization_code", "refresh_token"}
	}
	responseTypes := document.ResponseTypes
	if len(responseTypes) == 0 {
		responseTypes = []string{"code"}
	}

	return storage.Client{
		ID:              clientID,
		Name:            name,
		RedirectURIs:    append([]string{}, document.RedirectURIs...),
		GrantTypes:      grantTypes,
		ResponseTypes:   responseTypes,
		Scopes:          scopes,
		Audience:        f.audienceArguments(),
		IsPublic:        true,
		TokenAuthMethod: authMethodNone,
		LogoURI:         document.LogoURI,
	}, nil
}

func (f *ClientIDMetadataFetcher) audienceArguments() []string {
	if f.audience == "" {
		return nil
	}
	return []string{f.audience}
}

func validateMetadataGrantTypes(grantTypes []string) error {
	for _, grantType := range grantTypes {
		if _, ok := supportedMetadataGrantTypes[grantType]; !ok {
			return fmt.Errorf("client metadata: grant type %q is not supported", grantType)
		}
	}
	return nil
}

func validateMetadataResponseTypes(responseTypes []string) error {
	for _, responseType := range responseTypes {
		if _, ok := supportedMetadataResponseTypes[responseType]; !ok {
			return fmt.Errorf("client metadata: response type %q is not supported", responseType)
		}
	}
	return nil
}

// validateMetadataApplicationType checks application_type only when supplied.
// It is optional metadata, so rejecting its absence would break clients that
// legitimately omit it.
func validateMetadataApplicationType(applicationType string) error {
	if applicationType == "" {
		return nil
	}
	if _, ok := supportedMetadataApplicationTypes[applicationType]; !ok {
		return fmt.Errorf("client metadata: application_type %q is not supported", applicationType)
	}
	return nil
}

// validateMetadataAuthMethod rejects confidential-client methods. A metadata
// document is public by construction: anyone can fetch it, so it cannot carry a
// secret.
func validateMetadataAuthMethod(method string) error {
	switch method {
	case "", authMethodNone:
		return nil
	default:
		return fmt.Errorf("client metadata: token_endpoint_auth_method %q requires a client secret, which a public document cannot hold", method)
	}
}

// cacheLifetime honours the response's HTTP cache lifetime when present.
func cacheLifetime(header http.Header, fallback time.Duration, now time.Time) time.Duration {
	cacheControl := strings.ToLower(header.Get("Cache-Control"))
	if strings.Contains(cacheControl, "no-store") || strings.Contains(cacheControl, "no-cache") {
		return 0
	}
	for directive := range strings.SplitSeq(cacheControl, ",") {
		value, ok := strings.CutPrefix(strings.TrimSpace(directive), "max-age=")
		if !ok {
			continue
		}
		seconds, err := time.ParseDuration(value + "s")
		if err != nil {
			return fallback
		}
		if seconds < 0 {
			return 0
		}
		return seconds
	}
	if expires, err := http.ParseTime(header.Get("Expires")); err == nil {
		if lifetime := expires.Sub(now); lifetime > 0 {
			return lifetime
		}
		return 0
	}
	return fallback
}

func (f *ClientIDMetadataFetcher) cached(clientID string) (storage.Client, bool) {
	if f.cacheTTL <= 0 {
		return storage.Client{}, false
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	entry, ok := f.cache[clientID]
	if !ok {
		return storage.Client{}, false
	}
	if !entry.expiresAt.After(f.now()) {
		delete(f.cache, clientID)
		return storage.Client{}, false
	}
	return entry.client, true
}

func (f *ClientIDMetadataFetcher) store(clientID string, client storage.Client, lifetime time.Duration) {
	if lifetime <= 0 {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.cache) >= clientMetadataMaxCacheItems {
		f.evictExpiredLocked()
	}
	if len(f.cache) >= clientMetadataMaxCacheItems {
		for key := range f.cache {
			delete(f.cache, key)
			break
		}
	}
	f.cache[clientID] = cachedMetadataDocument{client: client, expiresAt: f.now().Add(lifetime)}
}

func (f *ClientIDMetadataFetcher) evictExpiredLocked() {
	now := f.now()
	for key, entry := range f.cache {
		if !entry.expiresAt.After(now) {
			delete(f.cache, key)
		}
	}
}
