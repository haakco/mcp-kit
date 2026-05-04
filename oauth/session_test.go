package oauth

import "testing"

func TestSubjectExtraRoundTripsViaNewSession(t *testing.T) {
	subject := Subject{
		ID:    "11111111-1111-1111-1111-111111111111",
		Email: "alice@example.com",
		Extra: map[string]any{"role": "admin"},
	}

	session := NewSession(subject)

	role, ok := session.Claims.Extra["role"].(string)
	if !ok || role != "admin" {
		t.Fatalf("Extra[role] not propagated to session claims: %#v", session.Claims.Extra)
	}
}
