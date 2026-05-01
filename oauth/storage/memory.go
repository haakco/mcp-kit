package storage

import (
	"context"
	"sync"
)

// MemoryStore is an in-memory Store implementation for tests and examples.
type MemoryStore struct {
	mu       sync.Mutex
	clients  map[string]Client
	sessions map[sessionKey]Session
}

// NewMemoryStore creates an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		clients:  map[string]Client{},
		sessions: map[sessionKey]Session{},
	}
}

// SaveClient stores or replaces an OAuth client.
func (s *MemoryStore) SaveClient(_ context.Context, client Client) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clients[client.ID] = cloneClient(client)
	return nil
}

func (s *MemoryStore) GetClient(_ context.Context, clientID string) (Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	client, ok := s.clients[clientID]
	if !ok {
		return Client{}, ErrNotFound
	}
	return cloneClient(client), nil
}

func (s *MemoryStore) SaveSession(_ context.Context, session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[sessionKey{sessionType: session.Type, signature: session.Signature}] = cloneSession(session)
	return nil
}

func (s *MemoryStore) GetSession(_ context.Context, sessionType string, signature string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionKey{sessionType: sessionType, signature: signature}]
	if !ok {
		return Session{}, ErrNotFound
	}
	return cloneSession(session), nil
}

func (s *MemoryStore) SetSessionActive(_ context.Context, sessionType string, signature string, active bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := sessionKey{sessionType: sessionType, signature: signature}
	session, ok := s.sessions[key]
	if !ok {
		return ErrNotFound
	}
	session.Active = active
	s.sessions[key] = session
	return nil
}

func (s *MemoryStore) DeleteSession(_ context.Context, sessionType string, signature string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, sessionKey{sessionType: sessionType, signature: signature})
	return nil
}

func (s *MemoryStore) DeleteSessionsByRequestID(_ context.Context, sessionType string, requestID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, session := range s.sessions {
		if key.sessionType == sessionType && session.RequestID == requestID {
			delete(s.sessions, key)
		}
	}
	return nil
}

type sessionKey struct {
	sessionType string
	signature   string
}

func cloneClient(client Client) Client {
	client.RedirectURIs = append([]string{}, client.RedirectURIs...)
	client.GrantTypes = append([]string{}, client.GrantTypes...)
	client.ResponseTypes = append([]string{}, client.ResponseTypes...)
	client.Scopes = append([]string{}, client.Scopes...)
	client.Audience = append([]string{}, client.Audience...)
	return client
}

func cloneSession(session Session) Session {
	session.Scopes = append([]string{}, session.Scopes...)
	session.Data = cloneMap(session.Data)
	if session.ExpiresAt != nil {
		expiresAt := *session.ExpiresAt
		session.ExpiresAt = &expiresAt
	}
	return session
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	cloned := map[string]any{}
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}
