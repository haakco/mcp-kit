package schema

import (
	"entgo.io/ent"

	kitschema "github.com/haakco/mcp-kit/entschema"
)

type OAuthSigningKey struct {
	ent.Schema
}

func (OAuthSigningKey) Mixin() []ent.Mixin {
	return []ent.Mixin{kitschema.OAuthSigningKey{}}
}
