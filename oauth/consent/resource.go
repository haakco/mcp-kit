package consent

import "net/http"

// ResourceURLFunc derives the canonical MCP resource URL for a request.
type ResourceURLFunc func(r *http.Request) (string, error)

// StaticResourceURL returns a ResourceURLFunc that always returns url.
func StaticResourceURL(url string) ResourceURLFunc {
	return func(*http.Request) (string, error) { return url, nil }
}
