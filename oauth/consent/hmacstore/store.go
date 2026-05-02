package hmacstore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/haakco/mcp-kit/oauth"
	"github.com/haakco/mcp-kit/oauth/consent"
)

// Store is a stateless HMAC implementation of consent.ApprovalTokenStore.
type Store struct {
	key []byte
	now func() time.Time

	mu     sync.Mutex
	issued map[string]time.Time
}

// New constructs a Store with the supplied HMAC key and clock.
func New(key []byte, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{key: append([]byte{}, key...), now: now, issued: make(map[string]time.Time)}
}

// Issue implements consent.ApprovalTokenStore.
func (s *Store) Issue(_ context.Context, subject oauth.Subject, params url.Values) (string, error) {
	expiresAt := s.now().Add(consent.ApprovalTokenTTL()).Unix()
	payload := subject.ID + "|" + strconv.FormatInt(expiresAt, 10) + "|" + consent.ParamsDigest(params)
	signature := s.sign(payload)
	token := base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + signature))

	s.mu.Lock()
	s.issued[token] = time.Unix(expiresAt, 0)
	s.mu.Unlock()
	return token, nil
}

// Consume implements consent.ApprovalTokenStore.
func (s *Store) Consume(_ context.Context, token string, params url.Values) (oauth.Subject, error) {
	if token == "" {
		return oauth.Subject{}, consent.ErrApprovalTokenInvalid
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return oauth.Subject{}, consent.ErrApprovalTokenInvalid
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 4 {
		return oauth.Subject{}, consent.ErrApprovalTokenInvalid
	}
	payload := strings.Join(parts[:3], "|")
	if !hmac.Equal([]byte(parts[3]), []byte(s.sign(payload))) {
		return oauth.Subject{}, consent.ErrApprovalTokenInvalid
	}
	expiresAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return oauth.Subject{}, consent.ErrApprovalTokenInvalid
	}
	if s.now().Unix() > expiresAt {
		s.deleteStored(token)
		return oauth.Subject{}, consent.ErrApprovalTokenInvalid
	}
	if parts[2] != consent.ParamsDigest(params) {
		return oauth.Subject{}, consent.ErrApprovalTokenInvalid
	}
	if !s.consumeStored(token) {
		return oauth.Subject{}, consent.ErrApprovalTokenInvalid
	}
	return oauth.Subject{ID: parts[0]}, nil
}

func (s *Store) consumeStored(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	expiresAt, ok := s.issued[token]
	if !ok {
		return false
	}
	delete(s.issued, token)
	return s.now().Before(expiresAt)
}

func (s *Store) deleteStored(token string) {
	s.mu.Lock()
	delete(s.issued, token)
	s.mu.Unlock()
}

func (s *Store) sign(payload string) string {
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
