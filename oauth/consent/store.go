package consent

import (
	"context"
	"errors"
	"net/url"

	"github.com/haakco/mcp-kit/oauth"
)

// ErrApprovalTokenInvalid covers expired, forged, replayed, and mismatched
// approval tokens. Callers should not expose narrower failure reasons.
var ErrApprovalTokenInvalid = errors.New("consent: approval token invalid")

// ApprovalTokenStore mints and consumes single-use approval tokens bound to
// a specific OAuth authorize request.
type ApprovalTokenStore interface {
	Issue(ctx context.Context, subject oauth.Subject, params url.Values) (string, error)
	Consume(ctx context.Context, token string, params url.Values) (oauth.Subject, error)
}
