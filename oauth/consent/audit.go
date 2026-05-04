package consent

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/ory/fosite"

	"github.com/haakco/mcp-kit/audit"
	"github.com/haakco/mcp-kit/oauth"
)

const (
	// ActionConsentApproved is emitted when a user approves consent.
	ActionConsentApproved = "oauth.consent.approved"
	// ActionConsentDenied is emitted when a user explicitly denies consent.
	ActionConsentDenied = "oauth.consent.denied"
)

func (h *Handler) emitConsentEvent(ctx context.Context, r *http.Request, action string, requester fosite.AuthorizeRequester, subject oauth.Subject, decision string, resources []string) {
	clientID := ""
	clientName := ""
	if requester != nil && requester.GetClient() != nil {
		clientID = requester.GetClient().GetID()
		clientName = ClientNameFromRequester(requester)
	}
	scope := ""
	if requester != nil {
		scope = strings.Join(ArgumentsToStrings(requester.GetRequestedScopes()), " ")
	}

	event := audit.Event{
		EntityType: "oauth_authorize",
		EntityID:   clientID,
		Action:     action,
		ClientID:   clientID,
		Scope:      scope,
		Timestamp:  h.cfg.Now().UTC(),
		Metadata: map[string]any{
			"client_name": clientName,
			"decision":    decision,
			"ip":          clientIP(r),
			"user_agent":  r.UserAgent(),
			"resources":   resources,
		},
	}
	if subject.ID != "" {
		if parsed, err := uuid.Parse(subject.ID); err == nil {
			event.ActorUserID = &parsed
		}
	}
	_ = h.cfg.AuditEmitter.Emit(ctx, event)
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
