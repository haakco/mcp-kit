package oidc_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/haakco/mcp-kit/oauth/keys"
	"github.com/haakco/mcp-kit/oidc"
)

func TestJWKSIncludesActiveAndRetiredKeys(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	store := &memoryKeyStore{}
	manager := keys.NewManager(store, keys.WithClock(func() time.Time { return now }))
	first, err := manager.EnsureSigningKey(t.Context())
	if err != nil {
		t.Fatalf("EnsureSigningKey() error = %v", err)
	}
	second, err := manager.RotateSigningKey(t.Context(), time.Hour)
	if err != nil {
		t.Fatalf("RotateSigningKey() error = %v", err)
	}

	response := httptest.NewRecorder()
	oidc.JWKSHandler(manager).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Keys []struct {
			KID string `json:"kid"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode JWKS: %v", err)
	}
	got := map[string]bool{}
	for _, key := range payload.Keys {
		got[key.KID] = true
	}
	if !got[first.KID] || !got[second.KID] {
		t.Fatalf("JWKS kids = %#v, want active and retired keys", got)
	}
}

func TestJWKSRejectsNonGET(t *testing.T) {
	manager := keys.NewManager(&memoryKeyStore{})
	response := httptest.NewRecorder()

	oidc.JWKSHandler(manager).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/.well-known/jwks.json", nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", response.Code)
	}
	if response.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", response.Header().Get("Allow"))
	}
}

type memoryKeyStore struct {
	mu   sync.Mutex
	keys []keys.SigningKey
}

func (s *memoryKeyStore) FindActiveSigningKey(context.Context) (keys.SigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range s.keys {
		if key.IsActive {
			return key, nil
		}
	}
	return keys.SigningKey{}, keys.ErrNotFound
}

func (s *memoryKeyStore) EnsureSigningKey(_ context.Context, key keys.SigningKey) (keys.SigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.keys {
		if existing.IsActive {
			return existing, nil
		}
	}
	s.keys = append(s.keys, key)
	return key, nil
}

func (s *memoryKeyStore) RotateSigningKey(_ context.Context, replacement keys.SigningKey, retiredAt time.Time) (keys.SigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.keys {
		if s.keys[i].IsActive {
			s.keys[i].IsActive = false
			s.keys[i].RetiredAt = &retiredAt
		}
	}
	s.keys = append(s.keys, replacement)
	return replacement, nil
}

func (s *memoryKeyStore) ListVerifyingSigningKeys(_ context.Context, now time.Time) ([]keys.SigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []keys.SigningKey
	for _, key := range s.keys {
		if key.IsActive || (key.RetiredAt != nil && key.RetiredAt.After(now)) {
			result = append(result, key)
		}
	}
	return result, nil
}

func (s *memoryKeyStore) DeleteExpiredSigningKeys(_ context.Context, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var kept []keys.SigningKey
	deleted := 0
	for _, key := range s.keys {
		if !key.IsActive && key.RetiredAt != nil && key.RetiredAt.Before(now) {
			deleted++
			continue
		}
		kept = append(kept, key)
	}
	s.keys = kept
	return deleted, nil
}
