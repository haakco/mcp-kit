package oauth

import "golang.org/x/crypto/bcrypt"

// DefaultBcryptCost is the bcrypt work factor used for new hashes.
const DefaultBcryptCost = 12

// HashSecret returns a bcrypt hash of secret.
func HashSecret(secret string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), DefaultBcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifySecret reports whether secret matches hash.
func VerifySecret(hash string, secret string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret))
}

// HashPassword returns a bcrypt hash of password.
func HashPassword(password string) (string, error) {
	return HashSecret(password)
}

// VerifyPassword reports whether password matches hash.
func VerifyPassword(hash string, password string) error {
	return VerifySecret(hash, password)
}

// NeedsRehash reports whether hash was produced below DefaultBcryptCost.
func NeedsRehash(hash string) bool {
	cost, err := bcrypt.Cost([]byte(hash))
	return err == nil && cost < DefaultBcryptCost
}
