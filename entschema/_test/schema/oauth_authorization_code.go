package schema

import (
	"entgo.io/ent"

	kitschema "github.com/haakco/mcp-kit/entschema"
)

type OAuthAuthorizationCode struct {
	ent.Schema
}

func (OAuthAuthorizationCode) Mixin() []ent.Mixin {
	return []ent.Mixin{kitschema.OAuthAuthorizationCode{}}
}
