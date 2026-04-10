package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// VaultConfig holds the configuration for connecting to HashiCorp Vault.
// All fields are populated from environment variables at service startup —
// AppRole credentials are lower-privilege credentials that unlock access to
// T1 secrets; they are acceptable as env vars (T3 classification).
type VaultConfig struct {
	// Address is the Vault server URL, e.g. "https://vault.internal:8200".
	// Source: VAULT_ADDR env var.
	Address string

	// Mount is the KV v2 secrets engine mount path, e.g. "secret".
	// Source: VAULT_KV_MOUNT env var (default: "secret").
	Mount string

	// RoleID and SecretID are Vault AppRole credentials scoped to this service.
	// They grant read-only access to a specific set of secret paths.
	// Source: VAULT_ROLE_ID and VAULT_SECRET_ID env vars.
	RoleID   string
	SecretID string

	// HTTPClient is optional. If nil, a default client with a 10s timeout is used.
	HTTPClient *http.Client
}

// vaultLoginResponse is the JSON envelope from POST /v1/auth/approle/login.
type vaultLoginResponse struct {
	Auth struct {
		ClientToken   string `json:"client_token"`
		LeaseDuration int    `json:"lease_duration"` // seconds
	} `json:"auth"`
}

// vaultSecretResponse is the JSON envelope from GET /v1/<mount>/data/<path>.
type vaultSecretResponse struct {
	Data struct {
		Data map[string]string `json:"data"`
	} `json:"data"`
}

// vaultErrorResponse is the error envelope from Vault.
type vaultErrorResponse struct {
	Errors []string `json:"errors"`
}

// VaultSource fetches secrets from HashiCorp Vault KV v2 using AppRole auth.
// It caches the client token and refreshes it before expiry.
//
// VaultSource implements both the Source interface and
// observability.HealthChecker — register it with the health server so /readyz
// reports Vault connectivity:
//
//	srv.Health().RegisterChecker("vault", vaultSrc)
type VaultSource struct {
	cfg    VaultConfig
	client *http.Client

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
}

// NewVaultSource creates a VaultSource. Call CheckHealth at startup to verify
// connectivity before the service begins serving traffic.
func NewVaultSource(cfg VaultConfig) *VaultSource {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &VaultSource{cfg: cfg, client: client}
}

// GetSecret fetches the secret at the given path from the Vault KV v2 engine.
// name is treated as the full path within the mount, e.g. "iam/jwt-private-key".
// The secret value stored under the key "value" at that path is returned.
//
// Token refresh is handled transparently: if the cached token has expired (or
// is within 60 seconds of expiry), a new AppRole login is performed before the
// secret read.
func (v *VaultSource) GetSecret(ctx context.Context, name string) ([]byte, error) {
	token, err := v.ensureToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("secrets: vault auth: %w", err)
	}

	url := fmt.Sprintf("%s/v1/%s/data/%s", v.cfg.Address, v.cfg.Mount, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("secrets: build vault request: %w", err)
	}
	req.Header.Set("X-Vault-Token", token)

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("secrets: vault request %s: %w", name, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("secrets: read vault response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", ErrSecretNotFound, name)
	}
	if resp.StatusCode != http.StatusOK {
		var errResp vaultErrorResponse
		_ = json.Unmarshal(body, &errResp)
		return nil, fmt.Errorf("secrets: vault returned %d for %s: %v", resp.StatusCode, name, errResp.Errors)
	}

	var secretResp vaultSecretResponse
	if err := json.Unmarshal(body, &secretResp); err != nil {
		return nil, fmt.Errorf("secrets: unmarshal vault response for %s: %w", name, err)
	}

	// Convention: the secret value is stored under the key "value" within the
	// KV entry. This is the standard pattern for single-value secrets.
	val, ok := secretResp.Data.Data["value"]
	if !ok {
		return nil, fmt.Errorf("%w: path %q has no 'value' key", ErrSecretNotFound, name)
	}

	return []byte(val), nil
}

// CheckHealth verifies connectivity to Vault by performing a token lookup.
// This implements observability.HealthChecker so it can be registered with
// the health server:
//
//	srv.Health().RegisterChecker("vault", vaultSrc)
//
// The service should not mark itself ready (/readyz) until this check passes.
func (v *VaultSource) CheckHealth(ctx context.Context) error {
	_, err := v.ensureToken(ctx)
	if err != nil {
		return fmt.Errorf("vault: %w", err)
	}
	return nil
}

// ensureToken returns a valid client token, performing an AppRole login if the
// cached token is absent or within 60 seconds of expiry.
func (v *VaultSource) ensureToken(ctx context.Context) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.token != "" && time.Until(v.tokenExpiry) > 60*time.Second {
		return v.token, nil
	}

	token, ttl, err := v.login(ctx)
	if err != nil {
		return "", err
	}

	v.token = token
	// Renew at 80% of lease duration to avoid races.
	v.tokenExpiry = time.Now().Add(time.Duration(float64(ttl)*0.8) * time.Second)
	return token, nil
}

// login performs an AppRole authentication and returns the client token and its
// lease duration in seconds.
func (v *VaultSource) login(ctx context.Context) (token string, leaseDuration int, err error) {
	payload, err := json.Marshal(map[string]string{
		"role_id":   v.cfg.RoleID,
		"secret_id": v.cfg.SecretID,
	})
	if err != nil {
		return "", 0, fmt.Errorf("marshal login payload: %w", err)
	}

	url := fmt.Sprintf("%s/v1/auth/approle/login", v.cfg.Address)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", 0, fmt.Errorf("build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("vault login request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read login response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp vaultErrorResponse
		_ = json.Unmarshal(body, &errResp)
		return "", 0, fmt.Errorf("vault login returned %d: %v", resp.StatusCode, errResp.Errors)
	}

	var loginResp vaultLoginResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return "", 0, fmt.Errorf("unmarshal login response: %w", err)
	}
	if loginResp.Auth.ClientToken == "" {
		return "", 0, fmt.Errorf("vault login returned empty token")
	}

	return loginResp.Auth.ClientToken, loginResp.Auth.LeaseDuration, nil
}

// Compile-time interface check.
var _ Source = (*VaultSource)(nil)
