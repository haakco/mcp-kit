package consenttest

import (
	"net/url"
	"testing"

	"github.com/google/uuid"

	"github.com/haakco/mcp-kit/oauth"
	"github.com/haakco/mcp-kit/oauth/consent"
)

func TestConsenttestSelfTest(t *testing.T) {
	provider, store := Provider(t, "https://app.example.com")
	RegisterClient(t, store, ClientOptions{})
	renderer := &CapturingRenderer{}
	handler, err := consent.NewHandler(consent.Config{
		Provider:       provider,
		Authenticator:  StaticAuth(map[string]oauth.Subject{"alice@example.com:password": {ID: uuid.NewString(), Email: "alice@example.com"}}),
		Renderer:       renderer,
		PublicURL:      "https://app.example.com",
		ResourceURL:    consent.StaticResourceURL("https://app.example.com/mcp"),
		ApprovalSecret: []byte("approval-secret-32-byte-value!!!"),
	})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	RunCanonicalSuite(t, handler, SuiteOptions{
		Renderer: renderer,
		AuthorizeValues: url.Values{
			"response_type":         {"code"},
			"client_id":             {"client-id"},
			"redirect_uri":          {"http://127.0.0.1/callback"},
			"scope":                 {"openid mcp.read"},
			"state":                 {"state-123456"},
			"code_challenge":        {S256Challenge("verifier-123456789012345678901234567890")},
			"code_challenge_method": {"S256"},
			"resource":              {"https://app.example.com/mcp"},
		},
	})
}
