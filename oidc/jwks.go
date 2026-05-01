package oidc

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/haakco/mcp-kit/oauth/keys"
)

// JWKSHandler returns a handler for /.well-known/jwks.json.
func JWKSHandler(manager *keys.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		jwks, err := manager.JWKS(r.Context())
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(jwks); err != nil {
			slog.Error("write JWKS response", "error", err)
		}
	})
}
