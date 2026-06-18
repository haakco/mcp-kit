package consent

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/haakco/mcp-kit/oauth"
)

func TestDefaultApprovalStoreIssuesUniqueTokensForSameRequest(t *testing.T) {
	now := time.Date(2026, 6, 18, 21, 0, 0, 0, time.UTC)
	store := newDefaultApprovalStore(mustApprovalKey(t), func() time.Time { return now })
	subject := oauth.Subject{ID: "user-1"}
	params := url.Values{"client_id": {"abc"}, "state": {"same-state"}, "scope": {"openid"}}

	first, err := store.Issue(context.Background(), subject, params)
	if err != nil {
		t.Fatalf("first Issue: %v", err)
	}
	second, err := store.Issue(context.Background(), subject, params)
	if err != nil {
		t.Fatalf("second Issue: %v", err)
	}
	if first == second {
		t.Fatal("Issue returned identical tokens for the same request in the same second")
	}

	if _, err := store.Consume(context.Background(), first, params); err != nil {
		t.Fatalf("Consume first token: %v", err)
	}
	if _, err := store.Consume(context.Background(), second, params); err != nil {
		t.Fatalf("Consume second token: %v", err)
	}
}

func mustApprovalKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	return key
}
