package oauth

import (
	"context"
	"fmt"
)

// Servicer is the consumer-owned persistence/service boundary for PATs.
type Servicer interface {
	ValidateToken(ctx context.Context, rawToken string) (PATEntity, error)
	GetTokenScope(ctx context.Context, tokenID string) (scopeType string, scopeTarget string, err error)
	UpdateLastUsed(ctx context.Context, tokenID string) error
}

// PATEntity is the minimum resolved PAT shape needed for authentication.
type PATEntity interface {
	GetID() string
	GetUserID() string
	GetScopes() []string
}

// PATValidator adapts a PATServicer to Bearer middleware.
type PATValidator struct {
	service Servicer
}

// NewPATValidator creates a PAT validator.
func NewPATValidator(service Servicer) *PATValidator {
	return &PATValidator{service: service}
}

// ValidateAndResolve validates a raw PAT and resolves its scope restriction.
func (v *PATValidator) ValidateAndResolve(ctx context.Context, rawToken string) (*PATAuthResult, error) {
	pat, err := v.service.ValidateToken(ctx, rawToken)
	if err != nil {
		return nil, fmt.Errorf("validate PAT: %w", err)
	}

	scopeType, scopeTarget, err := v.service.GetTokenScope(ctx, pat.GetID())
	if err != nil {
		return nil, fmt.Errorf("get PAT scope: %w", err)
	}

	return &PATAuthResult{
		UserID:      pat.GetUserID(),
		TokenID:     pat.GetID(),
		ScopeType:   scopeType,
		ScopeTarget: scopeTarget,
		Scopes:      append([]string{}, pat.GetScopes()...),
	}, nil
}

// RecordUsage updates last_used_at. Bearer calls this asynchronously.
func (v *PATValidator) RecordUsage(ctx context.Context, tokenID string) {
	_ = v.service.UpdateLastUsed(ctx, tokenID) //nolint:errcheck // last-used updates must not fail authenticated requests
}
