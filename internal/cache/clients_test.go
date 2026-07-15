package cache

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type ShopFixture struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

func newTestCacheClient(t *testing.T, pool *RefreshPool) (*CacheClient, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		require.NoError(t, rdb.Close())
	})
	return NewCacheClient(rdb, pool), mr
}

func TestGetOrLoad(t *testing.T) {
	t.Run("cache hit do not call loader", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)
		shopFixture := ShopFixture{ID: 1, Name: "cached"}
		shopFixtureBytes, err := json.Marshal(shopFixture)
		require.NoError(t, err)
		shopFixtureStr := string(shopFixtureBytes)
		mr.Set("shop:1", shopFixtureStr)

		got, found, err := GetOrLoad(
			context.Background(),
			client,
			"shop:1",
			time.Minute,
			time.Second,
			func(context.Context) (ShopFixture, bool, error) {
				t.Fatal("loader must not be called")
				return ShopFixture{}, false, nil
			},
		)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, ShopFixture{ID: 1, Name: "cached"}, got)
	})
	t.Run("not found", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)
		loadCount := 0

		key := "shop:999"

		got, found, err := GetOrLoad(
			context.Background(),
			client,
			key,
			time.Minute,
			time.Second*30,
			func(ctx context.Context) (string, bool, error) {
				loadCount++
				return "", false, nil
			},
		)
		require.NoError(t, err)
		require.False(t, found)
		require.Empty(t, got)
		require.Equal(t, 1, loadCount)

		cached, err := mr.Get(key)
		require.NoError(t, err)
		require.Equal(t, "", cached)
	})

	t.Run("null cache hit", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)
		loadCount := 0

		key := "shop:999"
		mr.Set(key, "")

		_, found, err := GetOrLoad(
			context.Background(),
			client,
			key,
			time.Minute,
			time.Second*30,
			func(ctx context.Context) (string, bool, error) {
				loadCount++
				return "should not be there", false, nil
			},
		)

		require.NoError(t, err)
		require.False(t, found)
		require.Equal(t, 0, loadCount)
	})
	t.Run("loader returns sentinel error", func(t *testing.T) {
		client, _ := newTestCacheClient(t, nil)
		expectedErr := errors.New("database connection timeout")
		// 缓存未命中 → 进入 loadAndCache → loader 返回 expectedErr
		// loadAndCache 中 loader 错误直接透传（无 %w 包裹），
		// 所以 require.ErrorIs 能直接匹配
		_, _, err := GetOrLoad(
			context.Background(),
			client,
			"shop:1",
			time.Minute,
			time.Second*30,
			func(ctx context.Context) (ShopFixture, bool, error) {
				return ShopFixture{}, false, expectedErr
			},
		)
		require.ErrorIs(t, err, expectedErr)
	})
	t.Run("found equal false return non zero should be zero", func(t *testing.T) {
		client, _ := newTestCacheClient(t, nil)
		shop, found, err := GetOrLoad(
			context.Background(),
			client,
			"shop:1",
			time.Minute,
			time.Second*30,
			func(ctx context.Context) (ShopFixture, bool, error) {
				return ShopFixture{ID: 1, Name: "测试商户"}, false, nil
			},
		)
		require.Zero(t, shop)
		require.Equal(t, found, false)
		require.NoError(t, err)
	})
	t.Run("found equal true redis should be zero", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)
		shop, found, err := GetOrLoad(
			context.Background(),
			client,
			"shop:1",
			time.Minute,
			time.Second*30,
			func(ctx context.Context) (ShopFixture, bool, error) {
				return ShopFixture{}, true, nil
			},
		)
		require.Zero(t, shop)
		require.Equal(t, found, true)
		require.NoError(t, err)

		cached, err := mr.Get("shop:1")
		require.JSONEq(t, `{"id":0,"name":""}`, cached)
		require.NoError(t, err)
	})
	t.Run("json marshal failed", func(t *testing.T) {
		client, _ := newTestCacheClient(t, nil)
		type UnsupportedFixture struct {
			Name string
			Ch   chan int
		}
		shop, found, err := GetOrLoad(
			context.Background(),
			client,
			"shop:1",
			time.Minute,
			time.Second*30,
			func(ctx context.Context) (UnsupportedFixture, bool, error) {
				return UnsupportedFixture{Name: "fail", Ch: make(chan int)}, true, nil
			},
		)
		require.Error(t, err)
		require.Zero(t, shop)
		require.Equal(t, found, false)
		require.Contains(t, err.Error(), "marshal cache")
	})
	t.Run("positive cache TTL expireation", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)

		key := "shop:1"
		ttl := 5 * time.Minute
		nullTTL := 10 * time.Second
		shop := ShopFixture{ID: 1, Name: "Active Shop"}

		var loaderCalled int
		got1, found1, err := GetOrLoad(
			context.Background(), client, key, ttl, nullTTL,
			func(ctx context.Context) (ShopFixture, bool, error) {
				loaderCalled++
				return shop, true, nil
			},
		)
		require.NoError(t, err)
		require.True(t, found1)
		require.Equal(t, got1, shop)
		require.Equal(t, 1, loaderCalled)

		got2, found2, err := GetOrLoad(
			context.Background(), client, key, ttl, nullTTL,
			func(context.Context) (ShopFixture, bool, error) {
				loaderCalled++
				return shop, true, nil
			},
		)
		require.NoError(t, err)
		require.True(t, found2)
		require.Equal(t, shop, got2)
		require.Equal(t, 1, loaderCalled)

		mr.FastForward(ttl + time.Second)
		got3, found3, err := GetOrLoad(
			context.Background(), client, key, ttl, nullTTL,
			func(context.Context) (ShopFixture, bool, error) {
				loaderCalled++
				return shop, true, nil
			},
		)
		require.NoError(t, err)
		require.True(t, found3)
		require.Equal(t, shop, got3)
		require.Equal(t, 2, loaderCalled)
	})
	t.Run("null cache TTL expiration", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)

		key := "shop:null:2"
		ttl := 5 * time.Minute
		nullTTL := 10 * time.Second

		var loaderCalled int
		got1, found1, err := GetOrLoad(
			context.Background(), client, key, ttl, nullTTL,
			func(context.Context) (ShopFixture, bool, error) {
				loaderCalled++
				return ShopFixture{}, false, nil
			},
		)
		require.NoError(t, err)
		require.Zero(t, got1)
		require.False(t, found1)
		require.Equal(t, 1, loaderCalled)
		got2, found2, err := GetOrLoad(
			context.Background(), client, key, ttl, nullTTL,
			func(context.Context) (ShopFixture, bool, error) {
				loaderCalled++
				return ShopFixture{}, false, nil
			},
		)
		require.NoError(t, err)
		require.Zero(t, got2)
		require.False(t, found2)
		require.Equal(t, 1, loaderCalled)
		mr.FastForward(nullTTL + time.Second)

		_, _, err = GetOrLoad(
			context.Background(), client, key, ttl, nullTTL,
			func(context.Context) (ShopFixture, bool, error) {
				loaderCalled++
				return ShopFixture{}, false, nil
			},
		)
		require.NoError(t, err)
		require.Equal(t, 2, loaderCalled)
	})
	t.Run("legacy json null", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)

		mr.Set("shop:999", "null")

		shop, found, err := GetOrLoad(
			context.Background(),
			client,
			"shop:999",
			time.Minute,
			time.Second*30,
			func(context.Context) (ShopFixture, bool, error) {
				t.Fatal("loader should not be called")
				return ShopFixture{}, false, nil
			},
		)

		require.NoError(t, err)
		require.False(t, found)
		require.Zero(t, shop)
	})
	t.Run("corrupted cache reload", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)
		mr.Set("shop:1", "{broken-json")

		loadCount := 0

		got, found, err := GetOrLoad(
			context.Background(),
			client,
			"shop:1",
			time.Minute,
			time.Second*30,
			func(context.Context) (ShopFixture, bool, error) {
				loadCount++
				return ShopFixture{ID: 1, Name: "测试商户"}, true, nil
			},
		)

		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, uint64(1), got.ID)
		require.Equal(t, 1, loadCount)

		cached, err := mr.Get("shop:1")
		require.NoError(t, err)
		require.JSONEq(t, `{"id":1,"name":"测试商户"}`, cached)
	})

	t.Run("redis error", func(t *testing.T) {
		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{
			Addr:         mr.Addr(),
			DialTimeout:  50 * time.Millisecond,
			ReadTimeout:  50 * time.Millisecond,
			WriteTimeout: 50 * time.Millisecond,
		})
		t.Cleanup(func() { _ = rdb.Close() })

		client := NewCacheClient(rdb, nil)
		mr.Close()
		loaderCalled := false

		_, _, err := GetOrLoad(
			context.Background(),
			client,
			"shop:1",
			time.Minute,
			time.Second*30,
			func(context.Context) (ShopFixture, bool, error) {
				loaderCalled = true
				return ShopFixture{}, false, nil
			},
		)

		require.Error(t, err)
		require.False(t, loaderCalled)
	})
}

func TestGetOrLoadWithMutex(t *testing.T) {
	t.Run("cache hit do not call loader", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)
		shop := ShopFixture{ID: 1, Name: "cached_shop"}
		bytes, _ := json.Marshal(shop)
		mr.Set("shop:1", string(bytes))

		got, found, err := GetOrLoadWithMutex(
			context.Background(), client, "shop:1", time.Minute, time.Second,
			func(context.Context) (ShopFixture, bool, error) {
				t.Fatal("loader must not be called")
				return ShopFixture{}, false, nil
			},
		)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, shop, got)
	})
	t.Run("cache miss loader executes once and writes cache", func(t *testing.T) {
		client, _ := newTestCacheClient(t, nil)
		var callCount int
		var mu sync.Mutex

		loader := func(context.Context) (ShopFixture, bool, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			return ShopFixture{ID: 2, Name: "db_shop"}, true, nil
		}

		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got, found, err := GetOrLoadWithMutex(context.Background(), client, "shop:2", time.Minute, time.Second, loader)
				require.NoError(t, err)
				require.True(t, found)
				require.Equal(t, ShopFixture{ID: 2, Name: "db_shop"}, got)
			}()
		}
		wg.Wait()

		require.Equal(t, 1, callCount, "loader should be executed exactly once")
	})
	t.Run("empty cache hit do not call loader", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)
		mr.Set("shop:3", "")

		got, found, err := GetOrLoadWithMutex(
			context.Background(), client, "shop:3", time.Minute, time.Second,
			func(context.Context) (ShopFixture, bool, error) {
				t.Fatal("loader must not be called for cached null/empty values")
				return ShopFixture{}, false, nil
			},
		)
		require.NoError(t, err)
		require.False(t, found)
		require.Equal(t, ShopFixture{}, got)
	})
	t.Run("corrupted JSON deleted and fallback to loader", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)
		mr.Set("shop:4", "{invalid-json")

		var loaderCalled bool
		got, found, err := GetOrLoadWithMutex(
			context.Background(), client, "shop:4", time.Minute, time.Second,
			func(context.Context) (ShopFixture, bool, error) {
				loaderCalled = true
				return ShopFixture{ID: 4, Name: "recovered_shop"}, true, nil
			},
		)
		require.NoError(t, err)
		require.True(t, found)
		require.True(t, loaderCalled)
		require.Equal(t, ShopFixture{ID: 4, Name: "recovered_shop"}, got)

		require.True(t, mr.Exists("shop:4"))
		val, _ := mr.Get("shop:4")
		require.Contains(t, val, "recovered_shop")
	})
	t.Run("5. mutex released when loader returns error", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)
		expectedErr := errors.New("db error")

		got, found, err := GetOrLoadWithMutex(
			context.Background(), client, "shop:5", time.Minute, time.Second,
			func(context.Context) (ShopFixture, bool, error) {
				return ShopFixture{}, false, expectedErr
			},
		)
		require.ErrorIs(t, err, expectedErr)
		require.False(t, found)
		require.Equal(t, ShopFixture{}, got)

		lockKey := "mutex:shop:5"
		require.False(t, mr.Exists(lockKey), "mutex lock should be released after loader error")
	})
	t.Run("6. respond to ctx.Done while waiting for lock", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)

		lockKey := "mutex:shop:6"
		mr.Set(lockKey, "other_worker")

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		start := time.Now()
		got, found, err := GetOrLoadWithMutex(
			ctx, client, "shop:6", time.Minute, time.Second,
			func(context.Context) (ShopFixture, bool, error) {
				return ShopFixture{ID: 6}, true, nil
			},
		)
		duration := time.Since(start)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.False(t, found)
		require.Equal(t, ShopFixture{}, got)
		require.Less(t, duration, 200*time.Millisecond, "should return early upon context cancellation")
	})
}

func TestGetOrLoadWithLogicalExpire(t *testing.T) {
	t.Run("cache miss sync load and write envelope", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)
		key := "shop:1"
		shop := ShopFixture{ID: 1, Name: "real_shop"}

		got, found, err := GetOrLoadWithLogicalExpire(
			context.Background(), client, key, time.Minute, time.Second,
			func(context.Context) (ShopFixture, bool, error) {
				return shop, true, nil
			},
		)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, shop, got)

		val, err := mr.Get(key)
		require.NoError(t, err)
		var envelope RedisData
		err = json.Unmarshal([]byte(val), &envelope)
		require.NoError(t, err)

		var cachedShop ShopFixture
		err = json.Unmarshal(envelope.Data, &cachedShop)
		require.NoError(t, err)
		require.Equal(t, shop, cachedShop)
		require.True(t, envelope.ExpireAt.After(time.Now()))
	})
	t.Run("fresh cache hit do not call loader", func(t *testing.T) {
		client, _ := newTestCacheClient(t, nil)
		key := "shop:2"
		shop := ShopFixture{ID: 2, Name: "fresh_shop"}

		err := SetWithLogicalExpire(client.rdb, context.Background(), key, shop, time.Minute)
		require.NoError(t, err)

		got, found, err := GetOrLoadWithLogicalExpire(
			context.Background(), client, key, time.Minute, time.Second,
			func(context.Context) (ShopFixture, bool, error) {
				t.Fatal("loader must not be called for fresh cache")
				return ShopFixture{}, false, nil
			},
		)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, shop, got)
	})
	t.Run("expired cache return old value and async refresh", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)
		key := "shop:3"
		oldShop := ShopFixture{ID: 3, Name: "old_shop"}
		newShop := ShopFixture{ID: 3, Name: "new_shop"}

		err := SetWithLogicalExpire(client.rdb, context.Background(), key, oldShop, -time.Minute)
		require.NoError(t, err)

		var wg sync.WaitGroup
		wg.Add(1)

		got, found, err := GetOrLoadWithLogicalExpire(
			context.Background(), client, key, time.Minute, time.Second,
			func(context.Context) (ShopFixture, bool, error) {
				defer wg.Done()
				return newShop, true, nil
			},
		)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, oldShop, got)

		wg.Wait()
		time.Sleep(10 * time.Millisecond)

		val, err := mr.Get(key)
		require.NoError(t, err)
		var envelope RedisData
		_ = json.Unmarshal([]byte(val), &envelope)
		var cachedShop ShopFixture
		_ = json.Unmarshal(envelope.Data, &cachedShop)
		require.Equal(t, newShop, cachedShop)
	})
	t.Run("multiple requests read same expired key only one loader runs", func(t *testing.T) {
		client, _ := newTestCacheClient(t, nil)
		key := "shop:4"
		oldShop := ShopFixture{ID: 4, Name: "old"}

		err := SetWithLogicalExpire(client.rdb, context.Background(), key, oldShop, -time.Minute)
		require.NoError(t, err)

		var loaderCallCount atomic.Int32 //!!!
		var loaderWg sync.WaitGroup
		loaderWg.Add(1)

		loaderBlock := make(chan struct{})
		loaderFunc := func(context.Context) (ShopFixture, bool, error) {
			loaderWg.Done()
			<-loaderBlock
			loaderCallCount.Add(1)
			return ShopFixture{ID: 4, Name: "new"}, true, nil
		}

		for i := 0; i < 5; i++ {
			got, found, err := GetOrLoadWithLogicalExpire(context.Background(), client, key, time.Minute, time.Second, loaderFunc)
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, oldShop, got)
		}

		close(loaderBlock)
		loaderWg.Wait()

		time.Sleep(20 * time.Millisecond)
		require.Equal(t, int32(1), loaderCallCount.Load())
	})
	t.Run("refresh lock already exists do not start loader", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)
		key := "shop:5"
		oldShop := ShopFixture{ID: 5, Name: "old"}

		err := SetWithLogicalExpire(client.rdb, context.Background(), key, oldShop, -time.Minute)
		require.NoError(t, err)

		mr.Set("mutex:"+key, "someone_already_lock")

		got, found, err := GetOrLoadWithLogicalExpire(
			context.Background(), client, key, time.Minute, time.Second,
			func(context.Context) (ShopFixture, bool, error) {
				t.Fatal("loader should not be called when lock already exists")
				return ShopFixture{}, false, nil
			},
		)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, oldShop, got)
	})
	t.Run("loader error retain old value and release lock", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)
		key := "shop:6"
		oldShop := ShopFixture{ID: 6, Name: "old"}

		err := SetWithLogicalExpire(client.rdb, context.Background(), key, oldShop, -time.Minute)
		require.NoError(t, err)

		var wg sync.WaitGroup
		wg.Add(1)

		got, found, err := GetOrLoadWithLogicalExpire(
			context.Background(), client, key, time.Minute, time.Second,
			func(context.Context) (ShopFixture, bool, error) {
				defer wg.Done()
				return ShopFixture{}, false, errors.New("db error")
			},
		)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, oldShop, got)

		wg.Wait()
		time.Sleep(10 * time.Millisecond)

		require.False(t, mr.Exists("mutex:"+key))

		val, err := mr.Get(key)
		require.NoError(t, err)
		var envelope RedisData
		_ = json.Unmarshal([]byte(val), &envelope)
		var cachedShop ShopFixture
		_ = json.Unmarshal(envelope.Data, &cachedShop)
		require.Equal(t, oldShop, cachedShop)
	})
	t.Run("loader returns found false write null cache", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)
		key := "shop:7"
		oldShop := ShopFixture{ID: 7, Name: "old"}

		err := SetWithLogicalExpire(client.rdb, context.Background(), key, oldShop, -time.Minute)
		require.NoError(t, err)

		var wg sync.WaitGroup
		wg.Add(1)

		got, found, err := GetOrLoadWithLogicalExpire(
			context.Background(), client, key, time.Minute, time.Second,
			func(context.Context) (ShopFixture, bool, error) {
				defer wg.Done()
				return ShopFixture{}, false, nil
			},
		)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, oldShop, got)

		wg.Wait()
		time.Sleep(10 * time.Millisecond)
		val, err := mr.Get(key)
		require.NoError(t, err)
		require.Equal(t, "", val)
	})
	t.Run("envelope corrupted delete and sync load", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)
		key := "shop:8"
		shop := ShopFixture{ID: 8, Name: "sync_load_shop"}

		mr.Set(key, "{invalid_json")

		got, found, err := GetOrLoadWithLogicalExpire(
			context.Background(), client, key, time.Minute, time.Second,
			func(context.Context) (ShopFixture, bool, error) {
				return shop, true, nil
			},
		)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, shop, got)
	})
	t.Run("internal Data corrupted delete and sync load", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)
		key := "shop:8_2"
		shop := ShopFixture{ID: 82, Name: "sync_load_shop"}

		badEnvelope := RedisData{
			Data:     json.RawMessage(`"corrupted_internal_string"`),
			ExpireAt: time.Now().Add(time.Minute),
		}
		bytes, _ := json.Marshal(badEnvelope)
		mr.Set(key, string(bytes))

		got, found, err := GetOrLoadWithLogicalExpire(
			context.Background(), client, key, time.Minute, time.Second,
			func(context.Context) (ShopFixture, bool, error) {
				return shop, true, nil
			},
		)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, shop, got)
	})
	t.Run("redis read error returns error", func(t *testing.T) {
		badRdb := redis.NewClient(&redis.Options{Addr: "localhost:12345"})
		defer badRdb.Close()
		client := NewCacheClient(badRdb, nil)

		_, _, err := GetOrLoadWithLogicalExpire(
			context.Background(), client, "shop:9", time.Minute, time.Second,
			func(context.Context) (ShopFixture, bool, error) {
				return ShopFixture{}, false, nil
			},
		)
		require.Error(t, err)
	})
	t.Run("refresh pool full release acquired lock", func(t *testing.T) {
		client, mr := newTestCacheClient(t, nil)
		pool := NewRefreshPool(client.rdb, 1, 1, 0)
		defer pool.Shutdown()
		client.refreshPool = pool

		// 1. 提交一个持久阻塞的 Job，占满 worker 和 channel 队列
		blockChan := make(chan struct{})
		pool.Submit(refreshJob{run: func() {
			<-blockChan
		}})
		// 再次提交一个 Job 以填满缓冲队列 (现在的 jobs channel 已满)
		pool.Submit(refreshJob{run: func() {}})

		// 2. 构造一个用于测试过期调度的 Key
		key := "shop:10"
		oldShop := ShopFixture{ID: 10, Name: "old"}
		err := SetWithLogicalExpire(client.rdb, context.Background(), key, oldShop, -time.Minute)
		require.NoError(t, err)

		// 3. 调用方法。此时由于缓存已逻辑过期，它会抢占 mutex 并尝试将其提交给已满的 Pool。
		// 预期：Submit 返回 false，触发立刻释放锁的分支。
		got, found, err := GetOrLoadWithLogicalExpire(
			context.Background(), client, key, time.Minute, time.Second,
			func(context.Context) (ShopFixture, bool, error) {
				return ShopFixture{ID: 10, Name: "never_reached"}, true, nil
			},
		)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, oldShop, got)

		// 释放阻塞通道，让 pool 恢复
		close(blockChan)

		// 确认抢占到临时的互斥锁已经从 Redis 中释放
		time.Sleep(20 * time.Millisecond)
		require.False(t, mr.Exists("mutex:"+key))
	})
}
