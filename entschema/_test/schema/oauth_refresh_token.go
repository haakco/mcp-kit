package schema

import (
	"entgo.io/ent"

	kitschema "github.com/haakco/mcp-kit/entschema"
)

type OAuthRefreshToken struct {
	ent.Schema
}

func (OAuthRefreshToken) Mixin() []ent.Mixin {
	return []ent.Mixin{kitschema.OAuthRefreshToken{}}
}
