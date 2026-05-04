// Package sessionstore implements consent.ApprovalTokenStore using a
// consumer-supplied session backend.
package sessionstore

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrNotFound is returned when a session entry is absent, expired, or already pulled.
var ErrNotFound = errors.New("sessionstore: not found")

// Entry is the payload stored by SessionBackend.
type Entry struct {
	SubjectID    string
	SubjectEmail string
	SubjectExtra map[string]any
	Params       map[string][]string
	ExpiresAt    time.Time
}

// SessionBackend stores and atomically pulls approval-token entries.
type SessionBackend interface {
	Put(ctx context.Context, key string, entry Entry, ttl time.Duration) error
	Pull(ctx context.Context, key string) (Entry, error)
}

// MemoryBackend is an in-process SessionBackend for tests and simple deployments.
type MemoryBackend struct {
	mu    sync.Mutex
	items map[string]memoryItem
	now   func() time.Time
}

type memoryItem struct {
	entry     Entry
	expiresAt time.Time
}

// NewMemoryBackend returns a MemoryBackend.
func NewMemoryBackend(now func() time.Time) *MemoryBackend {
	if now == nil {
		now = time.Now
	}
	return &MemoryBackend{items: make(map[string]memoryItem), now: now}
}

// Put implements SessionBackend.
func (b *MemoryBackend) Put(_ context.Context, key string, entry Entry, ttl time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items[key] = memoryItem{entry: entry, expiresAt: b.now().Add(ttl)}
	return nil
}

// Pull implements SessionBackend.
func (b *MemoryBackend) Pull(_ context.Context, key string) (Entry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	item, ok := b.items[key]
	if !ok {
		return Entry{}, ErrNotFound
	}
	delete(b.items, key)
	if b.now().After(item.expiresAt) {
		return Entry{}, ErrNotFound
	}
	return item.entry, nil
}
