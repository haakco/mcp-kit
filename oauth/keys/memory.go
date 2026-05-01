package keys

import (
	"context"
	"sync"
	"time"
)

// MemoryStore is an in-memory signing-key store for examples and tests.
type MemoryStore struct {
	mu   sync.Mutex
	keys []SigningKey
}

// NewMemoryStore creates an empty in-memory signing-key store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (s *MemoryStore) FindActiveSigningKey(context.Context) (SigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range s.keys {
		if key.IsActive {
			return key, nil
		}
	}
	return SigningKey{}, ErrNotFound
}

func (s *MemoryStore) EnsureSigningKey(_ context.Context, key SigningKey) (SigningKey, error) {
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

func (s *MemoryStore) RotateSigningKey(_ context.Context, replacement SigningKey, retiredAt time.Time) (SigningKey, error) {
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

func (s *MemoryStore) ListVerifyingSigningKeys(_ context.Context, now time.Time) ([]SigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []SigningKey
	for _, key := range s.keys {
		if key.IsActive || (key.RetiredAt != nil && key.RetiredAt.After(now)) {
			result = append(result, key)
		}
	}
	return result, nil
}

func (s *MemoryStore) DeleteExpiredSigningKeys(_ context.Context, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var kept []SigningKey
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
