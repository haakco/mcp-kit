package consent

import (
	"encoding/json"
	"net/http"

	"github.com/ory/fosite"
)

// WriteAuthorizeError writes an RFC 6749 JSON error envelope.
func WriteAuthorizeError(w http.ResponseWriter, err error) {
	rfc := fosite.ErrorToRFC6749Error(err)
	status := rfc.StatusCode()
	if status == 0 {
		status = http.StatusBadRequest
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(rfc)
}
