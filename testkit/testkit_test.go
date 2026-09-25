package testkit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/haakco/mcp-kit/mcpkit"
	"github.com/haakco/mcp-kit/testkit"
	"github.com/haakco/mcp-kit/userstore"
)

func TestNewServerDiscoversToolsOverStatelessHTTP(t *testing.T) {
	server := testkit.NewServer(t)
	token := testkit.MintToken(t, "mcp.read")

	discover := testkit.Discover(t, server, token)
	if len(discover.SupportedVersions) != 1 || discover.SupportedVersions[0] != mcpkit.ProtocolVersion {
		t.Fatalf("supportedVersions = %v, want [%s]", discover.SupportedVersions, mcpkit.ProtocolVersion)
	}
	if ttlMs, scope := discover.GetTTLMs(), discover.GetCacheScope(); ttlMs <= 0 || scope != "private" {
		t.Fatalf("discover cache = (%d, %q), want positive ttl and private scope", ttlMs, scope)
	}

	tools := testkit.ListTools(t, server, token)
	testkit.AssertChecklistCoverage(t, tools, []string{"hello_world"})
}

func TestNewUserStoreFindsSingleUser(t *testing.T) {
	store := testkit.NewUserStore(t, "mcp.read", "mcp.write")

	user, err := store.FindByEmail(context.Background(), "TEST@example.com")
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}
	if user.Email() != "test@example.com" {
		t.Fatalf("email = %q, want test@example.com", user.Email())
	}
	if _, err := store.FindByID(context.Background(), user.ID()); err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if _, err := store.FindByEmail(context.Background(), "missing@example.com"); !errors.Is(err, userstore.ErrNotFound) {
		t.Fatalf("missing err = %v, want ErrNotFound", err)
	}
}
