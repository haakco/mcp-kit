package oauth_test

import (
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/haakco/mcp-kit/oauth"
)

func TestPasswordHashVerifyAndRehash(t *testing.T) {
	hash, err := oauth.HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if err := oauth.VerifyPassword(hash, "correct horse battery staple"); err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if err := oauth.VerifyPassword(hash, "wrong"); err == nil {
		t.Fatal("VerifyPassword() accepted wrong password")
	}
	if oauth.NeedsRehash(hash) {
		t.Fatal("NeedsRehash() = true for freshly generated hash")
	}

	oldHash, err := bcrypt.GenerateFromPassword([]byte("secret"), oauth.DefaultBcryptCost-1)
	if err != nil {
		t.Fatalf("GenerateFromPassword() error = %v", err)
	}
	if !oauth.NeedsRehash(string(oldHash)) {
		t.Fatal("NeedsRehash() = false for lower-cost hash")
	}
}
