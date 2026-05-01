package cliauth_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/haakco/mcp-kit/cliauth"
)

func sampleCreds(issuer string) cliauth.Credentials {
	return cliauth.Credentials{
		Issuer:       issuer,
		ClientID:     "dcr_" + issuer,
		AccessToken:  "at-" + issuer,
		RefreshToken: "rt-" + issuer,
		ExpiresAt:    time.Now().UTC().Add(time.Hour).Truncate(time.Second),
		Scope:        "skills.read skills.write",
	}
}

func TestHostFromIssuer(t *testing.T) {
	cases := []struct {
		issuer  string
		want    string
		wantErr bool
	}{
		{"http://localhost:8892", "localhost:8892", false},
		{"http://localhost:8892/", "localhost:8892", false},
		{"https://skills.example.com", "skills.example.com", false},
		{"HTTPS://Skills.Example.COM", "skills.example.com", false},
		{"https://skills.example.com:443", "skills.example.com:443", false},
		{"https://skills.example.com/oauth/authorize", "skills.example.com", false},
		{"not a url", "", true},
		{"", "", true},
		{"file:///etc/passwd", "", true},
		{"https://", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.issuer, func(t *testing.T) {
			got, err := cliauth.HostFromIssuer(tc.issuer)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("HostFromIssuer(%q) = %q, want %q", tc.issuer, got, tc.want)
			}
		})
	}
}

func TestCredStore_SaveHost_RoundTrip(t *testing.T) {
	store := cliauth.NewCredStore(filepath.Join(t.TempDir(), "auth.json"))
	creds := sampleCreds("http://localhost:8892")

	if err := store.SaveHost("localhost:8892", creds); err != nil {
		t.Fatalf("SaveHost: %v", err)
	}
	got, err := store.LoadHost("localhost:8892")
	if err != nil {
		t.Fatalf("LoadHost: %v", err)
	}
	if !reflect.DeepEqual(got, creds) {
		t.Fatalf("round-trip mismatch:\n in=%+v\nout=%+v", creds, got)
	}
}

func TestCredStore_MultiHostLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	store := cliauth.NewCredStore(path)

	if err := store.SaveHost("b.example.com", sampleCreds("https://b.example.com")); err != nil {
		t.Fatalf("save b: %v", err)
	}
	if err := store.SaveHost("a.example.com", sampleCreds("https://a.example.com")); err != nil {
		t.Fatalf("save a: %v", err)
	}

	host, _, err := store.LoadCurrent()
	if err != nil {
		t.Fatalf("LoadCurrent: %v", err)
	}
	if host != "b.example.com" {
		t.Fatalf("current = %q, want first saved host", host)
	}
	hosts, err := store.Hosts()
	if err != nil {
		t.Fatalf("Hosts: %v", err)
	}
	if want := []string{"a.example.com", "b.example.com"}; !reflect.DeepEqual(hosts, want) {
		t.Fatalf("Hosts() = %v, want %v", hosts, want)
	}
	if err := store.UseHost("a.example.com"); err != nil {
		t.Fatalf("UseHost: %v", err)
	}
	if err := store.DeleteHost("a.example.com"); err != nil {
		t.Fatalf("DeleteHost: %v", err)
	}
	host, _, err = store.LoadCurrent()
	if err != nil {
		t.Fatalf("LoadCurrent after delete: %v", err)
	}
	if host != "b.example.com" {
		t.Fatalf("current after delete = %q, want b.example.com", host)
	}
	if err := store.DeleteHost("b.example.com"); err != nil {
		t.Fatalf("DeleteHost last: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, stat err = %v", err)
	}
}

func TestCredStore_LoadMissingReturnsErrNotLoggedIn(t *testing.T) {
	store := cliauth.NewCredStore(filepath.Join(t.TempDir(), "missing.json"))

	if _, err := store.LoadHost("a.example.com"); !errors.Is(err, cliauth.ErrNotLoggedIn) {
		t.Fatalf("LoadHost err = %v, want ErrNotLoggedIn", err)
	}
	if _, _, err := store.LoadCurrent(); !errors.Is(err, cliauth.ErrNotLoggedIn) {
		t.Fatalf("LoadCurrent err = %v, want ErrNotLoggedIn", err)
	}
	hosts, err := store.Hosts()
	if err != nil {
		t.Fatalf("Hosts: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("Hosts() on missing file = %v, want empty", hosts)
	}
}

func TestCredStore_LoadCurrent_RejectsOldSingleHostShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	oldShape := sampleCreds("http://localhost:8892")
	data, err := json.MarshalIndent(oldShape, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write old-shape file: %v", err)
	}

	store := cliauth.NewCredStore(path)
	_, _, err = store.LoadCurrent()
	if !errors.Is(err, cliauth.ErrNotLoggedIn) {
		t.Fatalf("err = %v, want ErrNotLoggedIn", err)
	}
}

func TestCredStore_SaveHost_FileIsMode0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	store := cliauth.NewCredStore(path)

	if err := store.SaveHost("a", cliauth.Credentials{Issuer: "http://x", ClientID: "c"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("file mode = %o, want 0600", mode)
	}
}

func TestCredentials_NeedsRefresh(t *testing.T) {
	cases := []struct {
		name      string
		expiresIn time.Duration
		want      bool
	}{
		{"expired", -time.Hour, true},
		{"inside buffer", 10 * time.Second, true},
		{"outside buffer", 31 * time.Second, false},
		{"zero time", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			creds := cliauth.Credentials{}
			if tc.expiresIn != 0 {
				creds.ExpiresAt = time.Now().UTC().Add(tc.expiresIn)
			}
			if got := creds.NeedsRefresh(); got != tc.want {
				t.Fatalf("NeedsRefresh() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDefaultCredPath_UsesIssuerScopedMCPKitPath(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", "/should/not/be/used")

	got, err := cliauth.DefaultCredPath("https://skills.example.com/")
	if err != nil {
		t.Fatalf("DefaultCredPath: %v", err)
	}
	sum := sha256.Sum256([]byte("https://skills.example.com"))
	want := filepath.Join(configHome, "mcp-kit", hex.EncodeToString(sum[:]), "credentials.json")
	if got != want {
		t.Fatalf("DefaultCredPath() = %q, want %q", got, want)
	}
}

func TestDefaultCredPath_FallsBackToHomeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	got, err := cliauth.DefaultCredPath("http://localhost:8892")
	if err != nil {
		t.Fatalf("DefaultCredPath: %v", err)
	}
	sum := sha256.Sum256([]byte("http://localhost:8892"))
	want := filepath.Join(home, ".config", "mcp-kit", hex.EncodeToString(sum[:]), "credentials.json")
	if got != want {
		t.Fatalf("DefaultCredPath() = %q, want %q", got, want)
	}
}
