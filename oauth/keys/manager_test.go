package keys_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/haakco/mcp-kit/oauth/keys"
)

func TestEnsureSigningKeyCreatesWhenAbsent(t *testing.T) {
	store := newMemoryStore()
	manager := keys.NewManager(store)

	key, err := manager.EnsureSigningKey(t.Context())
	if err != nil {
		t.Fatalf("EnsureSigningKey() error = %v", err)
	}

	if key.KID == "" {
		t.Fatal("KID is empty")
	}
	if key.Algorithm != keys.SigningAlgorithm {
		t.Fatalf("Algorithm = %q, want %q", key.Algorithm, keys.SigningAlgorithm)
	}
	if !key.IsActive {
		t.Fatal("created key is not active")
	}

	again, err := manager.EnsureSigningKey(t.Context())
	if err != nil {
		t.Fatalf("EnsureSigningKey() second call error = %v", err)
	}
	if again.KID != key.KID {
		t.Fatalf("EnsureSigningKey() created a second key: %q != %q", again.KID, key.KID)
	}
}

func TestRotateSigningKeyMarksPriorRetired(t *testing.T) {
	store := newMemoryStore()
	manager := keys.NewManager(store)

	first, err := manager.EnsureSigningKey(t.Context())
	if err != nil {
		t.Fatalf("EnsureSigningKey() error = %v", err)
	}

	grace := 24 * time.Hour
	before := time.Now()
	second, err := manager.RotateSigningKey(t.Context(), grace)
	if err != nil {
		t.Fatalf("RotateSigningKey() error = %v", err)
	}
	after := time.Now()

	if second.KID == first.KID {
		t.Fatal("rotated key reused the prior KID")
	}
	if !second.IsActive {
		t.Fatal("rotated key is not active")
	}

	prior, err := store.byKID(first.KID)
	if err != nil {
		t.Fatalf("prior key missing: %v", err)
	}
	if prior.IsActive {
		t.Fatal("prior key is still active")
	}
	if prior.RetiredAt == nil {
		t.Fatal("prior key has no RetiredAt")
	}

	minRetiredAt := before.Add(grace).Add(-time.Second)
	maxRetiredAt := after.Add(grace).Add(time.Second)
	if prior.RetiredAt.Before(minRetiredAt) || prior.RetiredAt.After(maxRetiredAt) {
		t.Fatalf("RetiredAt = %v, want between %v and %v", prior.RetiredAt, minRetiredAt, maxRetiredAt)
	}
}

func TestActiveJWKSetIncludesRetiredWithinGrace(t *testing.T) {
	store := newMemoryStore()
	manager := keys.NewManager(store)

	if _, err := manager.EnsureSigningKey(t.Context()); err != nil {
		t.Fatalf("EnsureSigningKey() error = %v", err)
	}
	if _, err := manager.RotateSigningKey(t.Context(), 24*time.Hour); err != nil {
		t.Fatalf("RotateSigningKey() error = %v", err)
	}

	jwks, err := manager.JWKS(t.Context())
	if err != nil {
		t.Fatalf("JWKS() error = %v", err)
	}
	if len(jwks.Keys) != 2 {
		t.Fatalf("JWKS key count = %d, want 2", len(jwks.Keys))
	}
	for _, key := range jwks.Keys {
		if key.Use != keys.KeyUseSignature {
			t.Fatalf("JWK use = %q, want %q", key.Use, keys.KeyUseSignature)
		}
	}
}

func TestRetireExpiredKeysDeletesPastGrace(t *testing.T) {
	store := newMemoryStore()
	manager := keys.NewManager(store)

	first, err := manager.EnsureSigningKey(t.Context())
	if err != nil {
		t.Fatalf("EnsureSigningKey() error = %v", err)
	}
	if _, err := manager.RotateSigningKey(t.Context(), 24*time.Hour); err != nil {
		t.Fatalf("RotateSigningKey() error = %v", err)
	}

	deleted, err := manager.RetireExpiredKeys(t.Context())
	if err != nil {
		t.Fatalf("RetireExpiredKeys() error = %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted = %d, want 0 before grace expires", deleted)
	}

	expired := time.Now().Add(-time.Hour)
	store.setRetiredAt(first.KID, expired)

	deleted, err = manager.RetireExpiredKeys(t.Context())
	if err != nil {
		t.Fatalf("RetireExpiredKeys() after expiry error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	if _, err := store.byKID(first.KID); !errors.Is(err, keys.ErrNotFound) {
		t.Fatalf("prior key lookup error = %v, want ErrNotFound", err)
	}
}

type memoryStore struct {
	mu   sync.Mutex
	keys []keys.SigningKey
}

func newMemoryStore() *memoryStore {
	return &memoryStore{}
}

func (s *memoryStore) FindActiveSigningKey(context.Context) (keys.SigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range s.keys {
		if key.IsActive {
			return key, nil
		}
	}
	return keys.SigningKey{}, keys.ErrNotFound
}

func (s *memoryStore) CreateSigningKey(_ context.Context, key keys.SigningKey) (keys.SigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if key.CreatedAt.IsZero() {
		key.CreatedAt = time.Now()
	}
	s.keys = append(s.keys, key)
	return key, nil
}

func (s *memoryStore) RotateSigningKey(_ context.Context, replacement keys.SigningKey, retiredAt time.Time) (keys.SigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.keys {
		if s.keys[i].IsActive {
			s.keys[i].IsActive = false
			s.keys[i].RetiredAt = &retiredAt
		}
	}
	if replacement.CreatedAt.IsZero() {
		replacement.CreatedAt = time.Now()
	}
	s.keys = append(s.keys, replacement)
	return replacement, nil
}

func (s *memoryStore) ListVerifyingSigningKeys(_ context.Context, now time.Time) ([]keys.SigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var verifying []keys.SigningKey
	for _, key := range s.keys {
		if key.IsActive || (key.RetiredAt != nil && key.RetiredAt.After(now)) {
			verifying = append(verifying, key)
		}
	}
	return verifying, nil
}

func (s *memoryStore) DeleteExpiredSigningKeys(_ context.Context, now time.Time) (int, error) {
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

func (s *memoryStore) byKID(kid string) (keys.SigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range s.keys {
		if key.KID == kid {
			return key, nil
		}
	}
	return keys.SigningKey{}, keys.ErrNotFound
}

func (s *memoryStore) setRetiredAt(kid string, retiredAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.keys {
		if s.keys[i].KID == kid {
			s.keys[i].RetiredAt = &retiredAt
		}
	}
}

func (s *memoryStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.keys)
}
