package auth

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/wegofwd2020/thittam/pkg/secrets"
)

// Verifier checks access-token signatures using only the RSA public key.
//
// It holds no private key and needs no Redis: access tokens are short-lived
// (DefaultAccessTTL) and are not revocable — only refresh tokens are, and those
// never reach a service other than iam. Every service can therefore verify a
// caller's token locally, with no network call and nothing worth stealing.
type Verifier struct {
	publicKey *rsa.PublicKey
}

// NewVerifier accepts PKIX ("PUBLIC KEY") and PKCS#1 ("RSA PUBLIC KEY") PEM.
// A private-key PEM is rejected: a service holding one is a misconfiguration
// severe enough that it must not start.
func NewVerifier(publicKeyPEM []byte) (*Verifier, error) {
	if len(publicKeyPEM) == 0 {
		return nil, errors.New("auth: verifier: public key PEM is empty")
	}
	block, _ := pem.Decode(publicKeyPEM)
	if block == nil {
		return nil, errors.New("auth: verifier: failed to decode PEM block")
	}

	switch block.Type {
	case "PUBLIC KEY": // PKIX
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("auth: verifier: parse PKIX key: %w", err)
		}
		rsaKey, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("auth: verifier: PKIX key is not RSA")
		}
		return &Verifier{publicKey: rsaKey}, nil

	case "RSA PUBLIC KEY": // PKCS#1
		key, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("auth: verifier: parse PKCS#1 key: %w", err)
		}
		return &Verifier{publicKey: key}, nil

	default:
		return nil, fmt.Errorf("auth: verifier: unsupported PEM block type %q "+
			"(a service must be given a PUBLIC key, never a private one)", block.Type)
	}
}

// Verify parses and verifies an access token, returning its claims.
// Returns ErrTokenExpired or ErrTokenInvalid — and nothing more specific,
// so a caller cannot use the error text as an oracle.
func (v *Verifier) Verify(accessToken string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		accessToken,
		&jwtClaims{},
		func(t *jwt.Token) (interface{}, error) {
			// Rejects alg:none and every HMAC algorithm. Without this, a token
			// signed with HS256 using the (public, therefore known) key bytes
			// as the shared secret would verify.
			if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("auth: verifier: unexpected signing method %v", t.Header["alg"])
			}
			return v.publicKey, nil
		},
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}

	c, ok := token.Claims.(*jwtClaims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}

	userID, err := uuid.Parse(c.Subject)
	if err != nil {
		return nil, ErrTokenInvalid
	}
	tenantID, err := uuid.Parse(c.TenantID)
	if err != nil {
		return nil, ErrTokenInvalid
	}

	// iat is not required (only jwt.WithExpirationRequired() is enforced), and
	// exp/IssuedAt/ExpiresAt are *jwt.NumericDate — nil-dereferencing here would
	// panic in the request path of a security boundary. exp is guaranteed
	// non-nil today because the parser rejects a missing exp, but that guard
	// still gets a nil check in case it is ever loosened.
	var issuedAt, expiresAt time.Time
	if c.IssuedAt != nil {
		issuedAt = c.IssuedAt.Time
	}
	if c.ExpiresAt != nil {
		expiresAt = c.ExpiresAt.Time
	}

	return &Claims{
		Subject:     userID,
		TenantID:    tenantID,
		Email:       c.Email,
		Roles:       c.Roles,
		Permissions: c.Permissions,
		AuthMethod:  c.AuthMethod,
		IssuedAt:    issuedAt,
		ExpiresAt:   expiresAt,
	}, nil
}

// VerifierFromEnv loads the JWT public key the same way cmd/iam selects its
// secret source: VAULT_ADDR present → Vault ("iam/jwt-public-key"); absent →
// files under IAM_KEY_DIR (default ./keys), name "jwt_public.pem".
//
// A service that cannot load the key must refuse to start. Returning an error
// here — rather than a permissive fallback — is what makes that true.
func VerifierFromEnv(ctx context.Context) (*Verifier, error) {
	var src secrets.Source
	var name string

	if addr := os.Getenv("VAULT_ADDR"); addr != "" {
		src = secrets.NewVaultSource(secrets.VaultConfig{
			Address:  addr,
			Mount:    getenvOr("VAULT_KV_MOUNT", "secret"),
			RoleID:   os.Getenv("VAULT_ROLE_ID"),
			SecretID: os.Getenv("VAULT_SECRET_ID"),
		})
		name = "iam/jwt-public-key"
	} else {
		src = secrets.NewFileSource(getenvOr("IAM_KEY_DIR", "./keys"))
		name = "jwt_public.pem"
	}

	loadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	pemBytes, err := src.GetSecret(loadCtx, name)
	if err != nil {
		return nil, fmt.Errorf("auth: verifier: load %s: %w", name, err)
	}
	return NewVerifier(pemBytes)
}

func getenvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
