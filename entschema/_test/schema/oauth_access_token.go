package schema

import (
	"entgo.io/ent"

	kitschema "github.com/haakco/mcp-kit/entschema"
)

type OAuthAccessToken struct {
	ent.Schema
}

func (OAuthAccessToken) Mixin() []ent.Mixin {
	return []ent.Mixin{kitschema.OAuthAccessToken{}}
}
