package entschema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
)

// PersonalAccessToken provides fields for hashed PAT storage.
type PersonalAccessToken struct {
	mixin.Schema
}

// Fields returns PAT persistence fields.
func (PersonalAccessToken) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id").
			Comment("Consumer user ID that owns the token"),
		field.String("name").
			NotEmpty().
			MaxLen(200).
			Comment("Human-readable token name"),
		field.String("token_hash").
			NotEmpty().
			MaxLen(64).
			Unique().
			Comment("SHA-256 hash of the raw token"),
		field.String("token_prefix").
			NotEmpty().
			MaxLen(20).
			Comment("Safe token prefix for display"),
		field.Time("last_used_at").
			Optional().
			Nillable().
			Comment("Last time this token authenticated"),
		field.Time("expires_at").
			Optional().
			Nillable().
			Comment("Token expiry, nil means no expiry"),
	}
}

// Indexes returns PAT indexes.
func (PersonalAccessToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("token_prefix"),
		index.Fields("user_id"),
	}
}
