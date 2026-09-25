package mcpkit

import (
	"errors"
	"net/http"

	"github.com/haakco/mcp-kit/audit"
	"github.com/haakco/mcp-kit/mcpmw"
	"github.com/haakco/mcp-kit/oauth"
)

// Config configures a new MCP server.
//
// Config covers the kit-owned middleware around the SDK handler. It does not
// build or configure the SDK server itself: the consumer owns the
// mcp.Server, its identity, its instructions, and its transport options.
type Config struct {
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
	handler = oauth.Bearer(cfg.Bearer)(handler)
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
