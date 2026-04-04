package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// mockDBTX implements DBTX for unit testing without a real database.
type mockDBTX struct {
	queryRowFn func(ctx context.Context, query string, args ...interface{}) *sql.Row
}

func (m *mockDBTX) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return nil, nil
}

func (m *mockDBTX) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return nil, nil
}

func (m *mockDBTX) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	if m.queryRowFn != nil {
		return m.queryRowFn(ctx, query, args...)
	}
	// Return a row that will produce sql.ErrNoRows on scan.
	return emptyRow()
}

// emptyRow creates a *sql.Row that produces ErrNoRows.
// We open an in-memory connection for this purpose.
func emptyRow() *sql.Row {
	// We cannot easily create a *sql.Row without a driver.
	// Instead, we test via the Store integration path.
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
	desc := "Test description"
	params := UpsertVerticalDefinitionParams{
		ID:          "test-vertical",
		Name:        "Test Vertical",
		Version:     "1.0.0",
		Description: &desc,
		Config:      json.RawMessage(`{"entity_labels":{"project":"Test"}}`),
	}

	data, err := json.Marshal(params)
	require.NoError(t, err)

	var decoded UpsertVerticalDefinitionParams
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "test-vertical", decoded.ID)
	assert.Equal(t, "Test Vertical", decoded.Name)
	assert.Equal(t, &desc, decoded.Description)
}
