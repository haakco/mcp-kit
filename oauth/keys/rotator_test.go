package keys_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/haakco/mcp-kit/audit"
	"github.com/haakco/mcp-kit/oauth/keys"
)

func TestKeyRotatorRotatesWhenActiveKeyOlderThanInterval(t *testing.T) {
	store := newMemoryStore()
	manager := keys.NewManager(store)

	first, err := manager.EnsureSigningKey(t.Context())
	if err != nil {
		t.Fatalf("EnsureSigningKey() error = %v", err)
	}

	auditEmitter := &recordingAuditEmitter{}
	rotator := keys.NewRotator(manager, keys.RotationConfig{
		Interval: time.Hour,
		Grace:    time.Hour,
		Now:      func() time.Time { return first.CreatedAt.Add(2 * time.Hour) },
	}, slog.Default(), keys.WithAuditEmitter(auditEmitter))

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		rotator.Run(ctx)
		close(done)
	}()

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		active, err := store.FindActiveSigningKey(t.Context())
		if err == nil && active.KID != first.KID {
			cancel()
			<-done
			if len(auditEmitter.events) != 1 {
				t.Fatalf("audit event count = %d, want 1", len(auditEmitter.events))
			}
			if auditEmitter.events[0].EntityType != "oauth_key" || auditEmitter.events[0].Action != "rotated" {
				t.Fatalf("audit event = %#v, want oauth_key rotated", auditEmitter.events[0])
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	<-done
	t.Fatal("rotator did not produce a new active signing key")
}

func TestKeyRotatorLeavesYoungActiveKeyAlone(t *testing.T) {
	store := newMemoryStore()
	manager := keys.NewManager(store)

	first, err := manager.EnsureSigningKey(t.Context())
	if err != nil {
		t.Fatalf("EnsureSigningKey() error = %v", err)
	}

	rotator := keys.NewRotator(manager, keys.RotationConfig{
		Interval: time.Hour,
		Grace:    time.Hour,
	}, nil)

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	rotator.Run(ctx)

	active, err := store.FindActiveSigningKey(t.Context())
	if err != nil {
		t.Fatalf("FindActiveSigningKey() error = %v", err)
	}
	if active.KID != first.KID {
		t.Fatalf("active KID = %q, want original %q", active.KID, first.KID)
	}
}

func TestKeyRotatorAppliesDefaults(t *testing.T) {
	store := newMemoryStore()
	manager := keys.NewManager(store)

	rotator := keys.NewRotator(manager, keys.RotationConfig{}, nil)
	if rotator == nil {
		t.Fatal("NewRotator() returned nil for zero config")
	}

	tinyRotator := keys.NewRotator(manager, keys.RotationConfig{
		Interval: time.Nanosecond,
		Grace:    time.Nanosecond,
	}, nil)

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	tinyRotator.Run(ctx)

	if keyCount := store.count(); keyCount != 0 {
		t.Fatalf("key count = %d, want 0 after clamping tiny interval", keyCount)
	}
}

type recordingAuditEmitter struct {
	events []audit.Event
}

func (r *recordingAuditEmitter) Emit(_ context.Context, event audit.Event) error {
	r.events = append(r.events, event)
	return nil
}
