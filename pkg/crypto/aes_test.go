package crypto

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testKey is a deterministic 32-byte key for unit tests (exactly 32 bytes).
var testKey = []byte("thittam-test-key-32bytes-xxxxxxx")

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	t.Parallel()
	plaintext := "super-secret-oauth-client-secret"

	enc, err := Encrypt(testKey, plaintext)
	require.NoError(t, err)
	assert.NotEmpty(t, enc)
	assert.NotEqual(t, plaintext, enc)

	got, err := Decrypt(testKey, enc)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)
}

func TestEncrypt_ProducesUniqueNonces(t *testing.T) {
	t.Parallel()
	// Two encryptions of the same plaintext must differ (random nonce).
	a, err := Encrypt(testKey, "same")
	require.NoError(t, err)
	b, err := Encrypt(testKey, "same")
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
}

func TestEncrypt_EmptyPlaintext(t *testing.T) {
	t.Parallel()
	enc, err := Encrypt(testKey, "")
	require.NoError(t, err)
	got, err := Decrypt(testKey, enc)
	require.NoError(t, err)
	assert.Equal(t, "", got)
}

func TestEncrypt_InvalidKeyLength(t *testing.T) {
	t.Parallel()
	_, err := Encrypt([]byte("short"), "plaintext")
	assert.ErrorIs(t, err, ErrInvalidKeyLength)
}

func TestDecrypt_InvalidKeyLength(t *testing.T) {
	t.Parallel()
	enc, _ := Encrypt(testKey, "plaintext")
	_, err := Decrypt([]byte("short"), enc)
	assert.ErrorIs(t, err, ErrInvalidKeyLength)
}

func TestDecrypt_WrongKey(t *testing.T) {
	t.Parallel()
	enc, err := Encrypt(testKey, "secret")
	require.NoError(t, err)

	otherKey := []byte("thittam-othr-key-32bytes-xxxxxxx")
	_, err = Decrypt(otherKey, enc)
	assert.ErrorIs(t, err, ErrDecryptionFailed)
}

func TestDecrypt_Tampered(t *testing.T) {
	t.Parallel()
	enc, err := Encrypt(testKey, "secret")
	require.NoError(t, err)

	// Flip the last character of the base64url string.
	tampered := enc[:len(enc)-1] + strings.Map(func(r rune) rune {
		if r == 'A' {
			return 'B'
		}
		return 'A'
	}, string(enc[len(enc)-1]))

	_, err = Decrypt(testKey, tampered)
	assert.ErrorIs(t, err, ErrDecryptionFailed)
}

func TestDecrypt_TooShort(t *testing.T) {
	t.Parallel()
	_, err := Decrypt(testKey, "dG9vc2hvcnQ") // base64url("tooshort")
	assert.ErrorIs(t, err, ErrDecryptionFailed)
}

func TestDecrypt_InvalidBase64(t *testing.T) {
	t.Parallel()
	_, err := Decrypt(testKey, "not-valid-base64!!!")
	assert.ErrorIs(t, err, ErrDecryptionFailed)
}
