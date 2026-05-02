package consent

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/haakco/mcp-kit/audit"
	"github.com/haakco/mcp-kit/oauth"
)

const approvalSecretLength = 32

const approvalTokenTTL = 5 * time.Minute

// Config wires consent.NewHandler.
type Config struct {
	Provider       *oauth.Provider
	Authenticator  Authenticator
	Renderer       Renderer
	ApprovalStore  ApprovalTokenStore
	ApprovalSecret []byte
	ConsentPolicy  ConsentPolicy
	Challenge      ChallengeProvider
	ResourceURL    ResourceURLFunc
	PublicURL      string
	FormPath       string
	AuditEmitter   audit.Emitter
	FormBodyLimit  int64
	Now            func() time.Time
	Logger         *slog.Logger
}

func (c *Config) applyDefaults() error {
	if c.Provider == nil {
		return errors.New("consent: Provider is required")
	}
	if c.Authenticator == nil {
		return errors.New("consent: Authenticator is required")
	}
	if c.Renderer == nil {
		return errors.New("consent: Renderer is required")
	}
	if c.PublicURL == "" {
		return errors.New("consent: PublicURL is required")
	}
	if c.FormPath == "" {
		c.FormPath = "/oauth/authorize"
	}
	if c.ResourceURL == nil {
		c.ResourceURL = StaticResourceURL(c.PublicURL + "/mcp")
	}
	if c.ConsentPolicy == nil {
		c.ConsentPolicy = AlwaysAsk()
	}
	if c.AuditEmitter == nil {
		c.AuditEmitter = audit.Discard()
	}
	if c.FormBodyLimit == 0 {
		c.FormBodyLimit = 64 << 10
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return nil
}

// ValidateApprovalSecret enforces the 32-byte rule on operator-supplied
// approval secrets.
func ValidateApprovalSecret(secret []byte) error {
	if len(secret) != approvalSecretLength {
		return fmt.Errorf("consent: ApprovalSecret must be exactly %d bytes, got %d", approvalSecretLength, len(secret))
	}
	return nil
}

// ApprovalTokenTTL returns the bound on approval-token lifetime.
func ApprovalTokenTTL() time.Duration { return approvalTokenTTL }
