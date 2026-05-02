// Package consent provides one shared OAuth 2.1 authorization-endpoint
// handler that performs browser-based login, optional 2FA challenge, explicit
// user consent, and produces a fosite authorization response with the
// canonical MCP audience bound server-side per RFC 8707.
//
// Consumers wire five collaborators. The kit owns the protocol mechanics;
// the consumer owns user model and UI:
//
//   - Authenticator verifies credentials and returns oauth.Subject.
//   - Renderer writes login, consent, and redirect-bridge HTML.
//   - ApprovalTokenStore issues and consumes single-use approval tokens.
//   - ConsentPolicy decides skip-consent and validates grantable scopes.
//   - ChallengeProvider optionally runs an extra verification step.
package consent
