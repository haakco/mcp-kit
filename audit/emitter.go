// Package audit defines the audit emitter interface that mcp-kit consumers
// implement to receive tool-call, OAuth, and key-rotation events.
//
// Consumers wrap their existing audit log behind this interface; the kit
// itself owns no audit table or storage.
package audit

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Event is an audit-log entry emitted by the kit on behalf of the consumer.
// Consumers may extend Metadata with their own fields.
type Event struct {
	// EntityType is the kind of thing this event is about, e.g.
	// "mcp_tool", "oauth_token", "oauth_key", "oauth_client".
	EntityType string

	// EntityID identifies the entity. Tool name, jti, kid, client_id, etc.
	EntityID string

	// Action is the verb. "execute", "issued", "rotated", "revoked", ...
	Action string

	// ActorUserID is the user who triggered the event. Nil for system actions
	// (key rotation, server boot) and for unauthenticated events.
	ActorUserID *uuid.UUID

	// ClientID is the OAuth client_id (registered) or PAT id, when
	// applicable. Empty otherwise.
	ClientID string

	// Scope is the comma-separated active scope list at the time of the
	// event. Empty when not applicable.
	Scope string

	// PayloadHash is a hex sha256 of the redacted request payload, when the
	// event refers to a tool call. Empty otherwise.
	PayloadHash string

	// Metadata is free-form extra data — must be JSON-serializable. Run
	// through Redact before emitting if it may contain user input.
	Metadata map[string]any

	// Timestamp is when the event was observed. Defaults to time.Now() if
	// zero when the kit emits.
	Timestamp time.Time
}

// Emitter receives audit events from the kit. Implementations must be safe
// for concurrent use.
type Emitter interface {
	Emit(ctx context.Context, event Event) error
}

// Discard returns an Emitter that drops every event. Intended for tests and
// for wiring through a code path that does not need audit logging yet.
func Discard() Emitter { return discardEmitter{} }

type discardEmitter struct{}

func (discardEmitter) Emit(context.Context, Event) error { return nil }
