package consent

import (
	"net/url"
	"testing"
)

func TestOAuthAuthorizeValuesStripsCredentialFields(t *testing.T) {
	in := url.Values{
		"client_id":          {"abc"},
		"username":           {"alice@example.com"},
		"password":           {"hunter2"},
		"action":             {"login"},
		"approval_token":     {"deadbeef"},
		"challenge_id":       {"challenge"},
		"challenge_response": {"123456"},
		"state":              {"xxxxxxxx"},
		"code_challenge":     {"sha256-base64"},
		"resource":           {"https://example.com/mcp"},
	}

	out := oauthAuthorizeValues(in)

	for _, banned := range []string{"username", "password", "action", "approval_token", "challenge_id", "challenge_response"} {
		if _, has := out[banned]; has {
			t.Errorf("%q must be stripped", banned)
		}
	}
	for _, kept := range []string{"client_id", "state", "code_challenge", "resource"} {
		if _, has := out[kept]; !has {
			t.Errorf("%q must be retained", kept)
		}
	}
}

func TestParamsDigestStableAcrossLoginAndApprove(t *testing.T) {
	login := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}, "username": {"a"}}
	approve := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}, "approval_token": {"deadbeef"}}
	if ParamsDigest(login) != ParamsDigest(approve) {
		t.Fatal("digest must be stable across credential and token field churn")
	}
}

func TestParamsDigestDiffersOnCanonicalChange(t *testing.T) {
	a := url.Values{"client_id": {"abc"}, "state": {"xxxxxxxx"}}
	b := url.Values{"client_id": {"DIFFERENT"}, "state": {"xxxxxxxx"}}
	if ParamsDigest(a) == ParamsDigest(b) {
		t.Fatal("digest must change when canonical params change")
	}
}

func TestNormalizeOAuthStringSliceDropsBlanksAndDupes(t *testing.T) {
	out := normalizeOAuthStringSlice([]string{"https://x/", "", "  ", "https://x/", "https://y/"})
	if len(out) != 2 || out[0] != "https://x/" || out[1] != "https://y/" {
		t.Fatalf("got %#v", out)
	}
}
