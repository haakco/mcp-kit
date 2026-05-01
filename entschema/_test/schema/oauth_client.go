package schema

import (
	"entgo.io/ent"

	kitschema "github.com/haakco/mcp-kit/entschema"
)

type OAuthClient struct {
	ent.Schema
}

func (OAuthClient) Mixin() []ent.Mixin {
	return []ent.Mixin{kitschema.OAuthClient{}}
}
