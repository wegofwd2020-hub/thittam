package vertical

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	cacheTTL   = 5 * time.Minute
	cacheKeyFn = "vertical:config:tenant:%s"
)

// DB abstracts the database layer for loading vertical configs.
// GetVerticalConfigForTenant must return the fully-merged JSON
// (base config + tenant override) or an error wrapping ErrNotFound.
type DB interface {
	GetVerticalConfigForTenant(ctx context.Context, tenantID uuid.UUID) ([]byte, error)
}

// Logger defines structured logging for the vertical package.
// Adapters can bridge this to slog, zap, or any structured logger.
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// defaultLogger uses the standard log package.
type defaultLogger struct{}

func (defaultLogger) Info(msg string, kv ...interface{})  { log.Printf("[INFO] vertical: %s %v", msg, kv) }
func (defaultLogger) Warn(msg string, kv ...interface{})  { log.Printf("[WARN] vertical: %s %v", msg, kv) }
func (defaultLogger) Error(msg string, kv ...interface{}) { log.Printf("[ERROR] vertical: %s %v", msg, kv) }
func (defaultLogger) Debug(msg string, kv ...interface{}) { log.Printf("[DEBUG] vertical: %s %v", msg, kv) }

// Loader loads vertical configs with Redis caching and PostgreSQL fallback.
type Loader struct {
	redis  redis.Cmdable
	db     DB
	logger Logger
}

// NewLoader creates a Loader backed by Redis for caching and the DB for persistence.
func NewLoader(rdb redis.Cmdable, db DB, logger Logger) *Loader {
	if logger == nil {
		logger = defaultLogger{}
	}
	return &Loader{redis: rdb, db: db, logger: logger}
}

// GetConfig returns the vertical Config for the given tenant.
//
// Strategy: Redis cache (5-min TTL) → PostgreSQL → cache result → return.
// Returns ErrNotFound if the tenant has no registered vertical.
func (l *Loader) GetConfig(ctx context.Context, tenantID uuid.UUID) (*Config, error) {
	key := fmt.Sprintf(cacheKeyFn, tenantID)

	// 1. Try Redis cache
	cached, err := l.redis.Get(ctx, key).Bytes()
	if err == nil {
		var cfg Config
		if unmarshalErr := json.Unmarshal(cached, &cfg); unmarshalErr == nil {
			l.logger.Debug("config cache hit",
				"tenant_id", tenantID.String(),
				"vertical_id", cfg.ID,
			)
			return &cfg, nil
		}
		// Corrupt cache entry — log and fall through to DB
		l.logger.Warn("config cache corrupt, falling through to DB",
			"tenant_id", tenantID.String(),
		)
	}

	// 2. Load from DB
	data, err := l.db.GetVerticalConfigForTenant(ctx, tenantID)
	if err != nil {
		l.logger.Error("failed to load config from DB",
			"tenant_id", tenantID.String(),
			"error", err.Error(),
		)
		return nil, fmt.Errorf("vertical: load config for tenant %s: %w", tenantID, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		l.logger.Error("failed to unmarshal config from DB",
			"tenant_id", tenantID.String(),
			"error", err.Error(),
		)
		return nil, fmt.Errorf("vertical: unmarshal config for tenant %s: %w", tenantID, err)
	}

	// 3. Populate cache asynchronously (non-blocking)
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if setErr := l.redis.Set(cacheCtx, key, data, cacheTTL).Err(); setErr != nil {
			l.logger.Warn("failed to cache config",
				"tenant_id", tenantID.String(),
				"error", setErr.Error(),
			)
		}
	}()

	l.logger.Info("config loaded from DB",
		"tenant_id", tenantID.String(),
		"vertical_id", cfg.ID,
		"vertical_version", cfg.Version,
	)

	return &cfg, nil
}

// Invalidate removes the cached config for a tenant.
// Called when a tenant's vertical binding or override is updated.
func (l *Loader) Invalidate(ctx context.Context, tenantID uuid.UUID) error {
	key := fmt.Sprintf(cacheKeyFn, tenantID)
	if err := l.redis.Del(ctx, key).Err(); err != nil {
		l.logger.Error("failed to invalidate config cache",
			"tenant_id", tenantID.String(),
			"error", err.Error(),
		)
		return fmt.Errorf("vertical: invalidate cache for tenant %s: %w", tenantID, err)
	}

	l.logger.Info("config cache invalidated",
		"tenant_id", tenantID.String(),
	)
	return nil
}
