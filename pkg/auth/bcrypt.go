package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// BcryptHasher hashes passwords with bcrypt at the default cost (10).
// It satisfies the PasswordHasher interface defined in services/iam (via
// structural typing — the method signatures match) and the auth.PasswordVerifier
// interface is on BcryptVerifier.
type BcryptHasher struct {
	cost int
}

// NewBcryptHasher creates a BcryptHasher with bcrypt's default cost (10).
// For tests where you need faster hashing, create with cost = bcrypt.MinCost.
func NewBcryptHasher() *BcryptHasher {
	return &BcryptHasher{cost: bcrypt.DefaultCost}
}

// Hash returns the bcrypt hash of the given plain-text password.
func (h *BcryptHasher) Hash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("auth: bcrypt hash: %w", err)
	}
	return string(b), nil
}

// BcryptVerifier checks passwords against bcrypt hashes.
// It satisfies auth.PasswordVerifier.
type BcryptVerifier struct{}

// NewBcryptVerifier creates a BcryptVerifier.
func NewBcryptVerifier() *BcryptVerifier { return &BcryptVerifier{} }

// Verify returns nil if password matches hash, or ErrInvalidCredentials otherwise.
// The bcrypt timing is constant — no timing oracle is introduced.
func (v *BcryptVerifier) Verify(password, hash string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}

// Compile-time interface checks.
var _ PasswordVerifier = (*BcryptVerifier)(nil)
