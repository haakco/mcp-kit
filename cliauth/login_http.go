package cliauth

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func buildAuthorizeURL(endpoint, clientID, redirectURI, scope, state, challenge string) string { //nolint:revive // OAuth authorize has six required inputs.
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", scope)
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	return endpoint + "?" + q.Encode()
}

func postToken(ctx context.Context, httpClient *http.Client, endpoint string, form url.Values) (*tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var tok tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, fmt.Errorf("decode token: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, errors.New("token endpoint returned empty access_token")
	}
	return &tok, nil
}

func revokeToken(ctx context.Context, httpClient *http.Client, endpoint, clientID, token string) error {
	form := url.Values{}
	form.Set("token", token)
	form.Set("token_type_hint", "refresh_token")
	form.Set("client_id", clientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

type loopbackCallbackResult struct {
	code  string
	err   error
	state string
}

func startLoopbackCallback(listener net.Listener) (<-chan loopbackCallbackResult, *http.Server) {
	resCh := make(chan loopbackCallbackResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if errCode := q.Get("error"); errCode != "" {
			desc := q.Get("error_description")
			fmt.Fprint(w, callbackErrorPage(errCode, desc))
			resCh <- loopbackCallbackResult{err: fmt.Errorf("authorize error: %s: %s", errCode, desc)}
			return
		}
		fmt.Fprint(w, callbackSuccessPage)
		resCh <- loopbackCallbackResult{code: q.Get("code"), state: q.Get("state")}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("login server serve failed", "error", err)
		}
	}()
	return resCh, srv
}

func readPastedRedirect(in io.Reader, expectedState string) (string, error) {
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("read paste: %w", err)
		}
		return "", errors.New("no input received")
	}
	pasted := strings.TrimSpace(scanner.Text())
	if pasted == "" {
		return "", errors.New("pasted URL is empty")
	}
	parsed, err := url.Parse(pasted)
	if err != nil {
		return "", fmt.Errorf("parse pasted URL: %w", err)
	}
	q := parsed.Query()
	if errCode := q.Get("error"); errCode != "" {
		return "", fmt.Errorf("authorize error: %s: %s", errCode, q.Get("error_description"))
	}
	if got := q.Get("state"); got != expectedState {
		return "", fmt.Errorf("state mismatch: got %q want %q", got, expectedState)
	}
	code := q.Get("code")
	if code == "" {
		return "", errors.New("pasted URL has no ?code parameter")
	}
	return code, nil
}

// CallbackPort returns a TCP port from addr when available.
func CallbackPort(addr net.Addr) int {
	if t, ok := addr.(*net.TCPAddr); ok {
		return t.Port
	}
	if _, p, err := net.SplitHostPort(addr.String()); err == nil {
		if n, err := strconv.Atoi(p); err == nil {
			return n
		}
	}
	return 0
}

const callbackSuccessPage = `<!doctype html><html lang="en">
<head><meta charset="utf-8"><title>Authorized</title>
<style>body{font-family:system-ui,sans-serif;max-width:520px;margin:80px auto;padding:0 20px;color:#0f172a}
h1{color:#16a34a}p{color:#475569}</style></head>
<body><h1>Authorized</h1>
<p>You can close this tab and return to the terminal.</p></body></html>`

func callbackErrorPage(code, desc string) string {
	return fmt.Sprintf(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Error</title>
<style>body{font-family:system-ui,sans-serif;max-width:520px;margin:80px auto;padding:0 20px;color:#0f172a}
h1{color:#dc2626}p{color:#475569}</style></head>
<body><h1>Authorization failed</h1>
<p><strong>%s</strong></p><p>%s</p>
<p>Return to the terminal; the CLI has the details.</p></body></html>`, html.EscapeString(code), html.EscapeString(desc))
}
