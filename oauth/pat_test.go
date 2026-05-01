package oauth_test

import (
	"context"
	"testing"

	"github.com/haakco/mcp-kit/oauth"
)

func TestPATValidatorResolveAndRecordUsage(t *testing.T) {
	service := &fakePATService{
		token: fakePATEntity{id: "token-1", userID: "user-1", scopes: []string{"mcp.read"}},
	}
	validator := oauth.NewPATValidator(service)

	result, err := validator.ValidateAndResolve(t.Context(), "raw-token")
	if err != nil {
		t.Fatalf("ValidateAndResolve() error = %v", err)
	}
	if result.UserID != "user-1" || result.TokenID != "token-1" {
		t.Fatalf("result identity = %#v, want user-1/token-1", result)
	}
	if result.ScopeType != "workspace" || result.ScopeTarget != "workspace-1" {
		t.Fatalf("result scope = %#v, want workspace/workspace-1", result)
	}
	if len(result.Scopes) != 1 || result.Scopes[0] != "mcp.read" {
		t.Fatalf("result scopes = %#v, want [mcp.read]", result.Scopes)
	}

	validator.RecordUsage(t.Context(), "token-1")
	if service.usedTokenID != "token-1" {
		t.Fatalf("usedTokenID = %q, want token-1", service.usedTokenID)
	}
}

type fakePATService struct {
	token       fakePATEntity
	usedTokenID string
}

func (s *fakePATService) ValidateToken(_ context.Context, rawToken string) (oauth.PATEntity, error) {
	return s.token, nil
}

func (s *fakePATService) GetTokenScope(_ context.Context, tokenID string) (string, string, error) {
	return "workspace", "workspace-1", nil
}

func (s *fakePATService) UpdateLastUsed(_ context.Context, tokenID string) error {
	s.usedTokenID = tokenID
	return nil
}

type fakePATEntity struct {
	id     string
	userID string
	scopes []string
}

func (e fakePATEntity) GetID() string       { return e.id }
func (e fakePATEntity) GetUserID() string   { return e.userID }
func (e fakePATEntity) GetScopes() []string { return e.scopes }
