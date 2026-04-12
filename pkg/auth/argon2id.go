package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// argon2idPrefix is the standard encoded hash prefix for argon2id.
const argon2idPrefix = "$argon2id$"

// bcryptPrefixes covers all bcrypt version prefixes used in practice.
var bcryptPrefixes = []string{"$2a$", "$2b$", "$2y$", "$2x$"}

// Argon2idParams holds the cost parameters for argon2id.
// OWASP minimum recommended values: time=2, memory=64MB, threads=4.
type Argon2idParams struct {
	Time    uint32
	Memory  uint32 // in KiB
	Threads uint8
	KeyLen  uint32
	SaltLen uint32
}

// DefaultArgon2idParams returns OWASP-minimum argon2id parameters.
func DefaultArgon2idParams() Argon2idParams {
	return Argon2idParams{
		Time:    2,
		Memory:  64 * 1024, // 64 MiB
		Threads: 4,
		KeyLen:  32,
		SaltLen: 16,
	}
}

// Argon2idHasher hashes passwords with argon2id.
// It satisfies the iam.PasswordHasher interface.
type Argon2idHasher struct {
	params Argon2idParams
}

// NewArgon2idHasher creates an Argon2idHasher with the default OWASP parameters.
func NewArgon2idHasher() *Argon2idHasher {
	return &Argon2idHasher{params: DefaultArgon2idParams()}
}

// Hash returns an encoded argon2id hash of the given plain-text password.
// The hash is self-describing and includes salt, algorithm, and parameters.
// Format: $argon2id$v=19$m=<memory>,t=<time>,p=<threads>$<salt_b64>$<hash_b64>
func (h *Argon2idHasher) Hash(password string) (string, error) {
	salt := make([]byte, h.params.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: argon2id: generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, h.params.Time, h.params.Memory, h.params.Threads, h.params.KeyLen)
	return encodeArgon2id(hash, salt, h.params), nil
}

// encodeArgon2id formats an argon2id hash into the standard PHC string format.
func encodeArgon2id(hash, salt []byte, p Argon2idParams) string {
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Time, p.Threads, b64Salt, b64Hash)
}

// DualVerifier verifies passwords against argon2id hashes (primary) and
// bcrypt hashes (legacy). It is used during the gradual migration period
// where existing users still have bcrypt hashes in the database.
//
// Call NeedsRehash after a successful Verify to determine whether to upgrade
// the stored hash to argon2id.
type DualVerifier struct{}

// NewDualVerifier creates a DualVerifier.
func NewDualVerifier() *DualVerifier { return &DualVerifier{} }

// Verify returns nil if password matches hash, or ErrInvalidCredentials otherwise.
// Routes to argon2id or bcrypt based on the hash prefix.
func (v *DualVerifier) Verify(password, hash string) error {
	switch {
	case strings.HasPrefix(hash, argon2idPrefix):
		return verifyArgon2id(password, hash)
	case isBcryptHash(hash):
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
			return ErrInvalidCredentials
		}
		return nil
	default:
		// Unknown format — fail closed.
		return ErrInvalidCredentials
	}
}

// NeedsRehash returns true if the stored hash uses an algorithm or parameters
// that should be upgraded. Currently returns true for any bcrypt hash, and for
// argon2id hashes whose parameters are weaker than the current defaults.
func NeedsRehash(hash string) bool {
	if isBcryptHash(hash) {
		return true
	}
	if !strings.HasPrefix(hash, argon2idPrefix) {
		return false
	}
	_, _, params, err := decodeArgon2id(hash)
	if err != nil {
		return false
	}
	def := DefaultArgon2idParams()
	// Upgrade if stored params are below current minimums.
	return params.Time < def.Time || params.Memory < def.Memory || params.Threads < def.Threads
}

// verifyArgon2id parses an encoded argon2id hash and verifies the password.
func verifyArgon2id(password, encoded string) error {
	hash, salt, params, err := decodeArgon2id(encoded)
	if err != nil {
		return ErrInvalidCredentials
	}
	candidate := argon2.IDKey([]byte(password), salt, params.Time, params.Memory, params.Threads, params.KeyLen)
	if subtle.ConstantTimeCompare(hash, candidate) != 1 {
		return ErrInvalidCredentials
	}
	return nil
}

// decodeArgon2id parses a PHC-formatted argon2id string.
// Expected format: $argon2id$v=<v>$m=<m>,t=<t>,p=<p>$<salt_b64>$<hash_b64>
func decodeArgon2id(encoded string) (hash, salt []byte, params Argon2idParams, err error) {
	parts := strings.Split(encoded, "$")
	// parts: ["", "argon2id", "v=19", "m=...,t=...,p=...", "<salt>", "<hash>"]
	if len(parts) != 6 {
		return nil, nil, params, fmt.Errorf("auth: argon2id: invalid hash format")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return nil, nil, params, fmt.Errorf("auth: argon2id: parse version: %w", err)
	}
	if version != argon2.Version {
		return nil, nil, params, fmt.Errorf("auth: argon2id: unsupported version %d", version)
	}

	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.Memory, &params.Time, &params.Threads); err != nil {
		return nil, nil, params, fmt.Errorf("auth: argon2id: parse params: %w", err)
	}

	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, params, fmt.Errorf("auth: argon2id: decode salt: %w", err)
	}

	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, params, fmt.Errorf("auth: argon2id: decode hash: %w", err)
	}

	params.KeyLen = uint32(len(hash))
	params.SaltLen = uint32(len(salt))
	return hash, salt, params, nil
}

// isBcryptHash returns true if the encoded string is a bcrypt hash.
func isBcryptHash(hash string) bool {
	for _, prefix := range bcryptPrefixes {
		if strings.HasPrefix(hash, prefix) {
			return true
		}
	}
	return false
}

// Compile-time interface checks.
var _ PasswordVerifier = (*DualVerifier)(nil)
