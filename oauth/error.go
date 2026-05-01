package oauth

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func writeOAuthErrorBody(w http.ResponseWriter, code string, description string) {
	if err := json.NewEncoder(w).Encode(map[string]string{
		"error":             code,
		"error_description": description,
	}); err != nil {
		slog.Error("write OAuth error response", "error", err)
	}
}
