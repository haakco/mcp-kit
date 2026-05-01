package cliauth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ErrNotLoggedIn is returned when no credentials exist for the requested host.
var ErrNotLoggedIn = errors.New("cliauth: not logged in")

const refreshBuffer = 30 * time.Second

// Credentials is one issuer host's persisted OAuth state.
type Credentials struct {
	Issuer       string    `json:"issuer"`
	ClientID     string    `json:"client_id"`
	RedirectURIs []string  `json:"redirect_uris,omitempty"`
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitzero"`
	Scope        string    `json:"scope,omitempty"`
}

// NeedsRefresh reports whether the access token should be refreshed before use.
func (c Credentials) NeedsRefresh() bool {
	if c.ExpiresAt.IsZero() {
		return true
	}
	return time.Until(c.ExpiresAt) <= refreshBuffer
}

type store struct {
	Current string                 `json:"current"`
	Hosts   map[string]Credentials `json:"hosts"`
}

// CredStore persists OAuth credentials for one or more hosts.
type CredStore struct {
	path string
}

// NewCredStore returns a store backed by path.
func NewCredStore(path string) *CredStore {
	return &CredStore{path: path}
}

// Path returns the underlying credentials file path.
func (s *CredStore) Path() string {
	return s.path
}

// HostFromIssuer returns the canonical host key for an issuer URL.
func HostFromIssuer(issuer string) (string, error) {
	if strings.TrimSpace(issuer) == "" {
		return "", errors.New("issuer is empty")
	}
	u, err := url.Parse(issuer)
	if err != nil {
		return "", fmt.Errorf("parse issuer: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("issuer must use http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return "", errors.New("issuer has no host")
	}
	return strings.ToLower(u.Host), nil
}

// DefaultCredPath returns the standard issuer-scoped credentials path.
func DefaultCredPath(issuer string) (string, error) {
	normalized, err := normalizeIssuer(issuer)
	if err != nil {
		return "", err
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate home dir: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	sum := sha256.Sum256([]byte(normalized))
	return filepath.Join(base, "mcp-kit", hex.EncodeToString(sum[:]), "credentials.json"), nil
}

func normalizeIssuer(issuer string) (string, error) {
	if _, err := HostFromIssuer(issuer); err != nil {
		return "", err
	}
	return strings.TrimRight(strings.TrimSpace(issuer), "/"), nil
}

// LoadHost returns the credentials for host.
func (s *CredStore) LoadHost(host string) (Credentials, error) {
	st, err := s.loadAll()
	if err != nil {
		return Credentials{}, err
	}
	c, ok := st.Hosts[host]
	if !ok {
		return Credentials{}, ErrNotLoggedIn
	}
	return c, nil
}

// LoadCurrent returns the current host and its credentials.
func (s *CredStore) LoadCurrent() (string, Credentials, error) {
	st, err := s.loadAll()
	if err != nil {
		return "", Credentials{}, err
	}
	if st.Current == "" {
		return "", Credentials{}, ErrNotLoggedIn
	}
	c, ok := st.Hosts[st.Current]
	if !ok {
		return "", Credentials{}, ErrNotLoggedIn
	}
	return st.Current, c, nil
}

// SaveHost upserts one host's credentials.
func (s *CredStore) SaveHost(host string, creds Credentials) error {
	if host == "" {
		return errors.New("host is empty")
	}
	st, err := s.loadAll()
	if err != nil && !errors.Is(err, ErrNotLoggedIn) {
		return err
	}
	if st.Hosts == nil {
		st.Hosts = map[string]Credentials{}
	}
	st.Hosts[host] = creds
	if st.Current == "" {
		st.Current = host
	}
	return s.write(st)
}

// UseHost flips Current to host.
func (s *CredStore) UseHost(host string) error {
	st, err := s.loadAll()
	if err != nil {
		return err
	}
	if _, ok := st.Hosts[host]; !ok {
		return fmt.Errorf("host %q not found in credentials file", host)
	}
	st.Current = host
	return s.write(st)
}

// DeleteHost removes one host. If it was current, current advances by sort order.
func (s *CredStore) DeleteHost(host string) error {
	st, err := s.loadAll()
	if errors.Is(err, ErrNotLoggedIn) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, ok := st.Hosts[host]; !ok {
		return nil
	}
	delete(st.Hosts, host)
	if st.Current == host {
		st.Current = firstHost(st.Hosts)
	}
	if len(st.Hosts) == 0 {
		return s.removeFile()
	}
	return s.write(st)
}

// DeleteAll removes the entire credentials file.
func (s *CredStore) DeleteAll() error {
	return s.removeFile()
}

// Hosts returns sorted host keys.
func (s *CredStore) Hosts() ([]string, error) {
	st, err := s.loadAll()
	if errors.Is(err, ErrNotLoggedIn) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(st.Hosts))
	for k := range st.Hosts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *CredStore) loadAll() (store, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return store{}, ErrNotLoggedIn
	}
	if err != nil {
		return store{}, fmt.Errorf("read credentials: %w", err)
	}
	var st store
	if err := json.Unmarshal(data, &st); err != nil {
		return store{}, fmt.Errorf("parse credentials: %w", err)
	}
	if st.Hosts == nil {
		return store{}, ErrNotLoggedIn
	}
	return st, nil
}

func (s *CredStore) write(st store) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credentials: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".auth-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) } //nolint:errcheck // best-effort temp-file cleanup after write failure
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close() //nolint:errcheck // already failed; surfacing chmod error
		cleanup()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close() //nolint:errcheck // already failed; surfacing write error
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		cleanup()
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

func (s *CredStore) removeFile() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove credentials: %w", err)
	}
	return nil
}

func firstHost(hosts map[string]Credentials) string {
	if len(hosts) == 0 {
		return ""
	}
	keys := make([]string, 0, len(hosts))
	for k := range hosts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys[0]
}
