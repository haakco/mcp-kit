package consent

import (
	"context"
	"errors"

	"github.com/haakco/mcp-kit/oauth"
)

// ErrChallengeRequired tells Handler to pause before consent for extra
// verification.
var ErrChallengeRequired = errors.New("consent: additional challenge required")

// ChallengeProvider runs an optional second-factor step between login and
// consent. No stock implementation ships in the kit.
type ChallengeProvider interface {
	Begin(ctx context.Context, subject oauth.Subject) (challengeID string, err error)
	Verify(ctx context.Context, challengeID string, response string) error
}
