package cliauth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/haakco/mcp-kit/cliauth"
)

func TestLoginRejectsCrossOriginDiscoveredEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/oauth-authorization-server" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]string{
			"authorization_endpoint": "https://evil.example/oauth/authorize",
			"token_endpoint":         "https://evil.example/oauth/token",
			"registration_endpoint":  "https://evil.example/oauth/register",
		}); err != nil {
			t.Errorf("encode metadata: %v", err)
		}
	}))
	defer server.Close()

	login, err := cliauth.NewLogin(cliauth.LoginOptions{
		Issuer:   server.URL,
		CredPath: t.TempDir() + "/credentials.json",
	})
	if err != nil {
		t.Fatalf("NewLogin: %v", err)
	}

	if err := login.RunLoopback(context.Background(), cliauth.LoopbackOptions{Port: 0}); err == nil {
		t.Fatal("RunLoopback accepted cross-origin endpoints")
	}
}

func TestLoginRejectsNonHTTPDiscoveredEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/oauth-authorization-server" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]string{
			"authorization_endpoint": "javascript:alert(1)",
			"token_endpoint":         serverURL(r) + "/oauth/token",
			"registration_endpoint":  serverURL(r) + "/oauth/register",
		}); err != nil {
			t.Errorf("encode metadata: %v", err)
		}
	}))
	defer server.Close()

	login, err := cliauth.NewLogin(cliauth.LoginOptions{
		Issuer:   server.URL,
		CredPath: t.TempDir() + "/credentials.json",
	})
	if err != nil {
		t.Fatalf("NewLogin: %v", err)
	}

	if err := login.RunLoopback(context.Background(), cliauth.LoopbackOptions{Port: 0}); err == nil {
		t.Fatal("RunLoopback accepted non-http endpoint")
	}
}

func TestLoginRejectsUnsafeCompleteEndpointOverrides(t *testing.T) {
	login, err := cliauth.NewLogin(cliauth.LoginOptions{
		Issuer:   "http://localhost:8899",
		CredPath: t.TempDir() + "/credentials.json",
		Endpoints: cliauth.Endpoints{
			Authorization: "javascript:alert(1)",
			Token:         "https://evil.example/oauth/token",
			Registration:  "https://evil.example/oauth/register",
		},
	})
	if err != nil {
		t.Fatalf("NewLogin: %v", err)
	}

	if err := login.RunLoopback(context.Background(), cliauth.LoopbackOptions{Port: 0}); err == nil {
		t.Fatal("RunLoopback accepted unsafe complete endpoint overrides")
	}
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
