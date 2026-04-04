package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// API base URLs per environment.
var envURLs = map[string]string{
	"local":      "http://localhost:8086",
	"staging":    "https://api.staging.thittam.io",
	"production": "https://api.thittam.io",
}

// AdminClient calls the Thittam admin API.
type AdminClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewAdminClient creates a client for the given environment.
func NewAdminClient(env, token string) (*AdminClient, error) {
	baseURL, ok := envURLs[env]
	if !ok {
		return nil, fmt.Errorf("unknown environment %q (valid: local, staging, production)", env)
	}

	if token == "" {
		token = os.Getenv("THITTAM_ADMIN_TOKEN")
	}
	if token == "" {
		return nil, fmt.Errorf("admin token required: use --token or set THITTAM_ADMIN_TOKEN")
	}

	return &AdminClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// RegisterVerticalRequest is the payload for POST /admin/v1/verticals.
type RegisterVerticalRequest struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Config      json.RawMessage `json:"config"`
}

// VerticalListItem is a single row from GET /admin/v1/verticals.
type VerticalListItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	IsActive    bool   `json:"is_active"`
}

// RegisterVertical calls POST /admin/v1/verticals.
func (c *AdminClient) RegisterVertical(req RegisterVerticalRequest, force bool) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + "/admin/v1/verticals"
	if force {
		url += "?force=true"
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// ListVerticals calls GET /admin/v1/verticals.
func (c *AdminClient) ListVerticals() ([]VerticalListItem, error) {
	httpReq, err := http.NewRequest("GET", c.baseURL+"/admin/v1/verticals", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var items []VerticalListItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return items, nil
}

// DeprecateVertical calls DELETE /admin/v1/verticals/{id}.
func (c *AdminClient) DeprecateVertical(id string) error {
	httpReq, err := http.NewRequest("DELETE", c.baseURL+"/admin/v1/verticals/"+id, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
