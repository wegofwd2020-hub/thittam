// Package crypto provides AES-256-GCM authenticated encryption for secrets
// stored at rest (e.g. OIDC client secrets in tenant_auth_config).
//
// # Key requirements
//
// Keys must be exactly 32 bytes (AES-256). Keys are T1 secrets and must come
// from Vault or a gitignored local key file — never from environment variables.
//
// # Wire format
//
// Encrypt returns a URL-safe base64 string that encodes:
//
//	nonce (12 bytes) || ciphertext+tag (len(plaintext)+16 bytes)
//
// The GCM authentication tag is appended automatically by cipher.AEAD.Seal.
// Passing a ciphertext that has been tampered with returns ErrDecryptionFailed.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const (
	keyLen   = 32 // AES-256
	nonceLen = 12 // standard GCM nonce
)

// ErrDecryptionFailed is returned when the ciphertext is malformed or the key
// is wrong. Callers must not distinguish between these two cases — surfacing
// the distinction aids an attacker.
var ErrDecryptionFailed = errors.New("crypto: decryption failed")

// ErrInvalidKeyLength is returned when the key is not exactly 32 bytes.
var ErrInvalidKeyLength = errors.New("crypto: key must be exactly 32 bytes (AES-256)")

// Encrypt enciphers plaintext under key using AES-256-GCM with a random
// nonce and returns the result as a URL-safe base64 string (no padding).
// The key must be exactly 32 bytes.
func Encrypt(key []byte, plaintext string) (string, error) {
	if len(key) != keyLen {
		return "", ErrInvalidKeyLength
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: create GCM: %w", err)
	}

	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: generate nonce: %w", err)
	}

	// Seal appends the ciphertext and 16-byte authentication tag after the nonce.
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// Decrypt reverses Encrypt. Returns ErrDecryptionFailed if the ciphertext is
// malformed, truncated, or was produced with a different key.
func Decrypt(key []byte, encoded string) (string, error) {
	if len(key) != keyLen {
		return "", ErrInvalidKeyLength
	}

	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	// Minimum length: nonce (12) + GCM tag (16) = 28 bytes.
	if len(data) < nonceLen+16 {
		return "", ErrDecryptionFailed
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: create GCM: %w", err)
	}

	nonce, ciphertext := data[:nonceLen], data[nonceLen:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// Do not wrap — avoid leaking whether this was a tag mismatch or decode error.
		return "", ErrDecryptionFailed
	}

	return string(plaintext), nil
}
