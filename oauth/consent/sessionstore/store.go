package sessionstore

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/haakco/mcp-kit/oauth"
	"github.com/haakco/mcp-kit/oauth/consent"
)

// Store is a session-backed implementation of consent.ApprovalTokenStore.
type Store struct {
	backend SessionBackend
	now     func() time.Time
}

// New constructs a Store backed by backend.
func New(backend SessionBackend, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{backend: backend, now: now}
}

// Issue implements consent.ApprovalTokenStore.
func (s *Store) Issue(ctx context.Context, subject oauth.Subject, params url.Values) (string, error) {
	key, err := newKey()
	if err != nil {
		return "", err
	}
	entry := Entry{
		SubjectID:    subject.ID,
		SubjectEmail: subject.Email,
		SubjectExtra: subject.Extra,
		Params:       cloneValues(params),
		ExpiresAt:    s.now().Add(consent.ApprovalTokenTTL()),
	}
	if err := s.backend.Put(ctx, key, entry, consent.ApprovalTokenTTL()); err != nil {
		return "", err
	}
	return key + "." + consent.ParamsDigest(params), nil
}

// Consume implements consent.ApprovalTokenStore.
func (s *Store) Consume(ctx context.Context, token string, params url.Values) (oauth.Subject, error) {
	key, digest, ok := strings.Cut(token, ".")
	if !ok || key == "" || digest == "" {
		return oauth.Subject{}, consent.ErrApprovalTokenInvalid
	}
	if digest != consent.ParamsDigest(params) {
		return oauth.Subject{}, consent.ErrApprovalTokenInvalid
	}
	entry, err := s.backend.Pull(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return oauth.Subject{}, consent.ErrApprovalTokenInvalid
	}
	if err != nil {
		return oauth.Subject{}, err
	}
	if s.now().After(entry.ExpiresAt) {
		return oauth.Subject{}, consent.ErrApprovalTokenInvalid
	}
	if consent.ParamsDigest(url.Values(entry.Params)) != digest {
		return oauth.Subject{}, consent.ErrApprovalTokenInvalid
	}
	return oauth.Subject{ID: entry.SubjectID, Email: entry.SubjectEmail, Extra: entry.SubjectExtra}, nil
}

func newKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func cloneValues(in url.Values) map[string][]string {
	out := make(map[string][]string, len(in))
	for key, values := range in {
		out[key] = append([]string{}, values...)
	}
	return out
}
