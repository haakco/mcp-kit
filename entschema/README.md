# Ent Schema Mixins

`entschema` exposes OAuth and PAT Ent mixins for consumers that already own
their Ent schema package.

Example:

```go
package schema

import (
	"entgo.io/ent"
	kitschema "github.com/haakco/mcp-kit/entschema"
)

type OAuthClient struct{ ent.Schema }

func (OAuthClient) Mixin() []ent.Mixin {
	return []ent.Mixin{
		kitschema.OAuthClient{},
	}
}
```

The mixins intentionally include only kit-owned OAuth/PAT columns and indexes.
Consumers should compose their own timestamp, UUID, soft-delete, and edge
conventions alongside these mixins.

`OAuthAuthorizationCode`, `OAuthAccessToken`, and `OAuthRefreshToken` currently
share the same discriminator-backed field shape so the first `skills-mcp`
migration can stay schema-compatible while consumers can choose separate schema
types later.
