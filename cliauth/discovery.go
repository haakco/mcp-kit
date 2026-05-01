package cliauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type metadataResponse struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RevocationEndpoint    string `json:"revocation_endpoint"`
	RegistrationEndpoint  string `json:"registration_endpoint"`
}

func discoverEndpoints(ctx context.Context, httpClient *http.Client, issuer string) (Endpoints, error) {
	endpoint := strings.TrimRight(issuer, "/") + "/.well-known/oauth-authorization-server"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Endpoints{}, fmt.Errorf("build issuer metadata request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return Endpoints{}, fmt.Errorf("issuer metadata request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return Endpoints{}, fmt.Errorf("issuer metadata failed: %s", resp.Status)
	}

	var metadata metadataResponse
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return Endpoints{}, fmt.Errorf("decode issuer metadata: %w", err)
	}
	return Endpoints{
		Authorization: metadata.AuthorizationEndpoint,
		Token:         metadata.TokenEndpoint,
		Revocation:    metadata.RevocationEndpoint,
		Registration:  metadata.RegistrationEndpoint,
	}, nil
}

func (e Endpoints) complete() bool {
	return e.Authorization != "" && e.Token != "" && e.Registration != ""
}

func (e Endpoints) validate(issuer string) error {
	for name, endpoint := range map[string]string{
		"authorization_endpoint": e.Authorization,
		"token_endpoint":         e.Token,
		"revocation_endpoint":    e.Revocation,
		"registration_endpoint":  e.Registration,
	} {
		if endpoint == "" && name == "revocation_endpoint" {
			continue
		}
		if err := validateEndpoint(issuer, endpoint); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func (e Endpoints) merge(fallback Endpoints) Endpoints {
	if e.Authorization == "" {
		e.Authorization = fallback.Authorization
	}
	if e.Token == "" {
		e.Token = fallback.Token
	}
	if e.Revocation == "" {
		e.Revocation = fallback.Revocation
	}
	if e.Registration == "" {
		e.Registration = fallback.Registration
	}
	return e
}

func validateEndpoint(issuer string, endpoint string) error {
	issuerURL, err := url.Parse(strings.TrimRight(issuer, "/"))
	if err != nil {
		return fmt.Errorf("parse issuer: %w", err)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return fmt.Errorf("endpoint %q must be absolute", endpoint)
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("endpoint %q must not contain a fragment", endpoint)
	}
	if parsed.Scheme != issuerURL.Scheme || !strings.EqualFold(parsed.Host, issuerURL.Host) {
		return fmt.Errorf("endpoint %q must share issuer origin", endpoint)
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && isLoopbackEndpointHost(parsed.Hostname()) {
		return nil
	}
	return fmt.Errorf("endpoint %q must use https or loopback http", endpoint)
}

func isLoopbackEndpointHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
