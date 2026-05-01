// Package keys manages OAuth JWT signing keys.
package keys

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/google/uuid"
)

// SigningAlgorithm is the JWS algorithm used for OAuth signing keys.
const SigningAlgorithm = "RS256"

// KeyUseSignature is the JWK "use" value indicating a signing key.
const KeyUseSignature = "sig"

// ErrNotFound indicates the requested signing key does not exist.
var ErrNotFound = errors.New("oauth keys: not found")

// SigningKey is the persisted representation of an OAuth signing key.
type SigningKey struct {
	KID           string
	Algorithm     string
	PrivateKeyPEM string
	PublicKeyPEM  string
	IsActive      bool
	CreatedAt     time.Time
	RetiredAt     *time.Time
}

// Store persists signing keys for Manager. Implementations must make
// RotateSigningKey atomic: the previous active key is retired and the
// replacement key is created as one storage operation.
type Store interface {
	FindActiveSigningKey(ctx context.Context) (SigningKey, error)
	CreateSigningKey(ctx context.Context, key SigningKey) (SigningKey, error)
	RotateSigningKey(ctx context.Context, replacement SigningKey, retiredAt time.Time) (SigningKey, error)
	ListVerifyingSigningKeys(ctx context.Context, now time.Time) ([]SigningKey, error)
	DeleteExpiredSigningKeys(ctx context.Context, now time.Time) (int, error)
}

// Manager handles OAuth signing key lifecycle.
type Manager struct {
	store Store
	now   func() time.Time
}

// NewManager creates a Manager backed by store.
func NewManager(store Store) *Manager {
	return &Manager{store: store, now: time.Now}
}

// EnsureSigningKey generates and persists a signing key if none exists.
func (m *Manager) EnsureSigningKey(ctx context.Context) (SigningKey, error) {
	existing, err := m.store.FindActiveSigningKey(ctx)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return SigningKey{}, fmt.Errorf("query active signing key: %w", err)
	}

	key, err := generateSigningKey()
	if err != nil {
		return SigningKey{}, err
	}

	created, err := m.store.CreateSigningKey(ctx, key)
	if err != nil {
		return SigningKey{}, fmt.Errorf("persist signing key: %w", err)
	}
	return created, nil
}

// GetActivePrivateKey loads the active signing key and returns the parsed RSA private key.
func (m *Manager) GetActivePrivateKey(ctx context.Context) (*rsa.PrivateKey, string, error) {
	key, err := m.store.FindActiveSigningKey(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("query active key: %w", err)
	}

	privateKey, err := parsePrivateKeyPEM(key.PrivateKeyPEM)
	if err != nil {
		return nil, "", err
	}
	return privateKey, key.KID, nil
}

// JWKS returns active and retired-but-still-verifying signing keys.
func (m *Manager) JWKS(ctx context.Context) (*jose.JSONWebKeySet, error) {
	signingKeys, err := m.store.ListVerifyingSigningKeys(ctx, m.now())
	if err != nil {
		return nil, fmt.Errorf("query signing keys: %w", err)
	}

	jwks := &jose.JSONWebKeySet{}
	for _, signingKey := range signingKeys {
		publicKey, err := parsePublicKeyPEM(signingKey.PublicKeyPEM)
		if err != nil {
			slog.Warn("jwks: skipping invalid signing key", "kid", signingKey.KID, "error", err)
			continue
		}

		jwks.Keys = append(jwks.Keys, jose.JSONWebKey{
			Key:       publicKey,
			KeyID:     signingKey.KID,
			Algorithm: signingKey.Algorithm,
			Use:       KeyUseSignature,
		})
	}
	return jwks, nil
}

// RotateSigningKey creates a new active key and retires the prior active key.
func (m *Manager) RotateSigningKey(ctx context.Context, grace time.Duration) (SigningKey, error) {
	key, err := generateSigningKey()
	if err != nil {
		return SigningKey{}, err
	}

	created, err := m.store.RotateSigningKey(ctx, key, m.now().Add(grace))
	if err != nil {
		return SigningKey{}, fmt.Errorf("rotate signing key: %w", err)
	}
	return created, nil
}

// RetireExpiredKeys deletes signing keys whose retired_at is in the past.
func (m *Manager) RetireExpiredKeys(ctx context.Context) (int, error) {
	deleted, err := m.store.DeleteExpiredSigningKeys(ctx, m.now())
	if err != nil {
		return 0, fmt.Errorf("delete expired signing keys: %w", err)
	}
	return deleted, nil
}

func generateSigningKey() (SigningKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return SigningKey{}, fmt.Errorf("generate RSA key: %w", err)
	}

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return SigningKey{}, fmt.Errorf("marshal public key: %w", err)
	}

	privatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	publicPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return SigningKey{
		KID:           uuid.NewString(),
		Algorithm:     SigningAlgorithm,
		PrivateKeyPEM: string(privatePEM),
		PublicKeyPEM:  string(publicPEM),
		IsActive:      true,
		CreatedAt:     time.Now(),
	}, nil
}

func parsePrivateKeyPEM(privateKeyPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, errors.New("decode private key PEM")
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	return privateKey, nil
}

func parsePublicKeyPEM(publicKeyPEM string) (any, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, errors.New("decode public key PEM")
	}
	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	return publicKey, nil
}
