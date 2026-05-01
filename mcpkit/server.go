package mcpkit

import (
	"errors"
	"net/http"

	"github.com/haakco/mcp-kit/audit"
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

	// Validator authenticates bearer tokens. Required when v0.2.0 lands.
	// Typically obtained from oauth.Provider.TokenValidator().
	Validator any // oauth.TokenValidator in v0.2.0

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
	cfg Config
}

// New constructs an MCP server from the given config.
//
// v0.1.0: returns ErrNotImplemented. v0.2.0 will compose the SDK server with
// origin → bearer → envelope → SDK handler chain.
func New(cfg Config) (*Server, error) {
	return nil, ErrNotImplemented
}

// Handler returns the http.Handler to mount at /mcp.
//
// v0.1.0: returns a handler that responds with 501 to every request.
// v0.2.0 will return the composed handler.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "mcpkit: server not implemented in v0.1.0", http.StatusNotImplemented)
	})
}
