package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrDataNotFound = errors.New("cache: data not found in database")

type RedisData struct {
	Data     json.RawMessage `json:"data"`
	ExpireAt time.Time       `json:"expire_at"`
}

type CacheClient struct {
	rdb         redis.Cmdable
	runtime     *ReadRuntime
	refreshPool *RefreshPool
	jitterRatio float64
}

func NewCacheClient(rdb redis.Cmdable, refreshPool *RefreshPool, runtime *ReadRuntime) *CacheClient {
	return &CacheClient{
		rdb:         rdb,
		runtime:     runtime,
		refreshPool: refreshPool,
		jitterRatio: 0.2,
	}
}

func mutexKey(key string) string {
	return "mutex:" + key
}

func (c *CacheClient) Del(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

// GetOrLoad 查询缓存；未命中时调用 loader 回源
func GetOrLoad[T any](
	ctx context.Context,
	client *CacheClient,
	cacheName string,
	key string,
	ttl time.Duration,
	nullTTL time.Duration,
	loader func(context.Context) (T, bool, error),
) (T, bool, error) {
	var zero T
	// 获取全局Runtime
	rt := client.runtime

	// 熔断前置判断，快速失败的机制入口
	permit, allowed := rt.breaker.Allow()
	// 如果熔断已经打开，完全不会调用redis，直接进入DB查询部分
	if !allowed {
		rt.metrics.IncRedisBypass(cacheName, "breaker_open")
		return loadProtected(
			ctx, client, cacheName, key, ttl, nullTTL, "breaker_open", loader,
		)
	}

	// 使用请求Context的子Context作为Redis查询的Context
	// 请求截止时间比RedisReadTimeout更短时以请求截止时间为准
	readCtx, cancel := context.WithTimeout(
		ctx,
		rt.policy.RedisReadTimeout,
	)
	val, err := client.rdb.Get(readCtx, key).Result()
	cancel()

	// 请求取消不等于Redis成功或故障，不该将其记作true成功
	// TODO：增加permit.Ignore()，对于普通请求不增加total/failures，半开指针只释放probing，不关闭熔断器，让其下一次调用重新探测
	if ctx.Err() != nil {
		permit.Ignore()
		return zero, false, ctx.Err()
	}

	switch {
	case err == nil:
		// Redis命中，熔断器记作操作成功
		permit.Done(true)

		// 命中空值缓存，返回found=false
		trimmed := bytes.TrimSpace([]byte(val))
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			rt.metrics.IncCacheHit(cacheName)
			return zero, false, nil
		}

		// 命中正常数据
		var result T
		if err := json.Unmarshal(trimmed, &result); err == nil {
			rt.metrics.IncCacheHit(cacheName)
			return result, true, nil
		}

		// 命中但为不可解析值
		rt.metrics.IncCacheCorrupted(cacheName)
		rt.logger.ErrorContext(
			ctx,
			"corrupted cache value",
			"cache_name", cacheName,
			"key", key,
			"err", err,
		)
		// 异步尝试删除坏Key
		client.deleteCorruptedAsync(cacheName, key, val)
		return loadProtected(
			ctx, client, cacheName, key,
			ttl, nullTTL, "corrupted", loader,
		)
	case errors.Is(err, redis.Nil):
		// Redis未命中，熔断器记作操作成功
		permit.Done(true)
		rt.metrics.IncCacheMiss(cacheName)

		// 直接进入DB查询环节
		return loadProtected(
			ctx, client, cacheName, key,
			ttl, nullTTL, "cache_miss", loader,
		)
	default:
		// Redis出现非预期错误，熔断器记作操作失败
		permit.Done(false)
		rt.metrics.IncRedisError(cacheName, "get")
		rt.logger.ErrorContext(
			ctx,
			"redis cache read failed, falling back to database",
			"cache_name", cacheName,
			"key", key,
			"error", err,
		)

		// 直接进入DB查询阶段
		return loadProtected(
			ctx, client, cacheName, key,
			ttl, nullTTL, "redis_error", loader,
		)
	}
}

type loadResult[T any] struct {
	value T
	found bool
}

// DB查询和回源
func loadProtected[T any](
	ctx context.Context,
	client *CacheClient,
	cacheName string,
	key string,
	ttl time.Duration,
	nullTTL time.Duration,
	reason string,
	loader func(context.Context) (T, bool, error),
) (T, bool, error) {
	var zero T
	rt := client.runtime

	// 相同flightKey的并发请求合并，cacheName的加入并非必要，key已经足够区分不同缓存类型，只是为了防御性编程
	flightKey := cacheName + ":" + key

	// DoChan返回一个结果channel，同key的调用只有一个函数实际执行，其他调用共享其结果
	// 与Do不同的是，Do直接等待返回结果，因此即使请求的ctx已经取消，调用Do的goroutine仍旧会阻塞在Do中直到查询完成
	// 而DoChan返回的结果Channel，调用方可以通过select在ctx取消时提前返回该请求，防止无意义等待
	resultCh := rt.flights.DoChan(flightKey, func() (any, error) {
		// singlefilght是多个任务的公共请求，不能完全依赖第一个请求的取消信号
		// 因此通过WithoutCancel先保留原始context的value，再移除第一个请求的取消和deadline，在附加独立的LoaderTimeout
		baseCtx := context.WithoutCancel(ctx)
		loadCtx, cancel := context.WithTimeout(
			baseCtx,
			rt.policy.LoaderTimeout,
		)
		defer cancel()

		// 高并发中的请求只有一个占用DB信号量，持有信号量的DB才能执行loader
		if err := rt.acquireDB(loadCtx); err != nil {
			return nil, err
		}
		defer rt.releaseDB()
		rt.metrics.IncDBFallback(cacheName, reason)

		// 进入实际DB查询
		started := time.Now()
		data, found, err := loader(loadCtx)
		rt.metrics.ObserveDBFallbackDuration(cacheName, time.Since(started))
		if err != nil {
			return nil, err
		}

		// DB回源写回缓存，在多个并发请求下也只会执行一次
		client.scheduleWriteback(ctx, cacheName, key, data, found, ttl, nullTTL)
		return loadResult[T]{
			value: data,
			found: found,
		}, nil
	})
	select {
	case <-ctx.Done():
		return zero, false, ctx.Err()

	case result := <-resultCh:
		if result.Shared {
			// shared表示该结果被多个调用共享
			rt.metrics.IncSingleflightShared(cacheName)
		}
		if result.Err != nil {
			return zero, false, result.Err
		}

		loaded, ok := result.Val.(loadResult[T])
		if !ok {
			return zero, false, errors.New(
				"cache: unexpected singleflight result type",
			)
		}

		return loaded.value, loaded.found, nil
	}
}

// 异步DB回填至Redis（回填不影响DB返回的事实源数据）
func (c *CacheClient) scheduleWriteback(
	ctx context.Context,
	cacheName string,
	key string,
	data any,
	found bool,
	ttl time.Duration,
	nullTTL time.Duration,
) {
	rt := c.runtime

	var (
		payload  any
		writeTTL time.Duration
		kind     string
	)

	// 将序列化从提取出来，普通数据和空值缓存共用一套写入逻辑
	if !found {
		payload = ""
		writeTTL = nullTTL
		kind = "null"
	} else {
		encoded, err := json.Marshal(data)
		if err != nil {
			rt.metrics.IncWritebackError(cacheName, "value", "marshal_error")
			rt.logger.Error(
				"cache value marshal failed",
				"cache_name", cacheName,
				"key", key,
				"error", err,
			)
			return
		}

		if bytes.Equal(bytes.TrimSpace(encoded), []byte("null")) {
			rt.metrics.IncWritebackError(cacheName, "value", "unexpected_null")
			rt.logger.Error(
				"cache loader returned found=true with null value",
				"cache_name", cacheName,
				"key", key,
			)
			return
		}

		payload = encoded
		writeTTL = ttl
		kind = "value"
	}

	// 回填的回调，通过协程池异步执行
	run := func() {
		// 依旧需要先判断redis是否处于熔断状态
		permit, allowed := rt.breaker.Allow()
		if !allowed {
			rt.metrics.IncWritebackDropped(cacheName, "breaker_open")
			return
		}

		// 独立的写回用ctx
		baseCtx := context.WithoutCancel(ctx)
		writeCtx, cancel := context.WithTimeout(baseCtx, rt.policy.RedisWriteTimeout)
		defer cancel()

		err := c.rdb.Set(writeCtx, key, payload, writeTTL).Err()
		permit.Done(err == nil)
		if err != nil {
			rt.metrics.IncRedisError(cacheName, "set")
			rt.metrics.IncWritebackError(cacheName, kind, "redis_error")
			rt.logger.Error(
				"cache writeback failed",
				"cache_name", cacheName,
				"key", key,
				"kind", kind,
				"error", err,
			)
			return
		}

		rt.metrics.IncWritebackSuccess(cacheName, kind)
	}

	// 协程池未启动时，直接放弃，缓存回写并不影响业务请求的稳定性
	if rt.refreshPool == nil {
		rt.metrics.IncWritebackDropped(cacheName, "pool_unavailable")
		rt.logger.Error(
			"cache writeback dropped: refresh pool unavailable",
			"cache_name", cacheName,
			"key", key,
		)
		return
	}

	// 协程池满时，同样直接放弃
	if !rt.refreshPool.Submit(refreshJob{run: run}) {
		rt.metrics.IncWritebackDropped(cacheName, "queue_full")
		rt.logger.Warn(
			"cache writeback dropped: queue full",
			"cache_name", cacheName,
			"key", key,
		)
	}
}

const deleteLua = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
	return redis.call('DEL', KEYS[1])
end
return 0
`

// 坏Key（数据无法被JSON格式化）的异步删除
func (c *CacheClient) deleteCorruptedAsync(
	cacheName string,
	key string,
	observeValue string,
) {
	rt := c.runtime

	run := func() {
		permit, allowed := rt.breaker.Allow()
		if !allowed {
			rt.metrics.IncRedisBypass(
				cacheName,
				"delete_corrupted_breaker_open",
			)
			return
		}

		deleteCtx, cancel := context.WithTimeout(
			context.Background(),
			rt.policy.RedisWriteTimeout,
		)
		defer cancel()

		err := c.rdb.Eval(deleteCtx, deleteLua, []string{key}, observeValue).Err()
		permit.Done(err == nil)

		if err != nil {
			rt.metrics.IncRedisError(
				cacheName,
				"delete_corrupted",
			)
			rt.logger.Error(
				"failed to delete corrupted cache value",
				"cache_name", cacheName,
				"key", key,
				"error", err,
			)
		}
	}

	if rt.refreshPool == nil ||
		!rt.refreshPool.Submit(refreshJob{run: run}) {
		rt.metrics.IncWritebackDropped(
			cacheName,
			"delete_queue_full",
		)
	}
}

func GetOrLoadWithMutex[T any](
	ctx context.Context,
	client *CacheClient,
	key string,
	ttl time.Duration,
	nullTTL time.Duration,
	loader func(context.Context) (T, bool, error),
) (T, bool, error) {
	var zero T
	const (
		lockTTL     = 10 * time.Second
		maxAttempts = 40
	)
	lockKey := mutexKey(key)
	for attempts := 0; attempts < maxAttempts; attempts++ {
		val, found, hit, err := readCachedValue[T](ctx, client, key)
		if err != nil {
			return zero, false, err
		}
		if hit {
			return val, found, nil
		}
		lockVal := genUniqueID()
		locked, err := client.rdb.SetNX(ctx, lockKey, lockVal, lockTTL).Result()
		if err != nil {
			return zero, false, fmt.Errorf("set mutex for %q failed: %w", key, err)
		}
		if locked {
			loaderCtx, loaderCancel := context.WithTimeout(ctx, 8*time.Second)
			defer loaderCancel()
			val, found, loadErr := func() (resVal T, resFound bool, resErr error) {
				defer func(k string, v string) {
					bgCtx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					releaseErr := client.rdb.Eval(bgCtx, releaseLua, []string{k}, v).Err()
					if releaseErr != nil {
						log.Printf("release mutex for %q failed: %v", k, releaseErr)
					}
					if r := recover(); r != nil {
						log.Printf("loader panic: %v", r)
						resVal = zero
						resFound = false
						resErr = fmt.Errorf("loader panicked: %v", r)
					}
				}(lockKey, lockVal)

				val, found, hit, err := readCachedValue[T](ctx, client, key)
				if err != nil {
					return zero, false, err
				}
				if hit {
					return val, found, nil
				}
				return loadAndCache(loaderCtx, client, key, ttl, nullTTL, loader)
			}()
			if loadErr != nil {
				return zero, false, loadErr
			}
			return val, found, nil
		}
		backOff := 10 * time.Millisecond * time.Duration(1<<min(attempts, 4))
		timer := time.NewTimer(backOff)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return zero, false, ctx.Err()
		case <-timer.C:
		}
	}
	return zero, false, fmt.Errorf("cached rebuild contention exceeded retry limit: key=%q", key)
}

func readCachedValue[T any](
	ctx context.Context,
	client *CacheClient,
	key string,
) (value T, found bool, hit bool, err error) {
	var zero T
	val, err := client.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return zero, false, false, nil
	}
	if err != nil {
		return zero, false, false, fmt.Errorf("get cache %q, %w", key, err)
	}
	trimmed := bytes.TrimSpace([]byte(val))
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return zero, false, true, nil
	}

	var result T
	if err := json.Unmarshal(trimmed, &result); err == nil {
		return result, true, true, nil
	} else {
		if delErr := client.rdb.Del(ctx, key).Err(); delErr != nil {
			return zero, false, false, fmt.Errorf("cached %q is corrupted: %v; delete failed: %w", key, err, delErr)
		}
	}
	return zero, false, false, nil
}

func loadAndCache[T any](
	ctx context.Context,
	client *CacheClient,
	key string,
	ttl time.Duration,
	nullTTL time.Duration,
	loader func(context.Context) (T, bool, error),
) (T, bool, error) {
	var zero T
	data, found, err := loader(ctx)
	if err != nil {
		return zero, false, err
	}
	if !found {
		if err := client.rdb.Set(ctx, key, "", nullTTL).Err(); err != nil {
			return zero, false, fmt.Errorf("set null cache %q: %w", key, err)
		}
		return zero, false, nil
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		return zero, false, fmt.Errorf("marshal cache %q: %w", key, err)
	}

	if bytes.Equal(bytes.TrimSpace(encoded), []byte("null")) {
		return zero, false, errors.New("cache: loader returned found=true with a nil/null value")
	}

	if err := client.rdb.Set(ctx, key, encoded, ttl).Err(); err != nil {
		return zero, false, fmt.Errorf("set cache %q: %w", key, err)
	}

	return data, true, nil
}

func GetOrLoadWithLogicalExpire[T any](
	ctx context.Context,
	client *CacheClient,
	key string,
	logicalTTL time.Duration,
	nullTTL time.Duration,
	loader func(context.Context) (T, bool, error),
) (T, bool, error) {
	var zero T
	val, found, hit, expired, err := readLogicalCache[T](ctx, client, key)
	if err != nil {
		return zero, false, err
	}
	if !hit {
		return loadAndCacheWithLogicalExpire(ctx, client, key, logicalTTL, nullTTL, loader)
	}
	if !found {
		return zero, false, nil
	}
	if !expired {
		return val, true, nil
	}
	scheduleLogicalRefresh(ctx, client, key, logicalTTL, nullTTL, loader)
	return val, true, nil
}

func loadAndCacheWithLogicalExpire[T any](
	ctx context.Context,
	client *CacheClient,
	key string,
	logicalTTL time.Duration,
	nullTTL time.Duration,
	loader func(context.Context) (T, bool, error),
) (T, bool, error) {
	var zero T
	data, found, err := loader(ctx)
	if err != nil {
		return zero, false, err
	}
	if !found {
		if err := client.rdb.Set(ctx, key, "", nullTTL).Err(); err != nil {
			return zero, false, fmt.Errorf("set logical null cache %q: %w", key, err)
		}
		return zero, false, nil
	}

	if err := client.setWithJitter(ctx, key, data, logicalTTL); err != nil {
		return zero, false, fmt.Errorf("set logical cache %q with jitter: %w", key, err)
	}
	return data, true, nil
}

func readLogicalCache[T any](
	ctx context.Context,
	client *CacheClient,
	key string,
) (value T, found bool, hit bool, expire bool, err error) {
	var zero T
	val, err := client.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return zero, false, false, false, nil
	}
	if err != nil {
		return zero, false, false, false, fmt.Errorf("get logical cache %q: %w", key, err)
	}
	trimmed := bytes.TrimSpace([]byte(val))
	if len(trimmed) == 0 {
		return zero, false, true, false, nil
	}
	if bytes.Equal(trimmed, []byte("null")) {
		if err := client.rdb.Del(ctx, key).Err(); err != nil {
			return zero, false, false, false, fmt.Errorf("delete legacy null cache %q: %w", key, err)
		}
		return zero, false, false, false, nil
	}

	var cached RedisData
	if err := json.Unmarshal(trimmed, &cached); err != nil {
		if delErr := client.rdb.Del(ctx, key).Err(); delErr != nil {
			return zero, false, false, false, fmt.Errorf("logical cache %q is corrupted: %v; delete failed %w", key, err, delErr)
		}
		return zero, false, false, false, nil
	}
	data := bytes.TrimSpace(cached.Data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		if err := client.rdb.Del(ctx, key).Err(); err != nil {
			return zero, false, false, false, fmt.Errorf("delete legacy null cache %q: %w", key, err)
		}
		return zero, false, false, false, nil
	}

	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		if delErr := client.rdb.Del(ctx, key).Err(); delErr != nil {
			return zero, false, false, false, fmt.Errorf("logical cache %q is corrupted: %v; delete failed %w", key, err, delErr)
		}
		return zero, false, false, false, nil
	}
	return result, true, true, !time.Now().Before(cached.ExpireAt), nil
}

func scheduleLogicalRefresh[T any](
	ctx context.Context,
	client *CacheClient,
	key string,
	logicalTTL time.Duration,
	nullTTL time.Duration,
	loader func(context.Context) (T, bool, error),
) {
	const refreshTimeout = 5 * time.Second
	lockTTL := refreshTimeout + 2*time.Second

	lockKey := mutexKey(key)
	lockVal := genUniqueID()

	locked, err := client.rdb.SetNX(ctx, lockKey, lockVal, lockTTL).Result()
	if err != nil || !locked {
		log.Printf("failed to set mutex for %q: %v", key, err)
		return
	}

	effectiveTTL := logicalTTL
	if client.refreshPool != nil {
		effectiveTTL = client.refreshPool.jitteredTTL(logicalTTL)
	}

	run := func() {
		defer releaseRefreshLock(client, lockKey, lockVal)
		baseCtx := context.WithoutCancel(ctx)
		bgCtx, cancel := context.WithTimeout(baseCtx, refreshTimeout)
		defer cancel()
		data, found, err := loader(bgCtx)
		if err != nil {
			return
		}
		if !found {
			if err = client.rdb.Set(bgCtx, key, "", nullTTL).Err(); err != nil {
				log.Printf("failed to set null for %q: %v", key, err)
			}
			return
		}

		if err = SetWithLogicalExpire(client.rdb, bgCtx, key, data, effectiveTTL); err != nil {
			log.Printf("failed to set logical expire for %q: %v", key, err)
		}

	}

	if client.refreshPool != nil {
		if !client.refreshPool.Submit(refreshJob{run: run}) {
			releaseRefreshLock(client, lockKey, lockVal)
		}
		return
	}
	go run()
}

func releaseRefreshLock(
	client *CacheClient,
	lockKey string,
	lockVal string,
) {
	bgCtx, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancel()
	_ = client.rdb.Eval(bgCtx, releaseLua, []string{lockKey}, lockVal).Err()
}

// SetWithLogicalExpire 写入数据时附加逻辑过期时间
// 物理上数据永不过期，查询时判断逻辑过期时间
func SetWithLogicalExpire(
	rdb redis.Cmdable,
	ctx context.Context,
	key string,
	value any,
	expire time.Duration,
) error {
	bytesData, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if bytes.Equal(bytes.TrimSpace(bytesData), []byte("null")) {
		return errors.New("cache: cannot set logical expire for a nil/null value when data is found")
	}
	rd := RedisData{
		Data:     bytesData,
		ExpireAt: time.Now().Add(expire),
	}
	data, err := json.Marshal(rd)
	if err != nil {
		return err
	}
	return rdb.Set(ctx, key, data, 24*time.Hour).Err()
}

func (c *CacheClient) SetWithLogicalExpire(
	ctx context.Context,
	key string,
	value any,
	expire time.Duration,
) error {
	return SetWithLogicalExpire(c.rdb, ctx, key, value, expire)
}

func (c *CacheClient) setWithJitter(ctx context.Context, key string, value any, baseTTL time.Duration) error {
	effectiveTTL := baseTTL
	if c.refreshPool != nil {
		effectiveTTL = c.refreshPool.jitteredTTL(baseTTL)
	} else {
		jitter := time.Duration(rand.Int63n(int64(float64(baseTTL) * c.jitterRatio)))
		effectiveTTL = baseTTL + jitter
	}
	return c.SetWithLogicalExpire(ctx, key, value, effectiveTTL)
}

func genUniqueID() string {
	return fmt.Sprintf("%d_%d", time.Now().UnixNano(), rand.Int63())
}

type GeoSearchResult struct {
	Name     string  `json:"name"`
	Distance float64 `json:"distance"`
}

func (c *CacheClient) GeoAdd(ctx context.Context, key, name string, longitude, latitude float64) error {
	return c.rdb.GeoAdd(ctx, key, &redis.GeoLocation{
		Name:      name,
		Longitude: longitude,
		Latitude:  latitude,
	}).Err()
}

func (c *CacheClient) GeoSearch(ctx context.Context, cacheName string, key string, lon, lat, radius float64, count int) ([]GeoSearchResult, error) {
	res, err := c.rdb.GeoSearchLocation(ctx, key, &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{
			Longitude:  lon,
			Latitude:   lat,
			Radius:     radius,
			RadiusUnit: "m",
			Sort:       "ASC",
			Count:      count,
		},
		WithDist: true,
	}).Result()
	rt := c.runtime
	if err != nil {
		rt.metrics.IncRedisError(
			cacheName,
			"geosearch",
		)
		rt.logger.Error(
			"failed to delete corrupted cache value",
			"cache_name", cacheName,
			"key", key,
			"error", err,
		)
		return nil, err
	}

	results := make([]GeoSearchResult, 0, len(res))
	for _, loc := range res {
		results = append(results, GeoSearchResult{
			Name:     loc.Name,
			Distance: loc.Dist,
		})
	}
	return results, nil
}

// DB查询（不回源）
func LoadProtectedGeo[T any](
	ctx context.Context,
	client *CacheClient,
	cacheName string,
	key string,
	reason string,
	loader func(context.Context) (T, error),
) (T, error) {
	var zero T
	rt := client.runtime

	// 相同flightKey的并发请求合并，cacheName的加入并非必要，key已经足够区分不同缓存类型，只是为了防御性编程
	flightKey := cacheName + ":" + key

	// DoChan返回一个结果channel，同key的调用只有一个函数实际执行，其他调用共享其结果
	// 与Do不同的是，Do直接等待返回结果，因此即使请求的ctx已经取消，调用Do的goroutine仍旧会阻塞在Do中直到查询完成
	// 而DoChan返回的结果Channel，调用方可以通过select在ctx取消时提前返回该请求，防止无意义等待
	resultCh := rt.flights.DoChan(flightKey, func() (any, error) {
		// singlefilght是多个任务的公共请求，不能完全依赖第一个请求的取消信号
		// 因此通过WithoutCancel先保留原始context的value，再移除第一个请求的取消和deadline，在附加独立的LoaderTimeout
		baseCtx := context.WithoutCancel(ctx)
		loadCtx, cancel := context.WithTimeout(
			baseCtx,
			rt.policy.LoaderTimeout,
		)
		defer cancel()

		// 高并发中的请求只有一个占用DB信号量，持有信号量的DB才能执行loader
		if err := rt.acquireDB(loadCtx); err != nil {
			return nil, err
		}
		defer rt.releaseDB()
		rt.metrics.IncDBFallback(cacheName, reason)

		// 进入实际DB查询
		started := time.Now()
		data, err := loader(loadCtx)
		rt.metrics.ObserveDBFallbackDuration(cacheName, time.Since(started))
		if err != nil {
			return nil, err
		}

		return loadResult[T]{
			value: data,
		}, nil
	})
	select {
	case <-ctx.Done():
		return zero, ctx.Err()

	case result := <-resultCh:
		if result.Shared {
			// shared表示该结果被多个调用共享
			rt.metrics.IncSingleflightShared(cacheName)
		}
		if result.Err != nil {
			return zero, result.Err
		}

		loaded, ok := result.Val.(loadResult[T])
		if !ok {
			return zero, errors.New(
				"cache: unexpected singleflight result type",
			)
		}

		return loaded.value, nil
	}
}
