# Recipe: Admin-only mutations across HTTP API + MCP

**Status:** Pattern, not a kit feature. The kit ships interfaces; this recipe shows how a consumer wires an admin-only gate consistently.

**Surfaced from:** skills-mcp 2026-05-02 (`AG-01` in `docs/lessons.md`).

## Problem

You expose a privileged mutation — for example, changing a record's `visibility` from `private` to `public`, or rotating a key — via two HTTP surfaces:

1. **REST API** under `/api/...`, authenticated with cookie/session middleware.
2. **MCP tool** under `/mcp`, authenticated with a bearer token via the kit's OAuth middleware.

Both surfaces eventually call the same service mutator (e.g. `UpdateSkill`). The service does the write; the handlers do auth.

If you patch the admin check into one handler and forget the other, you ship a privilege-escalation bug. This is a real failure mode, not a theoretical one — it shipped in skills-mcp Phase 1.5 and was caught in the Phase 1 validation review for the redesign milestone.

## Recommended pattern

**Push the check into the service layer.** Two surfaces, one rule, no chance of skew:

```go
// service/skill.go

type UpdateSkillInput struct {
    Title       *string
    Description *string
    Tags        *[]string
    Visibility  *string  // privileged: only admins can set this
}

func (s *SkillService) UpdateSkill(
    ctx context.Context,
    name string,
    actor Actor,           // pass the authenticated caller, not just credentials
    input UpdateSkillInput,
) (*ent.Skill, error) {
    if input.Visibility != nil {
        ok, err := s.authz.IsAdmin(ctx, actor.UserID)
        if err != nil {
            return nil, fmt.Errorf("admin probe: %w", err)
        }
        if !ok {
            return nil, ErrAdminRequired
        }
    }
    // ... rest of update
}
```

Now both handlers just resolve the actor from their own auth context and pass it through. They cannot forget the check.

## Fallback pattern: probe at every surface

If you can't push the check down (e.g. the service is shared with code paths that legitimately don't need it), then publish a single probe and call it from each surface. The kit's `authz.Service` interface from `authz/authz.go` already supports this — wire a `Check(ctx, userID, "admin")` call at every surface.

```go
// At every handler that touches the privileged field:
if err := authz.Check(ctx, userID, PermissionAdmin); err != nil {
    if errors.Is(err, authz.ErrForbidden) {
        // Render appropriate error for this surface.
        return
    }
    // Wrapped 500.
    return
}
```

For this to be safe, you need a **contract test** that exercises the privileged field via every surface with a non-admin token. Without that test, the next surface added (say, a CLI command, a webhook receiver, or an MCP resource handler) will likely skip the gate.

## Test scaffold

A minimal contract test that prevents AG-01 from regressing:

```go
func TestPrivilegedFields_RejectedAcrossSurfaces(t *testing.T) {
    privileged := []struct {
        name string
        field string
        nonAdminPayload any
    }{
        {"visibility", "visibility", "public"},
        // Add new privileged fields here. Each surface test below auto-iterates.
    }

    surfaces := []struct {
        name string
        send func(t *testing.T, field string, payload any) (status int, err string)
    }{
        {"http-api", sendHTTPUpdate},
        {"mcp-tool",  sendMCPUpdate},
        // Add new surfaces here.
    }

    for _, p := range privileged {
        for _, s := range surfaces {
            t.Run(p.name+"/"+s.name, func(t *testing.T) {
                status, errMsg := s.send(t, p.field, p.nonAdminPayload)
                if status < 400 {
                    t.Fatalf("non-admin should be denied; got status=%d body=%q", status, errMsg)
                }
            })
        }
    }
}
```

The tight loop is what makes this scale — adding a new surface or a new privileged field keeps both axes covered.

## Why this is in the kit's docs, not its code

The kit doesn't own your domain rules. It can't know which fields of `UpdateSkill` are privileged. What the kit *can* do is make it ergonomic to call the consumer's authz from any handler (`authz.Service`) and remind every consumer to enforce the rule at the right layer. This recipe is that reminder.

## Related lessons

- `AG-01` — Cross-surface admin gates need to live in the service layer.
- `AG-02` — List/Count parity for visibility-filtered endpoints.
- `EG-05` — Scope mapping should live in code (kit's existing engineering gap; same shape).

## Related kit packages

- `authz/authz.go` — the `Service` interface consumers implement.
- `oauth/middleware.go` — extracts userID from bearer tokens; same shape works for cookie sessions.
