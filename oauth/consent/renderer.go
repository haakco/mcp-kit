package consent

import "net/http"

// Renderer writes one of the consent pages. When a renderer writes zero bytes
// for PageRedirectBridge, Handler falls back to a direct 302 redirect.
type Renderer interface {
	Render(w http.ResponseWriter, page Page, data PageData)
}

// RendererFunc adapts a function to Renderer.
type RendererFunc func(w http.ResponseWriter, page Page, data PageData)

// Render calls f.
func (f RendererFunc) Render(w http.ResponseWriter, page Page, data PageData) {
	f(w, page, data)
}
