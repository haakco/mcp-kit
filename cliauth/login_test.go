package cliauth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/haakco/mcp-kit/cliauth"
)

var osStat = os.Stat

func mustHost(t *testing.T, issuer string) string {
	t.Helper()
	host, err := cliauth.HostFromIssuer(issuer)
	if err != nil {
		t.Fatalf("HostFromIssuer(%q): %v", issuer, err)
	}
	return host
}

type fakeAuthServer struct {
	server      *httptest.Server
	mu          sync.Mutex
	clientID    string
	scope       string
	redirectURI string
	codeIssued  string
	state       string
	pkceChall   string
	accessToken string
	refresh     string
	exchanged   bool
	refreshHits int
	dcrCalls    int
}

func newFakeAuthServer(t *testing.T) *fakeAuthServer {
	t.Helper()
	f := &fakeAuthServer{
		clientID:    "fake-client-id",
		codeIssued:  "auth-code-xyz",
		accessToken: "access-token-1",
		refresh:     "refresh-token-1",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"authorization_endpoint": f.server.URL + "/oauth/authorize",
			"token_endpoint":         f.server.URL + "/oauth/token",
			"revocation_endpoint":    f.server.URL + "/oauth/revoke",
			"registration_endpoint":  f.server.URL + "/oauth/register",
		}); err != nil {
			t.Errorf("encode metadata response: %v", err)
		}
	})
	mux.HandleFunc("/oauth/register", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.dcrCalls++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if uris, _ := body["redirect_uris"].([]any); len(uris) > 0 {
			f.redirectURI, _ = uris[0].(string)
		}
		if v, _ := body["scope"].(string); v != "" {
			f.scope = v
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"client_id":     f.clientID,
			"redirect_uris": []string{f.redirectURI},
		}); err != nil {
			t.Errorf("encode dcr response: %v", err)
		}
	})
	mux.HandleFunc("/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.state = r.URL.Query().Get("state")
		f.pkceChall = r.URL.Query().Get("code_challenge")
		redirect := r.URL.Query().Get("redirect_uri")
		f.mu.Unlock()
		callback, _ := url.Parse(redirect)
		q := callback.Query()
		q.Set("code", f.codeIssued)
		q.Set("state", f.state)
		callback.RawQuery = q.Encode()
		http.Redirect(w, r, callback.String(), http.StatusFound)
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code") != f.codeIssued {
				http.Error(w, "bad code", http.StatusBadRequest)
				return
			}
			if r.Form.Get("code_verifier") == "" {
				http.Error(w, "pkce required", http.StatusBadRequest)
				return
			}
			f.exchanged = true
			writeTokenJSON(t, w, f.accessToken, f.refresh, 3600)
		case "refresh_token":
			if r.Form.Get("refresh_token") != f.refresh {
				http.Error(w, "bad refresh", http.StatusBadRequest)
				return
			}
			f.refreshHits++
			f.accessToken = "refreshed-access-token-" + fmt.Sprint(f.refreshHits)
			writeTokenJSON(t, w, f.accessToken, f.refresh, 3600)
		default:
			http.Error(w, "unsupported grant", http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/oauth/revoke", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func writeTokenJSON(t *testing.T, w http.ResponseWriter, access, refresh string, expiresIn int) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"access_token":  access,
		"refresh_token": refresh,
		"token_type":    "Bearer",
		"expires_in":    expiresIn,
	}); err != nil {
		t.Errorf("encode token response: %v", err)
	}
}

func newTestLogin(t *testing.T, issuer string) (*cliauth.Login, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth.json")
	login, err := cliauth.NewLogin(cliauth.LoginOptions{
		Issuer:   issuer,
		CredPath: path,
	})
	if err != nil {
		t.Fatalf("NewLogin: %v", err)
	}
	return login, path
}

func TestNewLogin_UsesDefaultIssuerScopedStore(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	login, err := cliauth.NewLogin(cliauth.LoginOptions{Issuer: "https://skills.example.com/"})
	if err != nil {
		t.Fatalf("NewLogin: %v", err)
	}
	if !strings.HasPrefix(login.Store.Path(), filepath.Join(configHome, "mcp-kit")) {
		t.Fatalf("store path = %q, want under mcp-kit config dir", login.Store.Path())
	}
	if !strings.HasSuffix(login.Store.Path(), "credentials.json") {
		t.Fatalf("store path = %q, want credentials.json", login.Store.Path())
	}
}

func TestNewLogin_UsesOverrideStoreAndOpenURL(t *testing.T) {
	store := cliauth.NewCredStore(filepath.Join(t.TempDir(), "custom.json"))
	openURL := func(string) error { return nil }

	login, err := cliauth.NewLogin(cliauth.LoginOptions{
		Issuer:  "https://skills.example.com",
		Store:   store,
		OpenURL: openURL,
	})
	if err != nil {
		t.Fatalf("NewLogin: %v", err)
	}
	if login.Store != store {
		t.Fatal("NewLogin did not preserve custom store")
	}
	if login.OpenURL == nil {
		t.Fatal("NewLogin did not preserve OpenURL override")
	}
}

func TestLogin_Loopback_HappyPath(t *testing.T) {
	fake := newFakeAuthServer(t)
	login, _ := newTestLogin(t, fake.server.URL)
	login.OpenURL = func(authURL string) error {
		go func() {
			resp, err := http.Get(authURL)
			if err != nil {
				t.Errorf("simulated browser get: %v", err)
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			if err := resp.Body.Close(); err != nil {
				t.Errorf("close response body: %v", err)
			}
		}()
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := login.RunLoopback(ctx, cliauth.LoopbackOptions{Port: 0, Scope: "skills.read skills.write"}); err != nil {
		t.Fatalf("RunLoopback: %v", err)
	}

	creds, err := login.Store.LoadHost(mustHost(t, fake.server.URL))
	if err != nil {
		t.Fatalf("load creds: %v", err)
	}
	if creds.AccessToken != "access-token-1" {
		t.Fatalf("access_token = %q, want access-token-1", creds.AccessToken)
	}
	if creds.RefreshToken != "refresh-token-1" {
		t.Fatalf("refresh_token = %q, want refresh-token-1", creds.RefreshToken)
	}
	if creds.ClientID != "fake-client-id" {
		t.Fatalf("client_id = %q, want fake-client-id", creds.ClientID)
	}
	if !fake.exchanged {
		t.Fatal("token exchange did not run")
	}
	if fake.pkceChall == "" {
		t.Fatal("PKCE challenge was not sent")
	}
}

func TestLogin_Loopback_ReregistersWhenRedirectURINotCached(t *testing.T) {
	fake := newFakeAuthServer(t)
	login, _ := newTestLogin(t, fake.server.URL)
	host := mustHost(t, fake.server.URL)
	if err := login.Store.SaveHost(host, cliauth.Credentials{
		Issuer:       fake.server.URL,
		ClientID:     "old-client-id",
		RedirectURIs: []string{"http://127.0.0.1:1/callback"},
	}); err != nil {
		t.Fatalf("save old client: %v", err)
	}
	login.OpenURL = func(authURL string) error {
		go func() {
			resp, err := http.Get(authURL)
			if err != nil {
				t.Errorf("simulated browser get: %v", err)
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			if err := resp.Body.Close(); err != nil {
				t.Errorf("close response body: %v", err)
			}
		}()
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := login.RunLoopback(ctx, cliauth.LoopbackOptions{Port: 0, Scope: "mcp.read"}); err != nil {
		t.Fatalf("RunLoopback: %v", err)
	}

	if fake.dcrCalls != 1 {
		t.Fatalf("dcrCalls = %d, want 1", fake.dcrCalls)
	}
	creds, err := login.Store.LoadHost(host)
	if err != nil {
		t.Fatalf("load creds: %v", err)
	}
	if creds.ClientID != "fake-client-id" {
		t.Fatalf("client_id = %q, want re-registered fake-client-id", creds.ClientID)
	}
	if len(creds.RedirectURIs) != 1 || creds.RedirectURIs[0] == "http://127.0.0.1:1/callback" {
		t.Fatalf("redirect URIs were not replaced: %#v", creds.RedirectURIs)
	}
}

func TestLogin_Paste_HappyPath(t *testing.T) {
	fake := newFakeAuthServer(t)
	login, _ := newTestLogin(t, fake.server.URL)
	out := &bytes.Buffer{}
	capturedCh := make(chan string, 1)
	login.OpenURL = func(authURL string) error {
		capturedCh <- authURL
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := login.RunPasteRedirect(ctx, cliauth.PasteOptions{
		Port:  8765,
		Scope: "skills.read skills.write",
		In:    newSimulatedPaste(t, capturedCh),
		Out:   out,
	}); err != nil {
		t.Fatalf("RunPasteRedirect: %v", err)
	}
	creds, err := login.Store.LoadHost(mustHost(t, fake.server.URL))
	if err != nil {
		t.Fatalf("load creds: %v", err)
	}
	if creds.AccessToken != "access-token-1" {
		t.Fatalf("access_token = %q", creds.AccessToken)
	}
	if !strings.Contains(out.String(), "Open this URL") {
		t.Fatalf("expected paste prompt in output, got %q", out.String())
	}
}

func newSimulatedPaste(t *testing.T, capturedCh <-chan string) io.Reader {
	t.Helper()
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		var captured string
		select {
		case captured = <-capturedCh:
		case <-time.After(2 * time.Second):
			_ = pw.CloseWithError(fmt.Errorf("authorize URL was never captured"))
			return
		}
		client := &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		resp, err := client.Get(captured)
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		defer resp.Body.Close()
		loc := resp.Header.Get("Location")
		if loc == "" {
			_ = pw.CloseWithError(fmt.Errorf("authorize did not redirect: %d", resp.StatusCode))
			return
		}
		if _, err := pw.Write([]byte(loc + "\n")); err != nil {
			t.Errorf("write paste: %v", err)
		}
	}()
	return pr
}

func TestLogin_GetAccessToken_RefreshesExpired(t *testing.T) {
	fake := newFakeAuthServer(t)
	login, _ := newTestLogin(t, fake.server.URL)
	if err := login.Store.SaveHost(mustHost(t, fake.server.URL), cliauth.Credentials{
		Issuer:       fake.server.URL,
		ClientID:     "fake-client-id",
		AccessToken:  "old",
		RefreshToken: "refresh-token-1",
		ExpiresAt:    time.Now().UTC().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	tok, err := login.GetAccessToken(context.Background())
	if err != nil {
		t.Fatalf("GetAccessToken: %v", err)
	}
	if !strings.HasPrefix(tok, "refreshed-access-token-") {
		t.Fatalf("token = %q, expected refreshed", tok)
	}
	if fake.refreshHits != 1 {
		t.Fatalf("refresh hits = %d, want 1", fake.refreshHits)
	}
}

func TestLogin_GetAccessToken_NoRefreshIfFresh(t *testing.T) {
	fake := newFakeAuthServer(t)
	login, _ := newTestLogin(t, fake.server.URL)
	if err := login.Store.SaveHost(mustHost(t, fake.server.URL), cliauth.Credentials{
		Issuer:       fake.server.URL,
		ClientID:     "fake-client-id",
		AccessToken:  "fresh",
		RefreshToken: "refresh-token-1",
		ExpiresAt:    time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	tok, err := login.GetAccessToken(context.Background())
	if err != nil {
		t.Fatalf("GetAccessToken: %v", err)
	}
	if tok != "fresh" {
		t.Fatalf("token = %q, want fresh", tok)
	}
	if fake.refreshHits != 0 {
		t.Fatalf("refresh hits = %d, want 0", fake.refreshHits)
	}
}

func TestLogin_GetAccessToken_ErrNotLoggedIn(t *testing.T) {
	fake := newFakeAuthServer(t)
	login, _ := newTestLogin(t, fake.server.URL)

	_, err := login.GetAccessToken(context.Background())
	if !errors.Is(err, cliauth.ErrNotLoggedIn) {
		t.Fatalf("err = %v, want ErrNotLoggedIn", err)
	}
}

func TestLogin_GetAccessToken_RejectsIssuerMismatch(t *testing.T) {
	fake := newFakeAuthServer(t)
	login, _ := newTestLogin(t, fake.server.URL)
	host := mustHost(t, fake.server.URL)
	if err := login.Store.SaveHost(host, cliauth.Credentials{
		Issuer:       fake.server.URL + "/other",
		ClientID:     "fake-client-id",
		AccessToken:  "wrong-issuer-token",
		RefreshToken: "refresh-token-1",
		ExpiresAt:    time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	_, err := login.GetAccessToken(context.Background())
	if !errors.Is(err, cliauth.ErrNotLoggedIn) {
		t.Fatalf("err = %v, want ErrNotLoggedIn", err)
	}
}

func TestLogin_Logout_DeletesCreds(t *testing.T) {
	fake := newFakeAuthServer(t)
	login, path := newTestLogin(t, fake.server.URL)
	host := mustHost(t, fake.server.URL)
	if err := login.Store.SaveHost(host, cliauth.Credentials{
		Issuer:       fake.server.URL,
		ClientID:     "fake-client-id",
		AccessToken:  "tok",
		RefreshToken: "rt",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := login.Logout(context.Background()); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := login.Store.LoadHost(host); !errors.Is(err, cliauth.ErrNotLoggedIn) {
		t.Fatalf("after logout, load err = %v, want ErrNotLoggedIn", err)
	}
	if _, statErr := osStat(path); statErr == nil {
		t.Fatal("creds file still exists after logout")
	}
}
