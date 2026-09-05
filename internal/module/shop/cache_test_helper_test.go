package shop

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"dianping/internal/cache"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func newModuleTestCacheClient(t *testing.T, rdb redis.Cmdable) *cache.CacheClient {
	t.Helper()
	pool := cache.NewRefreshPool(rdb, 2, 32, 0)
	t.Cleanup(pool.Shutdown)
	return newModuleTestCacheClientWithPool(rdb, pool)
}

func newModuleTestCacheClientWithoutPool(t *testing.T, rdb redis.Cmdable) *cache.CacheClient {
	t.Helper()
	return newModuleTestCacheClientWithPool(rdb, nil)
}

func newModuleTestCacheClientWithPool(rdb redis.Cmdable, pool *cache.RefreshPool) *cache.CacheClient {
	runtime := cache.NewReadRuntime(
		pool,
		cache.NewRedisBreaker(cache.BreakerConfig{
			Window:          time.Minute,
			MinimumRequests: 100,
			FailureRatio:    0.5,
			OpenDuration:    time.Second,
		}),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		cache.NoopMetrics{},
		cache.ReadPolicy{},
	)
	return cache.NewCacheClient(rdb, pool, runtime)
}

func waitForModuleCacheValue(t *testing.T, miniRedis *miniredis.Miniredis, key string) string {
	t.Helper()
	var value string
	assert.Eventually(t, func() bool {
		cached, err := miniRedis.Get(key)
		if err != nil {
			return false
		}
		value = cached
		return true
	}, time.Second, 10*time.Millisecond)
	return value
}
