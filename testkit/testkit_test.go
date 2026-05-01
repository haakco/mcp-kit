package testkit_test

import (
	"context"
	"testing"

	"github.com/haakco/mcp-kit/testkit"
	"github.com/haakco/mcp-kit/userstore"
)

func TestNewServerMintTokenAndHandshake(t *testing.T) {
	server := testkit.NewServer(t)
	token := testkit.MintToken(t, "mcp.read")

	sessionID := testkit.RunHandshake(t, server, token)
	if sessionID == "" {
		t.Fatal("sessionID is empty")
	}
	tools := testkit.ListTools(t, server, token, sessionID)
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
	if _, err := store.FindByEmail(context.Background(), "missing@example.com"); err != userstore.ErrNotFound {
		t.Fatalf("missing err = %v, want ErrNotFound", err)
	}
}
