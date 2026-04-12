package auth

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// --- Argon2idHasher ---

func TestArgon2idHasher_Hash(t *testing.T) {
	t.Parallel()

	h := NewArgon2idHasher()
	hash, err := h.Hash("correct-horse-battery-staple")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(hash, "$argon2id$"), "hash must have argon2id prefix")
}

func TestArgon2idHasher_HashDifferentEachTime(t *testing.T) {
	t.Parallel()

	h := NewArgon2idHasher()
	hash1, err := h.Hash("password")
	require.NoError(t, err)
	hash2, err := h.Hash("password")
	require.NoError(t, err)
	// Random salt ensures two hashes of the same password are always different.
	assert.NotEqual(t, hash1, hash2)
}

func TestArgon2idHasher_EmptyPassword(t *testing.T) {
	t.Parallel()

	h := NewArgon2idHasher()
	hash, err := h.Hash("")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(hash, "$argon2id$"))
}

func TestArgon2idHasher_ContainsParams(t *testing.T) {
	t.Parallel()

	p := DefaultArgon2idParams()
	h := &Argon2idHasher{params: p}
	hash, err := h.Hash("test")
	require.NoError(t, err)
	// Encoded hash must include the parameter string.
	assert.Contains(t, hash, "m=65536")
	assert.Contains(t, hash, "t=2")
	assert.Contains(t, hash, "p=4")
}

// --- DualVerifier — argon2id path ---

func TestDualVerifier_Argon2id_Correct(t *testing.T) {
	t.Parallel()

	h := NewArgon2idHasher()
	v := NewDualVerifier()

	hash, err := h.Hash("secret123")
	require.NoError(t, err)

	require.NoError(t, v.Verify("secret123", hash))
}

func TestDualVerifier_Argon2id_Wrong(t *testing.T) {
	t.Parallel()

	h := NewArgon2idHasher()
	v := NewDualVerifier()

	hash, err := h.Hash("correct")
	require.NoError(t, err)

	err = v.Verify("wrong", hash)
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

// --- DualVerifier — bcrypt legacy path ---

func TestDualVerifier_Bcrypt_Correct(t *testing.T) {
	t.Parallel()

	// Simulate an existing bcrypt hash in the database.
	bcryptHash, err := bcrypt.GenerateFromPassword([]byte("legacy-password"), bcrypt.MinCost)
	require.NoError(t, err)

	v := NewDualVerifier()
	require.NoError(t, v.Verify("legacy-password", string(bcryptHash)))
}

func TestDualVerifier_Bcrypt_Wrong(t *testing.T) {
	t.Parallel()

	bcryptHash, err := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.MinCost)
	require.NoError(t, err)

	v := NewDualVerifier()
	err = v.Verify("wrong", string(bcryptHash))
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

// --- DualVerifier — unknown format ---

func TestDualVerifier_UnknownFormat(t *testing.T) {
	t.Parallel()

	v := NewDualVerifier()
	err := v.Verify("password", "not-a-valid-hash")
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestDualVerifier_EmptyHash(t *testing.T) {
	t.Parallel()

	v := NewDualVerifier()
	err := v.Verify("password", "")
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

// --- NeedsRehash ---

func TestNeedsRehash_Bcrypt(t *testing.T) {
	t.Parallel()

	bcryptHash, err := bcrypt.GenerateFromPassword([]byte("pass"), bcrypt.MinCost)
	require.NoError(t, err)

	assert.True(t, NeedsRehash(string(bcryptHash)), "bcrypt hashes always need rehash")
}

func TestNeedsRehash_Argon2id_CurrentParams(t *testing.T) {
	t.Parallel()

	h := NewArgon2idHasher()
	hash, err := h.Hash("pass")
	require.NoError(t, err)

	assert.False(t, NeedsRehash(hash), "argon2id hash with current params does not need rehash")
}

func TestNeedsRehash_Argon2id_WeakParams(t *testing.T) {
	t.Parallel()

	// Produce a hash with parameters below the current defaults.
	weakParams := Argon2idParams{Time: 1, Memory: 16 * 1024, Threads: 1, KeyLen: 32, SaltLen: 16}
	h := &Argon2idHasher{params: weakParams}
	hash, err := h.Hash("pass")
	require.NoError(t, err)

	assert.True(t, NeedsRehash(hash), "argon2id hash with weak params needs rehash")
}

func TestNeedsRehash_Unknown(t *testing.T) {
	t.Parallel()

	assert.False(t, NeedsRehash("not-a-hash"), "unknown format: do not rehash (fail closed)")
}

// --- decodeArgon2id round-trip ---

func TestDecodeArgon2id_RoundTrip(t *testing.T) {
	t.Parallel()

	h := NewArgon2idHasher()
	encoded, err := h.Hash("round-trip")
	require.NoError(t, err)

	hash, salt, params, err := decodeArgon2id(encoded)
	require.NoError(t, err)

	assert.Len(t, salt, int(DefaultArgon2idParams().SaltLen))
	assert.Len(t, hash, int(DefaultArgon2idParams().KeyLen))
	assert.Equal(t, DefaultArgon2idParams().Memory, params.Memory)
	assert.Equal(t, DefaultArgon2idParams().Time, params.Time)
	assert.Equal(t, DefaultArgon2idParams().Threads, params.Threads)
}

func TestDecodeArgon2id_InvalidFormat(t *testing.T) {
	t.Parallel()

	_, _, _, err := decodeArgon2id("$argon2id$not-valid")
	assert.Error(t, err)
}

func TestDecodeArgon2id_BadVersionFormat(t *testing.T) {
	t.Parallel()

	// "v=xyz" cannot be scanned as an integer — Sscanf returns error.
	_, _, _, err := decodeArgon2id("$argon2id$v=xyz$m=65536,t=2,p=4$c2FsdA$aGFzaA")
	assert.Error(t, err)
}

func TestDecodeArgon2id_UnsupportedVersion(t *testing.T) {
	t.Parallel()

	// Version 0 != argon2.Version (19) — must be rejected.
	_, _, _, err := decodeArgon2id("$argon2id$v=0$m=65536,t=2,p=4$c2FsdA$aGFzaA")
	assert.Error(t, err)
}

func TestDecodeArgon2id_BadParamsFormat(t *testing.T) {
	t.Parallel()

	// Params field is not in m=N,t=N,p=N format — Sscanf fails.
	_, _, _, err := decodeArgon2id("$argon2id$v=19$not-valid-params$c2FsdA$aGFzaA")
	assert.Error(t, err)
}

func TestDecodeArgon2id_BadSaltBase64(t *testing.T) {
	t.Parallel()

	// "!!!" is not valid base64url — DecodeString must fail.
	_, _, _, err := decodeArgon2id("$argon2id$v=19$m=65536,t=2,p=4$!!!$aGFzaA")
	assert.Error(t, err)
}

func TestDecodeArgon2id_BadHashBase64(t *testing.T) {
	t.Parallel()

	// Salt is valid but hash field contains invalid base64.
	validSalt := base64.RawStdEncoding.EncodeToString([]byte("0123456789012345"))
	_, _, _, err := decodeArgon2id("$argon2id$v=19$m=65536,t=2,p=4$" + validSalt + "$!!!")
	assert.Error(t, err)
}
