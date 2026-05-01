package mcpkit

import (
	"errors"
	"net/http"

	"github.com/haakco/mcp-kit/audit"
	"github.com/haakco/mcp-kit/mcpmw"
	"github.com/haakco/mcp-kit/oauth"
)

// ErrNotImplemented is returned by symbols that are stubbed in v0.1.0.
// They will be implemented in v0.2.0 (OAuth core extracted from skills-mcp).
var ErrNotImplemented = errors.New("mcpkit: not implemented in v0.1.0")

// Config configures a new MCP server.
//
// In v0.1.0 most fields are accepted but unused — New returns
// ErrNotImplemented until the OAuth core lands. Use mcpmw.Envelope and
// mcpmw.Origin standalone in the meantime.
type Config struct {
	// Implementation identifies the MCP server (Name + Version).
	// Required when v0.2.0 lands.
	Implementation any // mcp.Implementation in v0.2.0; any to keep v0.1.0 dep-free

	// Instructions are the server-level guidance shown to clients on initialize.
	Instructions string

	// Handler is the SDK MCP HTTP handler before kit middleware.
	Handler http.Handler

	// Bearer authenticates bearer tokens before the SDK handler.
	Bearer BearerConfig

	// Validator is deprecated. Use Bearer.TokenValidator.
	Validator oauth.TokenValidator

	// AllowedOrigins is the Origin header allowlist for browser clients.
	AllowedOrigins []string

	// AllowLoopback permits Origin: http://127.0.0.1[:port], http://localhost[:port],
	// http://[::1][:port]. Default false. Set true in dev.
	AllowLoopback bool

	// AuditEmitter receives tool-call audit events. Required for production.
	// For tests, use audit.Discard().
	AuditEmitter audit.Emitter
}

// Server wraps the SDK MCP server with the kit's middleware composed.
type Server struct {
	cfg     Config
	handler http.Handler
}

// New constructs an MCP server from the given config.
func New(cfg Config) (*Server, error) {
	if cfg.Handler == nil {
		return nil, errors.New("mcpkit: handler is required")
	}
	if cfg.Bearer.TokenValidator == nil {
		cfg.Bearer.TokenValidator = cfg.Validator
	}

	handler := mcpmw.Envelope(cfg.Handler)
	handler = oauth.Bearer(oauth.BearerConfig(cfg.Bearer))(handler)
	handler = mcpmw.Origin(mcpmw.OriginConfig{
		Allowed:       cfg.AllowedOrigins,
		AllowLoopback: cfg.AllowLoopback,
	}, handler)

	return &Server{cfg: cfg, handler: handler}, nil
}

// Handler returns the http.Handler to mount at /mcp.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// BearerConfig is an alias for oauth.BearerConfig.
type BearerConfig = oauth.BearerConfig
