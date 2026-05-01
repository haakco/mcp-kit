package entschema

import (
	"fmt"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
)

type oauthSession struct {
	mixin.Schema
}

// Fields returns session persistence fields.
func (oauthSession) Fields() []ent.Field {
	return []ent.Field{
		field.String("session_type").
			MaxLen(32).
			Validate(validateSessionType).
			Comment("Type: authorization_code, access_token, refresh_token, pkce, oidc"),
		field.String("signature").
			NotEmpty().
			MaxLen(512).
			Comment("Unique token/code signature for lookup"),
		field.String("client_id").
			NotEmpty().
			MaxLen(255).
			Comment("OAuth client ID that owns this session"),
		field.Int("user_id").
			Optional().
			Nillable().
			Comment("Optional consumer user ID"),
		field.String("request_id").
			Default("").
			MaxLen(255).
			Comment("Fosite request ID for token revocation"),
		field.String("scopes").
			Default("").
			MaxLen(1000).
			Comment("Space-separated granted scopes"),
		field.JSON("session_data", map[string]any{}).
			Default(map[string]any{}).
			Comment("Serialized Fosite session data"),
		field.Bool("active").
			Default(true).
			Comment("Whether this session is still valid"),
		field.Time("expires_at").
			Optional().
			Nillable().
			Comment("When this session expires"),
		field.Time("requested_at").
			Optional().
			Nillable().
			Comment("When Fosite created the original request"),
	}
}

// Indexes returns session indexes.
func (oauthSession) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_type", "signature").Unique(),
		index.Fields("client_id"),
		index.Fields("user_id"),
		index.Fields("request_id"),
		index.Fields("session_type", "active"),
	}
}

func validateSessionType(value string) error {
	switch value {
	case "authorization_code", "access_token", "refresh_token", "pkce", "oidc":
		return nil
	default:
		return fmt.Errorf("invalid session_type %q", value)
	}
}
