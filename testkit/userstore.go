package testkit

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/haakco/mcp-kit/userstore"
)

// User is an in-memory test user with configurable scopes.
type User struct {
	IDValue           uuid.UUID
	EmailValue        string
	PasswordHashValue []byte
	Active            bool
	Scopes            []string
}

func (u User) ID() uuid.UUID { return u.IDValue }

func (u User) Email() string { return u.EmailValue }

func (u User) PasswordHash() []byte { return append([]byte{}, u.PasswordHashValue...) }

func (u User) IsActive() bool { return u.Active }

// UserStore is an in-memory userstore.Store implementation.
type UserStore struct {
	mu      sync.Mutex
	byID    map[uuid.UUID]User
	byEmail map[string]uuid.UUID
}

// NewUserStore creates a single-user in-memory user store.
func NewUserStore(t testing.TB, scopes ...string) *UserStore {
	t.Helper()
	if len(scopes) == 0 {
		scopes = []string{"mcp.read"}
	}
	user := User{
		IDValue:    uuid.New(),
		EmailValue: "test@example.com",
		Active:     true,
		Scopes:     append([]string{}, scopes...),
	}
	store := &UserStore{
		byID:    map[uuid.UUID]User{},
		byEmail: map[string]uuid.UUID{},
	}
	store.Add(user)
	return store
}

// Add inserts or replaces user.
func (s *UserStore) Add(user User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[user.IDValue] = user
	s.byEmail[strings.ToLower(user.EmailValue)] = user.IDValue
}

func (s *UserStore) FindByEmail(_ context.Context, email string) (userstore.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byEmail[strings.ToLower(email)]
	if !ok {
		return nil, userstore.ErrNotFound
	}
	return s.byID[id], nil
}

func (s *UserStore) FindByID(_ context.Context, id uuid.UUID) (userstore.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[id]
	if !ok {
		return nil, userstore.ErrNotFound
	}
	return user, nil
}
