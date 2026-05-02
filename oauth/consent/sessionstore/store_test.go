package sessionstore

import (
	"context"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/haakco/mcp-kit/oauth"
	"github.com/haakco/mcp-kit/oauth/consent"
)

func TestStoreRoundTrip(t *testing.T) {
	store := New(NewMemoryBackend(time.Now), time.Now)
	sub := oauth.Subject{ID: uuid.NewString(), Email: "alice@example.com", Extra: map[string]any{"role": "admin"}}
	params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}

	token, err := store.Issue(context.Background(), sub, params)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	got, err := store.Consume(context.Background(), token, params)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if got.ID != sub.ID || got.Email != sub.Email {
		t.Fatalf("subject mismatch: got %+v want %+v", got, sub)
	}
	if !reflect.DeepEqual(got.Extra, sub.Extra) {
		t.Fatalf("Extra mismatch: got %+v want %+v", got.Extra, sub.Extra)
	}
}

func TestStoreRejectsReplay(t *testing.T) {
	store := New(NewMemoryBackend(time.Now), time.Now)
	params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
	token, _ := store.Issue(context.Background(), oauth.Subject{ID: uuid.NewString()}, params)

	if _, err := store.Consume(context.Background(), token, params); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if _, err := store.Consume(context.Background(), token, params); err != consent.ErrApprovalTokenInvalid {
		t.Fatalf("second consume err = %v, want ErrApprovalTokenInvalid", err)
	}
}

func TestStoreRejectsParamMismatch(t *testing.T) {
	store := New(NewMemoryBackend(time.Now), time.Now)
	original := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
	tampered := url.Values{"client_id": {"DIFFERENT"}, "state": {"xxxxxxxx"}}
	token, _ := store.Issue(context.Background(), oauth.Subject{ID: uuid.NewString()}, original)

	if _, err := store.Consume(context.Background(), token, tampered); err != consent.ErrApprovalTokenInvalid {
		t.Fatalf("mismatched consume err = %v, want ErrApprovalTokenInvalid", err)
	}
}

func TestStoreRejectsExpired(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	store := New(NewMemoryBackend(clock), clock)
	params := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
	token, _ := store.Issue(context.Background(), oauth.Subject{ID: uuid.NewString()}, params)

	now = now.Add(consent.ApprovalTokenTTL() + time.Second)

	if _, err := store.Consume(context.Background(), token, params); err != consent.ErrApprovalTokenInvalid {
		t.Fatalf("expired consume err = %v, want ErrApprovalTokenInvalid", err)
	}
}
