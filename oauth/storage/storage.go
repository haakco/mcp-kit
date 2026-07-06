// Package storage adapts consumer persistence to Fosite's OAuth storage interfaces.
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/handler/oauth2"
	"github.com/ory/fosite/handler/openid"
	"github.com/ory/fosite/handler/pkce"
	fositestorage "github.com/ory/fosite/storage"
)

const (
	SessionTypeAuthorizationCode = "authorization_code"
	SessionTypeAccessToken       = "access_token"
	SessionTypeRefreshToken      = "refresh_token"
	SessionTypePKCE              = "pkce"
	SessionTypeOIDC              = "oidc"
)

var (
	_ fosite.Storage                     = (*Storage)(nil)
	_ oauth2.CoreStorage                 = (*Storage)(nil)
	_ pkce.PKCERequestStorage            = (*Storage)(nil)
	_ openid.OpenIDConnectRequestStorage = (*Storage)(nil)
	_ fositestorage.Transactional        = (*Storage)(nil)
)

// ErrNotFound indicates the requested client or session row does not exist.
var ErrNotFound = errors.New("oauth storage: not found")

// Client is the persisted OAuth client shape required by Fosite.
type Client struct {
	ID               string
	Name             string
	RedirectURIs     []string
	GrantTypes       []string
	ResponseTypes    []string
	Scopes           []string
	Audience         []string
	IsPublic         bool
	ClientSecretHash string
	TokenAuthMethod  string
	LogoURI          string
}

// Session stores the serializable Fosite request state.
type Session struct {
	Type      string
	Signature string
	ClientID  string
	RequestID string
	Scopes    []string
	Data      map[string]any
	Active    bool
	ExpiresAt *time.Time
}

// Store persists OAuth clients and sessions. Implementations must be safe for
// concurrent use.
type Store interface {
	GetClient(ctx context.Context, clientID string) (Client, error)
	SaveClient(ctx context.Context, client Client) error
	SaveSession(ctx context.Context, session Session) error
	GetSession(ctx context.Context, sessionType string, signature string) (Session, error)
	SetSessionActive(ctx context.Context, sessionType string, signature string, active bool) error
	DeleteSession(ctx context.Context, sessionType string, signature string) error
	DeleteSessionsByRequestID(ctx context.Context, sessionType string, requestID string) error
}

// TransactionalStore is the optional transaction interface used by Fosite
// during flows that require atomic storage semantics.
type TransactionalStore interface {
	BeginTX(ctx context.Context) (context.Context, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// Storage implements Fosite storage interfaces.
type Storage struct {
	store Store
}

// New creates a Fosite storage adapter.
func New(store Store) *Storage {
	return &Storage{store: store}
}

// GetClient loads an OAuth client by client_id.
func (s *Storage) GetClient(ctx context.Context, id string) (fosite.Client, error) {
	client, err := s.store.GetClient(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fosite.ErrNotFound
		}
		return nil, fmt.Errorf("query client: %w", err)
	}
	return clientWrapper{client: client}, nil
}

// ClientAssertionJWTValid fails closed because JWT client authentication is
// not supported by the kit yet. Returning success here would disable JTI
// replay protection if private_key_jwt were enabled later.
func (*Storage) ClientAssertionJWTValid(context.Context, string) error {
	return fosite.ErrJTIKnown
}

// SetClientAssertionJWT fails closed because JWT client authentication is not supported yet.
func (*Storage) SetClientAssertionJWT(context.Context, string, time.Time) error {
	return fosite.ErrInvalidClient
}

// BeginTX starts a transaction when the underlying store supports it.
func (s *Storage) BeginTX(ctx context.Context) (context.Context, error) {
	if txStore, ok := s.store.(TransactionalStore); ok {
		return txStore.BeginTX(ctx)
	}
	return ctx, nil
}

// Commit commits a transaction when the underlying store supports it.
func (s *Storage) Commit(ctx context.Context) error {
	if txStore, ok := s.store.(TransactionalStore); ok {
		return txStore.Commit(ctx)
	}
	return nil
}

// Rollback rolls back a transaction when the underlying store supports it.
func (s *Storage) Rollback(ctx context.Context) error {
	if txStore, ok := s.store.(TransactionalStore); ok {
		return txStore.Rollback(ctx)
	}
	return nil
}

func (s *Storage) CreateAuthorizeCodeSession(ctx context.Context, code string, request fosite.Requester) error {
	return s.createSession(ctx, SessionTypeAuthorizationCode, code, request)
}

func (s *Storage) GetAuthorizeCodeSession(ctx context.Context, code string, session fosite.Session) (fosite.Requester, error) {
	return s.getSession(ctx, SessionTypeAuthorizationCode, code, session)
}

func (s *Storage) InvalidateAuthorizeCodeSession(ctx context.Context, code string) error {
	if err := s.store.SetSessionActive(ctx, SessionTypeAuthorizationCode, code, false); err != nil {
		return fmt.Errorf("invalidate authorize code: %w", err)
	}
	return nil
}

func (s *Storage) CreateAccessTokenSession(ctx context.Context, signature string, request fosite.Requester) error {
	return s.createSession(ctx, SessionTypeAccessToken, signature, request)
}

func (s *Storage) GetAccessTokenSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	return s.getSession(ctx, SessionTypeAccessToken, signature, session)
}

func (s *Storage) DeleteAccessTokenSession(ctx context.Context, signature string) error {
	return s.deleteSession(ctx, SessionTypeAccessToken, signature)
}

func (s *Storage) CreateRefreshTokenSession(ctx context.Context, signature string, _ string, request fosite.Requester) error {
	return s.createSession(ctx, SessionTypeRefreshToken, signature, request)
}

func (s *Storage) GetRefreshTokenSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	return s.getSession(ctx, SessionTypeRefreshToken, signature, session)
}

func (s *Storage) DeleteRefreshTokenSession(ctx context.Context, signature string) error {
	return s.deleteSession(ctx, SessionTypeRefreshToken, signature)
}

// RotateRefreshToken invalidates prior access and refresh tokens for requestID.
func (s *Storage) RotateRefreshToken(ctx context.Context, requestID string, _ string) error {
	if err := s.store.DeleteSessionsByRequestID(ctx, SessionTypeAccessToken, requestID); err != nil {
		return fmt.Errorf("rotate: delete access tokens: %w", err)
	}
	if err := s.store.DeleteSessionsByRequestID(ctx, SessionTypeRefreshToken, requestID); err != nil {
		return fmt.Errorf("rotate: delete refresh tokens: %w", err)
	}
	return nil
}

func (s *Storage) RevokeAccessToken(ctx context.Context, requestID string) error {
	return s.store.DeleteSessionsByRequestID(ctx, SessionTypeAccessToken, requestID)
}

func (s *Storage) RevokeRefreshToken(ctx context.Context, requestID string) error {
	return s.store.DeleteSessionsByRequestID(ctx, SessionTypeRefreshToken, requestID)
}

func (s *Storage) RevokeRefreshTokenMaybeGracePeriod(ctx context.Context, requestID string, _ string) error {
	return s.RevokeRefreshToken(ctx, requestID)
}

func (s *Storage) CreatePKCERequestSession(ctx context.Context, signature string, requester fosite.Requester) error {
	return s.createSession(ctx, SessionTypePKCE, signature, requester)
}

func (s *Storage) GetPKCERequestSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	return s.getSession(ctx, SessionTypePKCE, signature, session)
}

func (s *Storage) DeletePKCERequestSession(ctx context.Context, signature string) error {
	return s.deleteSession(ctx, SessionTypePKCE, signature)
}

func (s *Storage) CreateOpenIDConnectSession(ctx context.Context, authorizeCode string, requester fosite.Requester) error {
	return s.createSession(ctx, SessionTypeOIDC, authorizeCode, requester)
}

func (s *Storage) GetOpenIDConnectSession(
	ctx context.Context, authorizeCode string, requester fosite.Requester,
) (fosite.Requester, error) {
	return s.getSession(ctx, SessionTypeOIDC, authorizeCode, requester.GetSession())
}

func (s *Storage) DeleteOpenIDConnectSession(ctx context.Context, authorizeCode string) error {
	return s.deleteSession(ctx, SessionTypeOIDC, authorizeCode)
}

func (s *Storage) createSession(ctx context.Context, sessionType string, signature string, requester fosite.Requester) error {
	data, err := marshalRequestData(requester)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	session := Session{
		Type:      sessionType,
		Signature: signature,
		ClientID:  requester.GetClient().GetID(),
		RequestID: requester.GetID(),
		Scopes:    append([]string{}, requester.GetGrantedScopes()...),
		Data:      data,
		Active:    true,
	}
	if expiresAt := sessionExpiresAt(sessionType, requester.GetSession()); !expiresAt.IsZero() {
		session.ExpiresAt = &expiresAt
	}
	return s.store.SaveSession(ctx, session)
}

func sessionExpiresAt(sessionType string, session fosite.Session) time.Time {
	if session == nil {
		return time.Time{}
	}
	expiresAt := session.GetExpiresAt(sessionTokenType(sessionType))
	if !expiresAt.IsZero() {
		return expiresAt
	}
	return session.GetExpiresAt(fosite.AccessToken)
}

func sessionTokenType(sessionType string) fosite.TokenType {
	switch sessionType {
	case SessionTypeAuthorizationCode, SessionTypePKCE, SessionTypeOIDC:
		return fosite.AuthorizeCode
	case SessionTypeRefreshToken:
		return fosite.RefreshToken
	default:
		return fosite.AccessToken
	}
}

func (s *Storage) getSession(ctx context.Context, sessionType string, signature string, session fosite.Session) (fosite.Requester, error) {
	row, err := s.store.GetSession(ctx, sessionType, signature)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fosite.ErrNotFound
		}
		return nil, fmt.Errorf("query session: %w", err)
	}

	requester, err := s.rowToRequester(ctx, row, session)
	if err != nil {
		return nil, err
	}

	if !row.Active {
		if sessionType == SessionTypeAuthorizationCode {
			return requester, fosite.ErrInvalidatedAuthorizeCode
		}
		return nil, fosite.ErrNotFound
	}
	return requester, nil
}

func (s *Storage) deleteSession(ctx context.Context, sessionType string, signature string) error {
	if err := s.store.DeleteSession(ctx, sessionType, signature); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s *Storage) rowToRequester(ctx context.Context, row Session, session fosite.Session) (*fosite.Request, error) {
	stored, err := unmarshalStoredRequestData(row.Data)
	if err != nil {
		return nil, err
	}
	if session != nil && stored.Session != nil {
		data, err := json.Marshal(stored.Session)
		if err != nil {
			return nil, fmt.Errorf("re-marshal session data: %w", err)
		}
		if err := json.Unmarshal(data, session); err != nil {
			return nil, fmt.Errorf("unmarshal session data: %w", err)
		}
	}

	client, err := s.GetClient(ctx, row.ClientID)
	if err != nil {
		return nil, fmt.Errorf("lookup client for session: %w", err)
	}

	grantedScopes := fosite.Arguments(row.Scopes)
	if len(stored.GrantedScopes) > 0 {
		grantedScopes = append(fosite.Arguments{}, stored.GrantedScopes...)
	}
	requestedScopes := append(fosite.Arguments{}, grantedScopes...)
	if len(stored.RequestedScopes) > 0 {
		requestedScopes = append(fosite.Arguments{}, stored.RequestedScopes...)
	}

	requestedAt := stored.RequestedAt
	if requestedAt.IsZero() {
		requestedAt = time.Now().UTC()
	}
	return &fosite.Request{
		ID:                row.RequestID,
		RequestedAt:       requestedAt,
		Client:            client,
		Session:           session,
		GrantedScope:      grantedScopes,
		RequestedScope:    requestedScopes,
		RequestedAudience: append(fosite.Arguments{}, stored.RequestedAudience...),
		GrantedAudience:   append(fosite.Arguments{}, stored.GrantedAudience...),
		Form:              cloneValues(stored.Form),
	}, nil
}

type clientWrapper struct {
	client Client
}

func (c clientWrapper) GetID() string { return c.client.ID }
func (c clientWrapper) GetHashedSecret() []byte {
	return []byte(c.client.ClientSecretHash)
}
func (c clientWrapper) GetRedirectURIs() []string {
	return append([]string{}, c.client.RedirectURIs...)
}
func (c clientWrapper) GetGrantTypes() fosite.Arguments {
	return append(fosite.Arguments{}, c.client.GrantTypes...)
}
func (c clientWrapper) GetResponseTypes() fosite.Arguments {
	return append(fosite.Arguments{}, c.client.ResponseTypes...)
}
func (c clientWrapper) GetScopes() fosite.Arguments {
	return append(fosite.Arguments{}, c.client.Scopes...)
}
func (c clientWrapper) GetAudience() fosite.Arguments {
	return append(fosite.Arguments{}, c.client.Audience...)
}
func (c clientWrapper) IsPublic() bool { return c.client.IsPublic }

type storedRequestData struct {
	Session           map[string]any      `json:"session,omitempty"`
	Form              map[string][]string `json:"form,omitempty"`
	RequestedAt       time.Time           `json:"requestedAt,omitempty"`
	RequestedScopes   []string            `json:"requestedScopes,omitempty"`
	GrantedScopes     []string            `json:"grantedScopes,omitempty"`
	RequestedAudience []string            `json:"requestedAudience,omitempty"`
	GrantedAudience   []string            `json:"grantedAudience,omitempty"`
}

func marshalRequestData(requester fosite.Requester) (map[string]any, error) {
	stored := storedRequestData{
		RequestedAt:       requester.GetRequestedAt(),
		RequestedScopes:   append([]string{}, requester.GetRequestedScopes()...),
		GrantedScopes:     append([]string{}, requester.GetGrantedScopes()...),
		RequestedAudience: append([]string{}, requester.GetRequestedAudience()...),
		GrantedAudience:   append([]string{}, requester.GetGrantedAudience()...),
		Form:              map[string][]string{},
	}
	if stored.RequestedAt.IsZero() {
		stored.RequestedAt = time.Now().UTC()
	}
	for key, values := range sanitizeRequestForm(requester.GetRequestForm()) {
		stored.Form[key] = append([]string{}, values...)
	}
	if requester.GetSession() != nil {
		session, err := marshalJSONMap(requester.GetSession())
		if err != nil {
			return nil, err
		}
		stored.Session = session
	}
	return marshalJSONMap(stored)
}

func unmarshalStoredRequestData(data map[string]any) (*storedRequestData, error) {
	if len(data) == 0 {
		return &storedRequestData{Form: map[string][]string{}}, nil
	}
	if _, ok := data["session"]; !ok {
		return &storedRequestData{Session: data, Form: map[string][]string{}}, nil
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var stored storedRequestData
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, fmt.Errorf("unmarshal stored request data: %w", err)
	}
	if stored.Form == nil {
		stored.Form = map[string][]string{}
	}
	return &stored, nil
}

func marshalJSONMap(value any) (map[string]any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func cloneValues(values map[string][]string) url.Values {
	cloned := url.Values{}
	for key, value := range values {
		cloned[key] = append([]string{}, value...)
	}
	return cloned
}

func sanitizeRequestForm(values url.Values) url.Values {
	sensitive := map[string]struct{}{
		"client_secret": {},
		"code_verifier": {},
		"password":      {},
		"refresh_token": {},
		"token":         {},
	}
	clean := url.Values{}
	for key, value := range values {
		if _, ok := sensitive[key]; ok {
			continue
		}
		clean[key] = append([]string{}, value...)
	}
	return clean
}

// ScopesFromString converts a space-separated scope string to a slice.
func ScopesFromString(scopes string) []string {
	return strings.Fields(scopes)
}
