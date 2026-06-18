package hmacstore

import (
	"context"
	"crypto/rand"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/haakco/mcp-kit/oauth"
	"github.com/haakco/mcp-kit/oauth/consent"
)

func TestStoreRoundTrip(t *testing.T) {
	store := New(mustRandKey(t), time.Now)
	sub := oauth.Subject{ID: uuid.NewString(), Email: "alice@example.com"}
	params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}

	token, err := store.Issue(context.Background(), sub, params)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	got, err := store.Consume(context.Background(), token, params)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if got.ID != sub.ID {
		t.Fatalf("subject mismatch: got %+v want %+v", got, sub)
	}
}

func TestStoreIssuesUniqueTokensForSameRequest(t *testing.T) {
	now := time.Date(2026, 6, 18, 21, 0, 0, 0, time.UTC)
	store := New(mustRandKey(t), func() time.Time { return now })
	sub := oauth.Subject{ID: uuid.NewString()}
	params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}, "scope": {"openid"}}

	first, err := store.Issue(context.Background(), sub, params)
	if err != nil {
		t.Fatalf("first issue: %v", err)
	}
	second, err := store.Issue(context.Background(), sub, params)
	if err != nil {
		t.Fatalf("second issue: %v", err)
	}
	if first == second {
		t.Fatal("Issue returned identical tokens for the same request in the same second")
	}

	if _, err := store.Consume(context.Background(), first, params); err != nil {
		t.Fatalf("consume first token: %v", err)
	}
	if _, err := store.Consume(context.Background(), second, params); err != nil {
		t.Fatalf("consume second token: %v", err)
	}
}

func TestStoreRejectsReplay(t *testing.T) {
	store := New(mustRandKey(t), time.Now)
	sub := oauth.Subject{ID: uuid.NewString()}
	params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
	token, _ := store.Issue(context.Background(), sub, params)

	if _, err := store.Consume(context.Background(), token, params); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if _, err := store.Consume(context.Background(), token, params); !errors.Is(err, consent.ErrApprovalTokenInvalid) {
		t.Fatalf("second consume err = %v, want ErrApprovalTokenInvalid", err)
	}
}

func TestStoreRejectsExpired(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	store := New(mustRandKey(t), clock)
	params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
	token, _ := store.Issue(context.Background(), oauth.Subject{ID: uuid.NewString()}, params)

	now = now.Add(consent.ApprovalTokenTTL() + time.Second)

	if _, err := store.Consume(context.Background(), token, params); !errors.Is(err, consent.ErrApprovalTokenInvalid) {
		t.Fatalf("expired consume err = %v, want ErrApprovalTokenInvalid", err)
	}
}

func TestStoreRejectsParamMismatch(t *testing.T) {
	store := New(mustRandKey(t), time.Now)
	original := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
	tampered := url.Values{"client_id": {"DIFFERENT"}, "state": {"xxxxxxxx"}}
	token, _ := store.Issue(context.Background(), oauth.Subject{ID: uuid.NewString()}, original)

	if _, err := store.Consume(context.Background(), token, tampered); !errors.Is(err, consent.ErrApprovalTokenInvalid) {
		t.Fatalf("mismatched consume err = %v, want ErrApprovalTokenInvalid", err)
	}
}

func TestStoreRejectsForgedSignature(t *testing.T) {
	params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
	token, _ := New(mustRandKey(t), time.Now).Issue(context.Background(), oauth.Subject{ID: uuid.NewString()}, params)

	if _, err := New(mustRandKey(t), time.Now).Consume(context.Background(), token, params); !errors.Is(err, consent.ErrApprovalTokenInvalid) {
		t.Fatalf("forged-key consume err = %v, want ErrApprovalTokenInvalid", err)
	}
}

func TestStoreConcurrentConsume(t *testing.T) {
	store := New(mustRandKey(t), time.Now)
	params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
	token, _ := store.Issue(context.Background(), oauth.Subject{ID: uuid.NewString()}, params)

	results := make(chan error, 8)
	for range 8 {
		go func() {
			_, err := store.Consume(context.Background(), token, params)
			results <- err
		}()
	}

	successes := 0
	for range 8 {
		if err := <-results; err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("got %d successes; want exactly 1", successes)
	}
}

func mustRandKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return key
}
