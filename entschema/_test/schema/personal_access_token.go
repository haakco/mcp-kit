package schema

import (
	"entgo.io/ent"

	kitschema "github.com/haakco/mcp-kit/entschema"
)

type PersonalAccessToken struct {
	ent.Schema
}

func (PersonalAccessToken) Mixin() []ent.Mixin {
	return []ent.Mixin{kitschema.PersonalAccessToken{}}
}
