package consenttest

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/haakco/mcp-kit/oauth/consent"
)

// SuiteOptions configures RunCanonicalSuite.
type SuiteOptions struct {
	AuthorizeValues url.Values
	Username        string
	Password        string
	Renderer        *CapturingRenderer
}

// RunCanonicalSuite runs the core consent handler contract.
func RunCanonicalSuite(t testing.TB, handler *consent.Handler, opts SuiteOptions) {
	t.Helper()
	runner, ok := t.(interface {
		Run(name string, f func(t *testing.T)) bool
	})
	if !ok {
		t.Fatalf("RunCanonicalSuite requires *testing.T")
	}
	renderer := opts.Renderer
	if renderer == nil {
		t.Fatalf("SuiteOptions.Renderer is required")
	}
	values := opts.AuthorizeValues
	if values == nil {
		t.Fatalf("SuiteOptions.AuthorizeValues is required")
	}
	if opts.Username == "" {
		opts.Username = "alice@example.com"
	}
	if opts.Password == "" {
		opts.Password = "password"
	}

	runner.Run("GET renders login", func(t *testing.T) {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+values.Encode(), nil))
		if renderer.LastPage != consent.PageLogin {
			t.Fatalf("page = %v, want PageLogin", renderer.LastPage)
		}
	})

	runner.Run("login renders consent", func(t *testing.T) {
		form := cloneValues(values)
		form.Set("action", "login")
		form.Set("username", opts.Username)
		form.Set("password", opts.Password)
		handler.ServeHTTP(httptest.NewRecorder(), FormRequest(form))
		if renderer.LastPage != consent.PageConsent {
			t.Fatalf("page = %v, want PageConsent", renderer.LastPage)
		}
		if HiddenInputValue(renderer.LastData.HiddenInputs, "approval_token") == "" {
			t.Fatal("approval token hidden input is empty")
		}
	})

	runner.Run("approve redirects with code", func(t *testing.T) {
		token := HiddenInputValue(renderer.LastData.HiddenInputs, "approval_token")
		form := cloneValues(values)
		form.Set("action", "approve")
		form.Set("approval_token", token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, FormRequest(form))
		if response.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302", response.Code)
		}
		location, err := url.Parse(response.Header().Get("Location"))
		if err != nil {
			t.Fatalf("parse Location: %v", err)
		}
		if location.Query().Get("code") == "" {
			t.Fatalf("Location missing code: %s", location.String())
		}
	})
}

func cloneValues(in url.Values) url.Values {
	out := make(url.Values, len(in))
	for key, values := range in {
		out[key] = append([]string{}, values...)
	}
	return out
}
