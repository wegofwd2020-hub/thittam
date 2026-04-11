package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestBcryptHasher_Hash(t *testing.T) {
	t.Parallel()

	h := &BcryptHasher{cost: bcrypt.MinCost} // MinCost for fast tests

	hash, err := h.Hash("correct-horse-battery-staple")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, "correct-horse-battery-staple", hash)
}

func TestBcryptHasher_HashDifferentEachTime(t *testing.T) {
	t.Parallel()

	h := &BcryptHasher{cost: bcrypt.MinCost}
	hash1, err := h.Hash("password")
	require.NoError(t, err)
	hash2, err := h.Hash("password")
	require.NoError(t, err)
	// bcrypt salt is random — two hashes of the same password must differ.
	assert.NotEqual(t, hash1, hash2)
}

func TestBcryptHasher_EmptyPassword(t *testing.T) {
	t.Parallel()

	h := &BcryptHasher{cost: bcrypt.MinCost}
	// bcrypt allows empty passwords; the hasher should not error.
	hash, err := h.Hash("")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestBcryptVerifier_CorrectPassword(t *testing.T) {
	t.Parallel()

	h := &BcryptHasher{cost: bcrypt.MinCost}
	v := NewBcryptVerifier()

	hash, err := h.Hash("secret")
	require.NoError(t, err)

	assert.NoError(t, v.Verify("secret", hash))
}

func TestBcryptVerifier_WrongPassword(t *testing.T) {
	t.Parallel()

	h := &BcryptHasher{cost: bcrypt.MinCost}
	v := NewBcryptVerifier()

	hash, err := h.Hash("correct")
	require.NoError(t, err)

	err = v.Verify("wrong", hash)
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestBcryptVerifier_EmptyHashReturnError(t *testing.T) {
	t.Parallel()

	v := NewBcryptVerifier()
	err := v.Verify("password", "")
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}
