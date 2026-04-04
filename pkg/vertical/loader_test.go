package vertical

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDB implements the DB interface for testing.
type mockDB struct {
	data map[uuid.UUID][]byte
	err  error
}

func (m *mockDB) GetVerticalConfigForTenant(_ context.Context, tenantID uuid.UUID) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	data, ok := m.data[tenantID]
	if !ok {
		return nil, ErrNotFound
	}
	return data, nil
}

func setupTestLoader(t *testing.T, db DB) (*Loader, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	return NewLoader(rdb, db, nil), mr
}

func testConfigJSON(t *testing.T) (uuid.UUID, []byte) {
	t.Helper()
	tenantID := uuid.New()
	cfg := &Config{
		ID:      "test-vertical",
		Name:    "Test Vertical",
		Version: "1.0.0",
	}
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	return tenantID, data
}

func TestGetConfig_CacheHit(t *testing.T) {
	tenantID, data := testConfigJSON(t)
	db := &mockDB{data: map[uuid.UUID][]byte{tenantID: data}}
	loader, mr := setupTestLoader(t, db)

	// Pre-populate cache
	key := "vertical:config:tenant:" + tenantID.String()
	mr.Set(key, string(data))

	cfg, err := loader.GetConfig(context.Background(), tenantID)
	require.NoError(t, err)
	assert.Equal(t, "test-vertical", cfg.ID)
}

func TestGetConfig_CacheMiss_DBHit(t *testing.T) {
	tenantID, data := testConfigJSON(t)
	db := &mockDB{data: map[uuid.UUID][]byte{tenantID: data}}
	loader, mr := setupTestLoader(t, db)

	cfg, err := loader.GetConfig(context.Background(), tenantID)
	require.NoError(t, err)
	assert.Equal(t, "test-vertical", cfg.ID)

	// Wait briefly for async cache write
	time.Sleep(50 * time.Millisecond)

	// Verify it was cached
	key := "vertical:config:tenant:" + tenantID.String()
	assert.True(t, mr.Exists(key))
}

func TestGetConfig_CacheMiss_DBNotFound(t *testing.T) {
	tenantID := uuid.New()
	db := &mockDB{data: map[uuid.UUID][]byte{}}
	loader, _ := setupTestLoader(t, db)

	_, err := loader.GetConfig(context.Background(), tenantID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestGetConfig_DBError(t *testing.T) {
	tenantID := uuid.New()
	dbErr := errors.New("connection refused")
	db := &mockDB{err: dbErr}
	loader, _ := setupTestLoader(t, db)

	_, err := loader.GetConfig(context.Background(), tenantID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, dbErr))
}

func TestGetConfig_CorruptCache_FallsThroughToDB(t *testing.T) {
	tenantID, data := testConfigJSON(t)
	db := &mockDB{data: map[uuid.UUID][]byte{tenantID: data}}
	loader, mr := setupTestLoader(t, db)

	// Set corrupt data in cache
	key := "vertical:config:tenant:" + tenantID.String()
	mr.Set(key, "not-valid-json{{{")

	cfg, err := loader.GetConfig(context.Background(), tenantID)
	require.NoError(t, err)
	assert.Equal(t, "test-vertical", cfg.ID)
}

func TestInvalidate(t *testing.T) {
	tenantID, data := testConfigJSON(t)
	db := &mockDB{data: map[uuid.UUID][]byte{tenantID: data}}
	loader, mr := setupTestLoader(t, db)

	// Pre-populate cache
	key := "vertical:config:tenant:" + tenantID.String()
	mr.Set(key, string(data))
	assert.True(t, mr.Exists(key))

	err := loader.Invalidate(context.Background(), tenantID)
	require.NoError(t, err)
	assert.False(t, mr.Exists(key))
}

func TestNewLoader_NilLogger(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	loader := NewLoader(rdb, &mockDB{}, nil)
	assert.NotNil(t, loader.logger)
}
