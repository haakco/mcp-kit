package consent

// Page identifies which page the Renderer should render.
type Page int

const (
	// PageLogin is the initial form and the re-render after failed login.
	PageLogin Page = iota

	// PageConsent is rendered after a successful credential check.
	PageConsent

	// PageRedirectBridge is rendered after a successful approve.
	PageRedirectBridge
)

// HiddenInput is one hidden form field the renderer must emit.
type HiddenInput struct {
	Name  string
	Value string
}

// PageData is the data the kit hands the Renderer.
type PageData struct {
	Authenticated bool
	DisplayName   string
	ClientName    string
	Scopes        []string
	Resources     []string
	HiddenInputs  []HiddenInput
	FormAction    string
	Error         string
	RedirectURL   string
}
