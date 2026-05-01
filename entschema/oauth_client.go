package entschema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
)

// OAuthClient provides fields for OAuth dynamic clients.
type OAuthClient struct {
	mixin.Schema
}

// Fields returns the OAuth client persistence fields.
func (OAuthClient) Fields() []ent.Field {
	return []ent.Field{
		field.String("client_id").
			Unique().
			NotEmpty().
			MaxLen(255).
			Comment("OAuth 2.0 client identifier"),
		field.String("client_secret_hash").
			Default("").
			Sensitive().
			Comment("Hashed client secret, empty for public clients"),
		field.JSON("redirect_uris", []string{}).
			Default([]string{}).
			Comment("Allowed redirect URIs"),
		field.JSON("grant_types", []string{}).
			Default([]string{}).
			Comment("Allowed grant types"),
		field.JSON("response_types", []string{}).
			Default([]string{}).
			Comment("Allowed response types"),
		field.String("scopes").
			Default("").
			MaxLen(1000).
			Comment("Space-separated allowed scopes"),
		field.Bool("is_public").
			Default(false).
			Comment("True for public clients"),
		field.String("name").
			NotEmpty().
			MaxLen(200).
			Comment("Human-readable client name"),
		field.Int("workspace_id").
			Optional().
			Nillable().
			Comment("Optional consumer workspace/tenant ID"),
	}
}

// Indexes returns OAuth client indexes.
func (OAuthClient) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("client_id"),
		index.Fields("workspace_id"),
	}
}
