package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// mockDBTX implements the pgx/v5 DBTX interface for unit testing.
type mockDBTX struct{}

func (m *mockDBTX) Exec(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (m *mockDBTX) Query(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (m *mockDBTX) QueryRow(_ context.Context, _ string, _ ...interface{}) pgx.Row {
	return nil
}

func TestStore_ImplementsDBInterface(t *testing.T) {
	// Compile-time check that Store satisfies vertical.DB.
	var _ vertical.DB = (*Store)(nil)
}

func TestStore_NewStore(t *testing.T) {
	mock := &mockDBTX{}
	store := NewStore(mock)
	require.NotNil(t, store)
	require.NotNil(t, store.Queries())
}

func TestStore_QueriesNotNil(t *testing.T) {
	mock := &mockDBTX{}
	store := NewStore(mock)
	assert.NotNil(t, store.Queries())
}

func TestBindTenantVerticalParams_JSON(t *testing.T) {
	tenantID := uuid.New()
	registeredBy := uuid.New()
	params := BindTenantVerticalParams{
		TenantID:       tenantID,
		VerticalID:     "movie-production",
		ConfigOverride: json.RawMessage(`{"entity_labels":{"project":"Film"}}`),
		RegisteredBy:   registeredBy,
	}

	data, err := json.Marshal(params)
	require.NoError(t, err)

	var decoded BindTenantVerticalParams
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, tenantID, decoded.TenantID)
	assert.Equal(t, "movie-production", decoded.VerticalID)
	assert.Equal(t, registeredBy, decoded.RegisteredBy)
}

func TestUpsertVerticalDefinitionParams_JSON(t *testing.T) {
	params := UpsertVerticalDefinitionParams{
		ID:          "test-vertical",
		Name:        "Test Vertical",
		Version:     "1.0.0",
		Description: pgtype.Text{String: "Test description", Valid: true},
		Config:      json.RawMessage(`{"entity_labels":{"project":"Test"}}`),
	}

	data, err := json.Marshal(params)
	require.NoError(t, err)

	var decoded UpsertVerticalDefinitionParams
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "test-vertical", decoded.ID)
	assert.Equal(t, "Test Vertical", decoded.Name)
}
