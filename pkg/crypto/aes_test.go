package crypto

import (
	"encoding/base64"
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

	// Round-trip through base64 and flip a byte we know is part of the
	// ciphertext payload (not the auth tag's structure or any padding bits
	// that base64.RawURLEncoding would silently strip on decode). Byte 0
	// is the first nonce byte — always 12 bytes of nonce live at the head.
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	require.NoError(t, err)
	require.Greater(t, len(raw), 0)
	raw[0] ^= 0x01
	tampered := base64.RawURLEncoding.EncodeToString(raw)

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
