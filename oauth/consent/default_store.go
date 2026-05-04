package consent

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
)

type defaultApprovalStore struct {
	key []byte
	now func() time.Time

	mu     sync.Mutex
	issued map[string]time.Time
}

func newDefaultApprovalStore(key []byte, now func() time.Time) *defaultApprovalStore {
	return &defaultApprovalStore{key: append([]byte{}, key...), now: now, issued: make(map[string]time.Time)}
}

func (s *defaultApprovalStore) Issue(_ context.Context, subject oauth.Subject, params url.Values) (string, error) {
	expiresAt := s.now().Add(ApprovalTokenTTL()).Unix()
	payload := subject.ID + "|" + strconv.FormatInt(expiresAt, 10) + "|" + ParamsDigest(params)
	signature := s.sign(payload)
	token := base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + signature))

	s.mu.Lock()
	s.issued[token] = time.Unix(expiresAt, 0)
	s.mu.Unlock()
	return token, nil
}

func (s *defaultApprovalStore) Consume(_ context.Context, token string, params url.Values) (oauth.Subject, error) {
	if token == "" {
		return oauth.Subject{}, ErrApprovalTokenInvalid
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return oauth.Subject{}, ErrApprovalTokenInvalid
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 4 {
		return oauth.Subject{}, ErrApprovalTokenInvalid
	}
	payload := strings.Join(parts[:3], "|")
	if !hmac.Equal([]byte(parts[3]), []byte(s.sign(payload))) {
		return oauth.Subject{}, ErrApprovalTokenInvalid
	}
	expiresAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return oauth.Subject{}, ErrApprovalTokenInvalid
	}
	if s.now().Unix() > expiresAt {
		s.deleteStored(token)
		return oauth.Subject{}, ErrApprovalTokenInvalid
	}
	if parts[2] != ParamsDigest(params) {
		return oauth.Subject{}, ErrApprovalTokenInvalid
	}
	if !s.consumeStored(token) {
		return oauth.Subject{}, ErrApprovalTokenInvalid
	}
	return oauth.Subject{ID: parts[0]}, nil
}

func (s *defaultApprovalStore) consumeStored(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	expiresAt, ok := s.issued[token]
	if !ok {
		return false
	}
	delete(s.issued, token)
	return s.now().Before(expiresAt)
}

func (s *defaultApprovalStore) deleteStored(token string) {
	s.mu.Lock()
	delete(s.issued, token)
	s.mu.Unlock()
}

func (s *defaultApprovalStore) sign(payload string) string {
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
