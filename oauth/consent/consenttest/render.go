package consenttest

import (
	"net/http"
	"sync"

	"github.com/haakco/mcp-kit/oauth/consent"
)

// CapturingRenderer records rendered pages for assertions.
type CapturingRenderer struct {
	mu       sync.Mutex
	LastPage consent.Page
	LastData consent.PageData
	Pages    []consent.Page
}

// Render implements consent.Renderer.
func (r *CapturingRenderer) Render(w http.ResponseWriter, page consent.Page, data consent.PageData) {
	r.mu.Lock()
	r.LastPage = page
	r.LastData = data
	r.Pages = append(r.Pages, page)
	r.mu.Unlock()
	if page != consent.PageRedirectBridge {
		w.WriteHeader(http.StatusOK)
	}
}

// Seen reports whether page has been rendered.
func (r *CapturingRenderer) Seen(page consent.Page) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, got := range r.Pages {
		if got == page {
			return true
		}
	}
	return false
}
