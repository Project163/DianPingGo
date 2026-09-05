package cache

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redismock/v9"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

type shopFixture struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

func TestGetOrLoad_CacheHit_ReturnCachedValue(t *testing.T) {
	env := newCacheTestEnv(t)
	expected := shopFixture{ID: 1, Name: "cached"}
	encoded, err := json.Marshal(expected)
	assert.NoError(t, err)
	env.redis.Set("shop:1", string(encoded))

	actual, found, err := GetOrLoad(
		context.Background(), env.client, "shop", "shop:1",
		time.Minute, 30*time.Second,
		func(context.Context) (shopFixture, bool, error) {
			t.Fatal("loader must not be called on a cache hit")
			return shopFixture{}, false, nil
		},
	)

	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, expected, actual)
	assert.Equal(t, 1, env.metrics.value("cache_hit", "shop"))
}

func TestGetOrLoad_NullMarkerHit_ReturnNotFoundWithoutLoading(t *testing.T) {
	testCases := map[string]string{
		"empty_string":     "",
		"legacy_json_null": "null",
	}
	for name, marker := range testCases {
		t.Run(name, func(t *testing.T) {
			env := newCacheTestEnv(t)
			env.redis.Set("shop:missing", marker)

			actual, found, err := GetOrLoad(
				context.Background(), env.client, "shop", "shop:missing",
				time.Minute, 30*time.Second,
				func(context.Context) (shopFixture, bool, error) {
					t.Fatal("loader must not be called on a null-cache hit")
					return shopFixture{}, false, nil
				},
			)

			assert.NoError(t, err)
			assert.False(t, found)
			assert.Zero(t, actual)
			assert.Equal(t, 1, env.metrics.value("cache_hit", "shop"))
		})
	}
}

func TestGetOrLoad_CacheMissFound_ReturnDatabaseValueAndWriteBackAsync(t *testing.T) {
	env := newCacheTestEnv(t)
	expected := shopFixture{ID: 2, Name: "database"}

	actual, found, err := GetOrLoad(
		context.Background(), env.client, "shop", "shop:2",
		5*time.Minute, 30*time.Second,
		func(context.Context) (shopFixture, bool, error) {
			return expected, true, nil
		},
	)

	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, expected, actual)
	assert.Eventually(t, func() bool {
		cached, getErr := env.redis.Get("shop:2")
		if getErr != nil {
			return false
		}
		var decoded shopFixture
		return json.Unmarshal([]byte(cached), &decoded) == nil && decoded == expected
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, 1, env.metrics.value("cache_miss", "shop"))
	assert.Equal(t, 1, env.metrics.value("db_fallback", "shop", "cache_miss"))
	assert.Equal(t, 1, env.metrics.value("writeback_success", "shop", "value"))
}

func TestGetOrLoad_WriteBackWorkerBlocked_ReturnWithoutWaitingForRedis(t *testing.T) {
	env := newCacheTestEnv(t, withTestPoolSize(1, 2))
	blockerStarted := make(chan struct{})
	releaseBlocker := make(chan struct{})
	assert.True(t, env.client.runtime.refreshPool.Submit(refreshJob{run: func() {
		close(blockerStarted)
		<-releaseBlocker
	}}))
	<-blockerStarted

	actual, found, err := GetOrLoad(
		context.Background(), env.client, "shop", "shop:async",
		time.Minute, 30*time.Second,
		func(context.Context) (shopFixture, bool, error) {
			return shopFixture{ID: 20, Name: "database"}, true, nil
		},
	)

	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, shopFixture{ID: 20, Name: "database"}, actual)
	assert.False(t, env.redis.Exists("shop:async"))
	close(releaseBlocker)
	assert.Eventually(t, func() bool {
		return env.redis.Exists("shop:async")
	}, time.Second, 10*time.Millisecond)
}

func TestGetOrLoad_CacheMissNotFound_ReturnZeroAndWriteBackNullAsync(t *testing.T) {
	env := newCacheTestEnv(t)

	actual, found, err := GetOrLoad(
		context.Background(), env.client, "shop", "shop:missing",
		time.Minute, 20*time.Second,
		func(context.Context) (shopFixture, bool, error) {
			return shopFixture{ID: 99, Name: "must be discarded"}, false, nil
		},
	)

	assert.NoError(t, err)
	assert.False(t, found)
	assert.NotZero(t, actual)
	assert.Eventually(t, func() bool {
		cached, getErr := env.redis.Get("shop:missing")
		return getErr == nil && cached == ""
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, 1, env.metrics.value("writeback_success", "shop", "null"))
}

func TestGetOrLoad_LoaderError_PropagateWithoutWriteBack(t *testing.T) {
	env := newCacheTestEnv(t)
	expectedErr := errors.New("database unavailable")

	actual, found, err := GetOrLoad(
		context.Background(), env.client, "shop", "shop:3",
		time.Minute, 30*time.Second,
		func(context.Context) (shopFixture, bool, error) {
			return shopFixture{}, false, expectedErr
		},
	)

	assert.ErrorIs(t, err, expectedErr)
	assert.False(t, found)
	assert.Zero(t, actual)
	assert.False(t, env.redis.Exists("shop:3"))
}

func TestGetOrLoad_MarshalFailure_ReturnDatabaseValueAndRecordMetric(t *testing.T) {
	type unsupportedFixture struct {
		Name string
		Ch   chan int
	}
	env := newCacheTestEnv(t)
	expected := unsupportedFixture{Name: "database", Ch: make(chan int)}

	actual, found, err := GetOrLoad(
		context.Background(), env.client, "unsupported", "unsupported:1",
		time.Minute, 30*time.Second,
		func(context.Context) (unsupportedFixture, bool, error) {
			return expected, true, nil
		},
	)

	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, expected, actual)
	assert.False(t, env.redis.Exists("unsupported:1"))
	assert.Equal(t, 1, env.metrics.value("writeback_error", "unsupported", "value", "marshal_error"))
}

func TestGetOrLoad_RedisReadFailure_FallbackToDatabase(t *testing.T) {
	rdb, mock := redismock.NewClientMock()
	expectedErr := errors.New("redis read failed")
	mock.ExpectGet("shop:4").SetErr(expectedErr)
	client, metrics := newCacheTestClient(t, rdb, withoutTestRefreshPool())
	expected := shopFixture{ID: 4, Name: "database"}

	actual, found, err := GetOrLoad(
		context.Background(), client, "shop", "shop:4",
		time.Minute, 30*time.Second,
		func(context.Context) (shopFixture, bool, error) {
			return expected, true, nil
		},
	)

	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, expected, actual)
	assert.Equal(t, 1, metrics.value("redis_error", "shop", "get"))
	assert.Equal(t, 1, metrics.value("db_fallback", "shop", "redis_error"))
	assert.Equal(t, 1, metrics.value("writeback_dropped", "shop", "pool_unavailable"))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetOrLoad_NullWriteBackFailure_DoesNotChangeDatabaseResult(t *testing.T) {
	rdb, mock := redismock.NewClientMock()
	mock.ExpectGet("shop:missing").RedisNil()
	mock.ExpectSet("shop:missing", "", 30*time.Second).SetErr(errors.New("redis write failed"))
	client, metrics := newCacheTestClient(t, rdb)

	actual, found, err := GetOrLoad(
		context.Background(), client, "shop", "shop:missing",
		time.Minute, 30*time.Second,
		func(context.Context) (shopFixture, bool, error) {
			return shopFixture{}, false, nil
		},
	)

	assert.NoError(t, err)
	assert.False(t, found)
	assert.Zero(t, actual)
	assert.Eventually(t, func() bool {
		return metrics.value("writeback_error", "shop", "null", "redis_error") == 1
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, 1, metrics.value("redis_error", "shop", "set"))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetOrLoad_CorruptedValue_ReloadAndRepairCache(t *testing.T) {
	env := newCacheTestEnv(t)
	env.redis.Set("shop:5", "{broken-json")
	expected := shopFixture{ID: 5, Name: "reloaded"}

	actual, found, err := GetOrLoad(
		context.Background(), env.client, "shop", "shop:5",
		time.Minute, 30*time.Second,
		func(context.Context) (shopFixture, bool, error) {
			return expected, true, nil
		},
	)

	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, expected, actual)
	assert.Eventually(t, func() bool {
		cached, getErr := env.redis.Get("shop:5")
		if getErr != nil {
			return false
		}
		var decoded shopFixture
		return json.Unmarshal([]byte(cached), &decoded) == nil && decoded == expected
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, 1, env.metrics.value("cache_corrupted", "shop"))
	assert.Equal(t, 1, env.metrics.value("db_fallback", "shop", "corrupted"))
}

func TestDeleteCorruptedAsync_ValueChangedBeforeDelete_PreserveNewValue(t *testing.T) {
	env := newCacheTestEnv(t, withTestPoolSize(1, 4))
	blockerStarted := make(chan struct{})
	releaseBlocker := make(chan struct{})
	assert.True(t, env.client.runtime.refreshPool.Submit(refreshJob{run: func() {
		close(blockerStarted)
		<-releaseBlocker
	}}))
	<-blockerStarted

	env.redis.Set("shop:6", "{broken-json")
	env.client.deleteCorruptedAsync("shop", "shop:6", "{broken-json")
	env.redis.Set("shop:6", `{"id":6,"name":"newer"}`)
	deleteFinished := make(chan struct{})
	assert.True(t, env.client.runtime.refreshPool.Submit(refreshJob{run: func() {
		close(deleteFinished)
	}}))
	close(releaseBlocker)
	<-deleteFinished

	cached, err := env.redis.Get("shop:6")
	assert.NoError(t, err)
	assert.Equal(t, `{"id":6,"name":"newer"}`, cached)
}

func TestGetOrLoad_ConcurrentSameKey_LoadOnceAndShareResult(t *testing.T) {
	env := newCacheTestEnv(t)
	const callers = 12
	start := make(chan struct{})
	loaderStarted := make(chan struct{})
	releaseLoader := make(chan struct{})
	var loaderCalls atomic.Int32

	loader := func(context.Context) (shopFixture, bool, error) {
		if loaderCalls.Add(1) == 1 {
			close(loaderStarted)
		}
		<-releaseLoader
		return shopFixture{ID: 7, Name: "shared"}, true, nil
	}

	type result struct {
		value shopFixture
		found bool
		err   error
	}
	results := make(chan result, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	for range callers {
		go func() {
			ready.Done()
			<-start
			value, found, err := GetOrLoad(
				context.Background(), env.client, "shop", "shop:7",
				time.Minute, 30*time.Second, loader,
			)
			results <- result{value: value, found: found, err: err}
		}()
	}
	ready.Wait()
	close(start)
	<-loaderStarted
	assert.Eventually(t, func() bool {
		return env.metrics.value("cache_miss", "shop") == callers
	}, time.Second, 10*time.Millisecond)
	close(releaseLoader)

	for range callers {
		result := <-results
		assert.NoError(t, result.err)
		assert.True(t, result.found)
		assert.Equal(t, shopFixture{ID: 7, Name: "shared"}, result.value)
	}
	assert.Equal(t, int32(1), loaderCalls.Load())
	assert.GreaterOrEqual(t, env.metrics.value("singleflight_shared", "shop"), callers-1)
}

func TestGetOrLoad_ConcurrentDifferentKeys_RespectDatabaseConcurrencyLimit(t *testing.T) {
	policy := defaultCacheTestConfig().policy
	policy.MaxDBConcurrency = 2
	policy.DBAcquireTimeout = 2 * time.Second
	policy.LoaderTimeout = 3 * time.Second
	env := newCacheTestEnv(t, withTestReadPolicy(policy))
	const callers = 6
	start := make(chan struct{})
	releaseLoaders := make(chan struct{})
	var active atomic.Int32
	var maximum atomic.Int32
	results := make(chan error, callers)

	for i := range callers {
		key := "shop:concurrent:" + string(rune('a'+i))
		go func() {
			<-start
			_, _, err := GetOrLoad(
				context.Background(), env.client, "shop", key,
				time.Minute, 30*time.Second,
				func(context.Context) (shopFixture, bool, error) {
					current := active.Add(1)
					updateAtomicMaximum(&maximum, current)
					<-releaseLoaders
					active.Add(-1)
					return shopFixture{ID: 8}, true, nil
				},
			)
			results <- err
		}()
	}
	close(start)
	assert.Eventually(t, func() bool { return active.Load() == 2 }, time.Second, 10*time.Millisecond)
	close(releaseLoaders)

	for range callers {
		assert.NoError(t, <-results)
	}
	assert.Equal(t, int32(2), maximum.Load())
}

func TestGetOrLoad_DatabaseSemaphoreTimeout_ReturnOverloaded(t *testing.T) {
	policy := defaultCacheTestConfig().policy
	policy.MaxDBConcurrency = 1
	policy.DBAcquireTimeout = 20 * time.Millisecond
	env := newCacheTestEnv(t, withTestReadPolicy(policy))
	env.client.runtime.dbSlots <- struct{}{}
	t.Cleanup(func() { <-env.client.runtime.dbSlots })
	var loaderCalled atomic.Bool

	actual, found, err := GetOrLoad(
		context.Background(), env.client, "shop", "shop:overloaded",
		time.Minute, 30*time.Second,
		func(context.Context) (shopFixture, bool, error) {
			loaderCalled.Store(true)
			return shopFixture{}, false, nil
		},
	)

	assert.ErrorIs(t, err, ErrDBFallbackOverloaded)
	assert.False(t, found)
	assert.Zero(t, actual)
	assert.False(t, loaderCalled.Load())
	assert.Equal(t, 1, env.metrics.value("db_fallback_rejected", "semaphore_timeout"))
}

func TestGetOrLoad_CanceledRequest_IgnoreBreakerResult(t *testing.T) {
	env := newCacheTestEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	actual, found, err := GetOrLoad(
		ctx, env.client, "shop", "shop:canceled",
		time.Minute, 30*time.Second,
		func(context.Context) (shopFixture, bool, error) {
			t.Fatal("loader must not run after request cancellation")
			return shopFixture{}, false, nil
		},
	)

	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, found)
	assert.Zero(t, actual)
	assert.Equal(t, uint64(0), env.client.runtime.breaker.total)
	assert.Equal(t, uint64(0), env.client.runtime.breaker.failures)
}

func TestGetOrLoad_BreakerOpen_BypassRedisAndUseProtectedLoader(t *testing.T) {
	rdb, mock := redismock.NewClientMock()
	breaker := NewRedisBreaker(BreakerConfig{
		Window:          time.Minute,
		MinimumRequests: 1,
		FailureRatio:    1,
		OpenDuration:    time.Minute,
	})
	permit, allowed := breaker.Allow()
	assert.True(t, allowed)
	permit.Done(false)
	client, metrics := newCacheTestClient(t, rdb, withoutTestRefreshPool(), withTestBreaker(breaker))

	actual, found, err := GetOrLoad(
		context.Background(), client, "shop", "shop:bypass",
		time.Minute, 30*time.Second,
		func(context.Context) (shopFixture, bool, error) {
			return shopFixture{ID: 9, Name: "database"}, true, nil
		},
	)

	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, shopFixture{ID: 9, Name: "database"}, actual)
	assert.Equal(t, 1, metrics.value("redis_bypass", "shop", "breaker_open"))
	assert.Equal(t, 1, metrics.value("db_fallback", "shop", "breaker_open"))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetOrLoad_NullMarkerExpired_LoadAgain(t *testing.T) {
	env := newCacheTestEnv(t)
	const nullTTL = 10 * time.Second
	var loaderCalls atomic.Int32
	loader := func(context.Context) (shopFixture, bool, error) {
		loaderCalls.Add(1)
		return shopFixture{}, false, nil
	}

	_, found, err := GetOrLoad(
		context.Background(), env.client, "shop", "shop:expired-null",
		time.Minute, nullTTL, loader,
	)
	assert.NoError(t, err)
	assert.False(t, found)
	assert.Eventually(t, func() bool {
		return env.redis.Exists("shop:expired-null")
	}, time.Second, 10*time.Millisecond)

	env.redis.FastForward(nullTTL + time.Second)
	_, found, err = GetOrLoad(
		context.Background(), env.client, "shop", "shop:expired-null",
		time.Minute, nullTTL, loader,
	)
	assert.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, int32(2), loaderCalls.Load())
}

func TestGetOrLoadWithMutex_CacheHit_ReturnCachedValue(t *testing.T) {
	env := newCacheTestEnv(t)
	expected := shopFixture{ID: 10, Name: "cached"}
	encoded, err := json.Marshal(expected)
	assert.NoError(t, err)
	env.redis.Set("shop:10", string(encoded))

	actual, found, err := GetOrLoadWithMutex(
		context.Background(), env.client, "shop:10", time.Minute, 30*time.Second,
		func(context.Context) (shopFixture, bool, error) {
			t.Fatal("loader must not be called on a cache hit")
			return shopFixture{}, false, nil
		},
	)

	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, expected, actual)
}

func TestGetOrLoadWithLogicalExpire_ExpiredValue_ReturnStaleAndRefreshAsync(t *testing.T) {
	env := newCacheTestEnv(t)
	stale := shopFixture{ID: 11, Name: "stale"}
	fresh := shopFixture{ID: 11, Name: "fresh"}
	assert.NoError(t, SetWithLogicalExpire(env.client.rdb, context.Background(), "shop:11", stale, -time.Second))

	actual, found, err := GetOrLoadWithLogicalExpire(
		context.Background(), env.client, "shop:11", time.Minute, 30*time.Second,
		func(context.Context) (shopFixture, bool, error) {
			return fresh, true, nil
		},
	)

	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, stale, actual)
	assert.Eventually(t, func() bool {
		value, valueFound, hit, expired, readErr := readLogicalCache[shopFixture](
			context.Background(), env.client, "shop:11",
		)
		return readErr == nil && hit && valueFound && !expired && value == fresh
	}, time.Second, 10*time.Millisecond)
}

type cacheTestEnv struct {
	client  *CacheClient
	redis   *miniredis.Miniredis
	metrics *recordingMetrics
}

type cacheTestConfig struct {
	createPool bool
	workers    int
	queueSize  int
	policy     ReadPolicy
	breaker    *RedisBreaker
}

type cacheTestOption func(*cacheTestConfig)

func newCacheTestEnv(t *testing.T, options ...cacheTestOption) *cacheTestEnv {
	t.Helper()
	miniRedis := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr:         miniRedis.Addr(),
		DialTimeout:  100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
		WriteTimeout: 100 * time.Millisecond,
		MaxRetries:   0,
	})
	t.Cleanup(func() { assert.NoError(t, rdb.Close()) })
	client, metrics := newCacheTestClient(t, rdb, options...)
	return &cacheTestEnv{client: client, redis: miniRedis, metrics: metrics}
}

func newCacheTestClient(
	t *testing.T,
	rdb redis.Cmdable,
	options ...cacheTestOption,
) (*CacheClient, *recordingMetrics) {
	t.Helper()
	config := defaultCacheTestConfig()
	for _, option := range options {
		option(&config)
	}

	metrics := newRecordingMetrics()
	var pool *RefreshPool
	if config.createPool {
		pool = NewRefreshPool(rdb, config.workers, config.queueSize, 0)
		t.Cleanup(pool.Shutdown)
	}
	if config.breaker == nil {
		config.breaker = NewRedisBreaker(BreakerConfig{
			Window:          time.Minute,
			MinimumRequests: 100,
			FailureRatio:    0.5,
			OpenDuration:    time.Second,
		})
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runtime := NewReadRuntime(pool, config.breaker, logger, metrics, config.policy)
	return NewCacheClient(rdb, pool, runtime), metrics
}

func defaultCacheTestConfig() cacheTestConfig {
	return cacheTestConfig{
		createPool: true,
		workers:    2,
		queueSize:  32,
		policy: ReadPolicy{
			RedisReadTimeout:  100 * time.Millisecond,
			RedisWriteTimeout: 200 * time.Millisecond,
			LoaderTimeout:     2 * time.Second,
			DBAcquireTimeout:  300 * time.Millisecond,
			MaxDBConcurrency:  20,
		},
	}
}

func withoutTestRefreshPool() cacheTestOption {
	return func(config *cacheTestConfig) {
		config.createPool = false
	}
}

func withTestPoolSize(workers, queueSize int) cacheTestOption {
	return func(config *cacheTestConfig) {
		config.createPool = true
		config.workers = workers
		config.queueSize = queueSize
	}
}

func withTestReadPolicy(policy ReadPolicy) cacheTestOption {
	return func(config *cacheTestConfig) {
		config.policy = policy
	}
}

func withTestBreaker(breaker *RedisBreaker) cacheTestOption {
	return func(config *cacheTestConfig) {
		config.breaker = breaker
	}
}

type recordingMetrics struct {
	mu       sync.Mutex
	counters map[string]int
}

func newRecordingMetrics() *recordingMetrics {
	return &recordingMetrics{counters: make(map[string]int)}
}

func (m *recordingMetrics) increment(parts ...string) {
	m.mu.Lock()
	m.counters[metricKey(parts...)]++
	m.mu.Unlock()
}

func (m *recordingMetrics) value(parts ...string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.counters[metricKey(parts...)]
}

func (m *recordingMetrics) IncCacheHit(cacheName string) {
	m.increment("cache_hit", cacheName)
}

func (m *recordingMetrics) IncCacheMiss(cacheName string) {
	m.increment("cache_miss", cacheName)
}

func (m *recordingMetrics) IncCacheCorrupted(cacheName string) {
	m.increment("cache_corrupted", cacheName)
}

func (m *recordingMetrics) IncRedisError(cacheName, operation string) {
	m.increment("redis_error", cacheName, operation)
}

func (m *recordingMetrics) IncRedisBypass(cacheName, reason string) {
	m.increment("redis_bypass", cacheName, reason)
}

func (m *recordingMetrics) IncSingleflightShared(cacheName string) {
	m.increment("singleflight_shared", cacheName)
}

func (m *recordingMetrics) IncDBFallback(cacheName, reason string) {
	m.increment("db_fallback", cacheName, reason)
}

func (m *recordingMetrics) IncDBFallbackRejected(reason string) {
	m.increment("db_fallback_rejected", reason)
}

func (m *recordingMetrics) ObserveDBFallbackDuration(cacheName string, _ time.Duration) {
	m.increment("db_fallback_duration", cacheName)
}

func (m *recordingMetrics) IncWritebackSuccess(cacheName, kind string) {
	m.increment("writeback_success", cacheName, kind)
}

func (m *recordingMetrics) IncWritebackError(cacheName, kind, reason string) {
	m.increment("writeback_error", cacheName, kind, reason)
}

func (m *recordingMetrics) IncWritebackDropped(cacheName, reason string) {
	m.increment("writeback_dropped", cacheName, reason)
}

func metricKey(parts ...string) string {
	var key string
	for _, part := range parts {
		key += "\x00" + part
	}
	return key
}

func updateAtomicMaximum(maximum *atomic.Int32, candidate int32) {
	for {
		current := maximum.Load()
		if candidate <= current || maximum.CompareAndSwap(current, candidate) {
			return
		}
	}
}
