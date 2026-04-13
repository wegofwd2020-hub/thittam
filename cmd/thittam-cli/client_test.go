package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAdminClient_ValidEnv(t *testing.T) {
	t.Setenv("THITTAM_ADMIN_TOKEN", "test-token")

	client, err := NewAdminClient("local", "")
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8086", client.baseURL)
	assert.Equal(t, "test-token", client.token)
}

func TestNewAdminClient_ExplicitToken(t *testing.T) {
	client, err := NewAdminClient("staging", "my-token")
	require.NoError(t, err)
	assert.Equal(t, "https://api.staging.thittam.io", client.baseURL)
	assert.Equal(t, "my-token", client.token)
}

func TestNewAdminClient_InvalidEnv(t *testing.T) {
	_, err := NewAdminClient("invalid", "token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown environment")
}

func TestNewAdminClient_MissingToken(t *testing.T) {
	t.Setenv("THITTAM_ADMIN_TOKEN", "")
	_, err := NewAdminClient("local", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "admin token required")
}

func TestListVerticals_ParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/admin/v1/verticals", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		items := []VerticalListItem{
			{ID: "movie-production", Name: "Movie Production", Version: "1.0.0", IsActive: true},
			{ID: "software-development", Name: "Software Dev", Version: "1.0.0", IsActive: true},
		}
		_ = json.NewEncoder(w).Encode(items)
	}))
	defer server.Close()

	client := &AdminClient{
		baseURL:    server.URL,
		token:      "test-token",
		httpClient: server.Client(),
	}

	items, err := client.ListVerticals()
	require.NoError(t, err)
	assert.Len(t, items, 2)
	assert.Equal(t, "movie-production", items[0].ID)
}

func TestRegisterVertical_SendsPayload(t *testing.T) {
	var receivedPayload RegisterVerticalRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/admin/v1/verticals", r.URL.Path)

		_ = json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := &AdminClient{
		baseURL:    server.URL,
		token:      "test-token",
		httpClient: server.Client(),
	}

	err := client.RegisterVertical(RegisterVerticalRequest{
		ID:      "test-vertical",
		Name:    "Test",
		Version: "1.0.0",
		Config:  json.RawMessage(`{}`),
	}, false)
	require.NoError(t, err)
	assert.Equal(t, "test-vertical", receivedPayload.ID)
}

func TestRegisterVertical_Force(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "true", r.URL.Query().Get("force"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &AdminClient{
		baseURL:    server.URL,
		token:      "test-token",
		httpClient: server.Client(),
	}

	err := client.RegisterVertical(RegisterVerticalRequest{
		ID: "test", Config: json.RawMessage(`{}`),
	}, true)
	require.NoError(t, err)
}

func TestRegisterVertical_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"already exists"}`))
	}))
	defer server.Close()

	client := &AdminClient{
		baseURL:    server.URL,
		token:      "test-token",
		httpClient: server.Client(),
	}

	err := client.RegisterVertical(RegisterVerticalRequest{
		ID: "test", Config: json.RawMessage(`{}`),
	}, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "409")
}

func TestDeprecateVertical_CallsDelete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "DELETE", r.Method)
		assert.Equal(t, "/admin/v1/verticals/movie-production", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &AdminClient{
		baseURL:    server.URL,
		token:      "test-token",
		httpClient: server.Client(),
	}

	err := client.DeprecateVertical("movie-production")
	require.NoError(t, err)
}
