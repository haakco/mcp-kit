package storage_test

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/handler/oauth2"
	oidcsession "github.com/ory/fosite/handler/openid"
	"github.com/ory/fosite/handler/pkce"
	fositestorage "github.com/ory/fosite/storage"

	"github.com/haakco/mcp-kit/oauth/storage"
)

func TestStorageInterfaceCompliance(t *testing.T) {
	var _ fosite.Storage = storage.New(storage.NewMemoryStore())
	var _ oauth2.CoreStorage = storage.New(storage.NewMemoryStore())
	var _ pkce.PKCERequestStorage = storage.New(storage.NewMemoryStore())
	var _ oidcsession.OpenIDConnectRequestStorage = storage.New(storage.NewMemoryStore())
	var _ fositestorage.Transactional = storage.New(storage.NewMemoryStore())
}

func TestStorageClientAssertionJWTFailsClosed(t *testing.T) {
	store := storage.New(storage.NewMemoryStore())

	if err := store.ClientAssertionJWTValid(t.Context(), "jti"); !errors.Is(err, fosite.ErrJTIKnown) {
		t.Fatalf("ClientAssertionJWTValid() error = %v, want ErrJTIKnown", err)
	}
	if err := store.SetClientAssertionJWT(t.Context(), "jti", time.Now()); !errors.Is(err, fosite.ErrInvalidClient) {
		t.Fatalf("SetClientAssertionJWT() error = %v, want ErrInvalidClient", err)
	}
}

func TestMemoryStoreClientLogoURIRoundtrip(t *testing.T) {
	memory := storage.NewMemoryStore()

	if err := memory.SaveClient(t.Context(), storage.Client{
		ID:       "logo-client",
		Name:     "Logo Client",
		LogoURI:  "https://assets.example.test/logo.png",
		IsPublic: true,
	}); err != nil {
		t.Fatalf("SaveClient() error = %v", err)
	}

	client, err := memory.GetClient(t.Context(), "logo-client")
	if err != nil {
		t.Fatalf("GetClient() error = %v", err)
	}
	if client.LogoURI != "https://assets.example.test/logo.png" {
		t.Fatalf("LogoURI = %q, want saved logo", client.LogoURI)
	}
}

func TestStorageAuthCodeRoundtrip(t *testing.T) {
	store := newTestStorage(t)
	requester := newRequester("auth-request", "auth-subject", "openid", "skills.read")

	if err := store.CreateAuthorizeCodeSession(t.Context(), "auth-code", requester); err != nil {
		t.Fatalf("CreateAuthorizeCodeSession() error = %v", err)
	}

	session := &oidcsession.DefaultSession{}
	loaded, err := store.GetAuthorizeCodeSession(t.Context(), "auth-code", session)
	if err != nil {
		t.Fatalf("GetAuthorizeCodeSession() error = %v", err)
	}
	assertRequesterRoundtrip(t, loaded, session, requester)

	if err := store.InvalidateAuthorizeCodeSession(t.Context(), "auth-code"); err != nil {
		t.Fatalf("InvalidateAuthorizeCodeSession() error = %v", err)
	}
	if _, err := store.GetAuthorizeCodeSession(t.Context(), "auth-code", &oidcsession.DefaultSession{}); !errors.Is(err, fosite.ErrInvalidatedAuthorizeCode) {
		t.Fatalf("GetAuthorizeCodeSession() after invalidate error = %v, want ErrInvalidatedAuthorizeCode", err)
	}
}

func TestStorageAccessTokenRoundtrip(t *testing.T) {
	store := newTestStorage(t)
	requester := newRequester("access-request", "access-subject", "skills.read")

	if err := store.CreateAccessTokenSession(t.Context(), "access-signature", requester); err != nil {
		t.Fatalf("CreateAccessTokenSession() error = %v", err)
	}

	session := &oidcsession.DefaultSession{}
	loaded, err := store.GetAccessTokenSession(t.Context(), "access-signature", session)
	if err != nil {
		t.Fatalf("GetAccessTokenSession() error = %v", err)
	}
	assertRequesterRoundtrip(t, loaded, session, requester)

	if err := store.DeleteAccessTokenSession(t.Context(), "access-signature"); err != nil {
		t.Fatalf("DeleteAccessTokenSession() error = %v", err)
	}
	if _, err := store.GetAccessTokenSession(t.Context(), "access-signature", &oidcsession.DefaultSession{}); !errors.Is(err, fosite.ErrNotFound) {
		t.Fatalf("GetAccessTokenSession() after delete error = %v, want ErrNotFound", err)
	}
}

func TestStorageRefreshTokenRotation(t *testing.T) {
	store := newTestStorage(t)
	requester := newRequester("refresh-request", "refresh-subject", "openid", "skills.read")

	if err := store.CreateAccessTokenSession(t.Context(), "access-old", requester); err != nil {
		t.Fatalf("CreateAccessTokenSession() error = %v", err)
	}
	if err := store.CreateRefreshTokenSession(t.Context(), "refresh-old", "", requester); err != nil {
		t.Fatalf("CreateRefreshTokenSession() error = %v", err)
	}

	if err := store.RotateRefreshToken(t.Context(), requester.GetID(), "refresh-old"); err != nil {
		t.Fatalf("RotateRefreshToken() error = %v", err)
	}

	if _, err := store.GetAccessTokenSession(t.Context(), "access-old", &oidcsession.DefaultSession{}); !errors.Is(err, fosite.ErrNotFound) {
		t.Fatalf("old access token error = %v, want ErrNotFound", err)
	}
	if _, err := store.GetRefreshTokenSession(t.Context(), "refresh-old", &oidcsession.DefaultSession{}); !errors.Is(err, fosite.ErrNotFound) {
		t.Fatalf("old refresh token error = %v, want ErrNotFound", err)
	}

	if err := store.CreateRefreshTokenSession(t.Context(), "refresh-new", "", requester); err != nil {
		t.Fatalf("CreateRefreshTokenSession() replacement error = %v", err)
	}
	if _, err := store.GetRefreshTokenSession(t.Context(), "refresh-new", &oidcsession.DefaultSession{}); err != nil {
		t.Fatalf("replacement refresh lookup error = %v", err)
	}
}

func TestStoragePersistsTokenTypeSpecificExpiry(t *testing.T) {
	memory := newTestMemoryStore(t)
	store := storage.New(memory)
	accessExpiry := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	refreshExpiry := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	requester := newRequesterWithExpiries("expiry-request", "expiry-subject", accessExpiry, refreshExpiry, "skills.read")

	if err := store.CreateAccessTokenSession(t.Context(), "access-expiry", requester); err != nil {
		t.Fatalf("CreateAccessTokenSession() error = %v", err)
	}
	if err := store.CreateRefreshTokenSession(t.Context(), "refresh-expiry", "", requester); err != nil {
		t.Fatalf("CreateRefreshTokenSession() error = %v", err)
	}

	accessRow, err := memory.GetSession(t.Context(), storage.SessionTypeAccessToken, "access-expiry")
	if err != nil {
		t.Fatalf("GetSession(access) error = %v", err)
	}
	refreshRow, err := memory.GetSession(t.Context(), storage.SessionTypeRefreshToken, "refresh-expiry")
	if err != nil {
		t.Fatalf("GetSession(refresh) error = %v", err)
	}
	assertSessionExpiry(t, accessRow, accessExpiry)
	assertSessionExpiry(t, refreshRow, refreshExpiry)
}

func newTestStorage(t *testing.T) *storage.Storage {
	return storage.New(newTestMemoryStore(t))
}

func newTestMemoryStore(t *testing.T) *storage.MemoryStore {
	t.Helper()

	memory := storage.NewMemoryStore()
	if err := memory.SaveClient(t.Context(), storage.Client{
		ID:            "client-id",
		Name:          "Test Client",
		RedirectURIs:  []string{"http://127.0.0.1/callback"},
		GrantTypes:    []string{"authorization_code", "refresh_token"},
		ResponseTypes: []string{"code"},
		Scopes:        []string{"openid", "skills.read", "skills.write"},
		IsPublic:      true,
	}); err != nil {
		t.Fatalf("SaveClient() error = %v", err)
	}

	return memory
}

func newRequester(id string, subject string, scopes ...string) fosite.Requester {
	return newRequesterWithExpiries(
		id,
		subject,
		time.Now().UTC().Add(time.Hour).Truncate(time.Second),
		time.Time{},
		scopes...,
	)
}

func newRequesterWithExpiries(
	id string,
	subject string,
	accessExpiry time.Time,
	refreshExpiry time.Time,
	scopes ...string,
) fosite.Requester {
	expiresAt := map[fosite.TokenType]time.Time{
		fosite.AccessToken: accessExpiry,
	}
	if !refreshExpiry.IsZero() {
		expiresAt[fosite.RefreshToken] = refreshExpiry
	}
	session := &oidcsession.DefaultSession{
		Subject:   subject,
		Username:  "roundtrip@example.com",
		ExpiresAt: expiresAt,
	}
	return &fosite.Request{
		ID:                id,
		Client:            &testClient{id: "client-id"},
		Session:           session,
		RequestedScope:    fosite.Arguments(scopes),
		GrantedScope:      fosite.Arguments(scopes),
		RequestedAudience: fosite.Arguments{"skills-api"},
		GrantedAudience:   fosite.Arguments{"skills-api"},
		Form: url.Values{
			"code_challenge":        {"roundtrip-challenge"},
			"code_challenge_method": {"S256"},
			"redirect_uri":          {"http://127.0.0.1/callback"},
		},
	}
}

func assertSessionExpiry(t *testing.T, row storage.Session, want time.Time) {
	t.Helper()

	if row.ExpiresAt == nil {
		t.Fatalf("%s ExpiresAt = nil, want %s", row.Type, want.Format(time.RFC3339))
	}
	if !row.ExpiresAt.Equal(want) {
		t.Fatalf("%s ExpiresAt = %s, want %s", row.Type, row.ExpiresAt.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func assertRequesterRoundtrip(t *testing.T, loaded fosite.Requester, session *oidcsession.DefaultSession, expected fosite.Requester) {
	t.Helper()

	if loaded.GetID() != expected.GetID() {
		t.Fatalf("request ID = %q, want %q", loaded.GetID(), expected.GetID())
	}
	if loaded.GetClient().GetID() != expected.GetClient().GetID() {
		t.Fatalf("client ID = %q, want %q", loaded.GetClient().GetID(), expected.GetClient().GetID())
	}
	if !loaded.GetGrantedScopes().Has("skills.read") {
		t.Fatalf("granted scopes = %v, want skills.read", loaded.GetGrantedScopes())
	}
	if loaded.GetRequestForm().Get("code_challenge") != "roundtrip-challenge" {
		t.Fatalf("code_challenge = %q, want roundtrip-challenge", loaded.GetRequestForm().Get("code_challenge"))
	}
	if !loaded.GetRequestedAudience().Has("skills-api") || !loaded.GetGrantedAudience().Has("skills-api") {
		t.Fatalf("audience roundtrip failed: requested=%v granted=%v", loaded.GetRequestedAudience(), loaded.GetGrantedAudience())
	}
	if session.GetSubject() != expected.GetSession().GetSubject() {
		t.Fatalf("subject = %q, want %q", session.GetSubject(), expected.GetSession().GetSubject())
	}
	if session.Username != "roundtrip@example.com" {
		t.Fatalf("username = %q, want roundtrip@example.com", session.Username)
	}
}

type testClient struct {
	id string
}

func (c *testClient) GetID() string             { return c.id }
func (c *testClient) GetHashedSecret() []byte   { return nil }
func (c *testClient) GetRedirectURIs() []string { return []string{"http://127.0.0.1/callback"} }
func (c *testClient) GetGrantTypes() fosite.Arguments {
	return fosite.Arguments{"authorization_code", "refresh_token"}
}
func (c *testClient) GetResponseTypes() fosite.Arguments { return fosite.Arguments{"code"} }
func (c *testClient) GetScopes() fosite.Arguments {
	return fosite.Arguments{"openid", "skills.read", "skills.write"}
}
func (c *testClient) GetAudience() fosite.Arguments { return fosite.Arguments{"skills-api"} }
func (c *testClient) IsPublic() bool                { return true }
