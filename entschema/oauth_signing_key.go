package entschema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
)

// OAuthSigningKey provides fields for JWT signing key rotation.
type OAuthSigningKey struct {
	mixin.Schema
}

// Fields returns signing key persistence fields.
func (OAuthSigningKey) Fields() []ent.Field {
	return []ent.Field{
		field.String("kid").
			Unique().
			NotEmpty().
			MaxLen(255).
			Comment("Key ID for JWKS"),
		field.String("algorithm").
			Default("RS256").
			MaxLen(32).
			Comment("Signing algorithm"),
		field.Text("private_key_pem").
			Sensitive().
			Comment("PEM-encoded RSA private key"),
		field.Text("public_key_pem").
			Comment("PEM-encoded RSA public key"),
		field.Bool("is_active").
			Default(true).
			Comment("Only active keys sign new tokens"),
		field.Time("expires_at").
			Optional().
			Nillable().
			Comment("Key expiry for rotation"),
		field.Time("retired_at").
			Optional().
			Nillable().
			Comment("Retired keys stay in JWKS until this time passes"),
	}
}

// Indexes returns signing key indexes.
func (OAuthSigningKey) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("kid"),
		index.Fields("is_active"),
	}
}
