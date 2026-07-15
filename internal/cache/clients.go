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
	refreshPool *RefreshPool
	jitterRatio float64
}

func NewCacheClient(rdb redis.Cmdable, refreshPool *RefreshPool) *CacheClient {
	return &CacheClient{
		rdb:         rdb,
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
	key string,
	ttl time.Duration,
	nullTTL time.Duration,
	loader func(context.Context) (T, bool, error),
) (T, bool, error) {
	var zero T

	val, err := client.rdb.Get(ctx, key).Result()
	if err == nil {
		if val == "" || string(bytes.TrimSpace([]byte(val))) == "null" {
			return zero, false, nil
		}
		var result T
		unmarshalErr := json.Unmarshal([]byte(val), &result)
		if unmarshalErr == nil {
			return result, true, nil
		}
		if delErr := client.rdb.Del(ctx, key).Err(); delErr != nil {
			return zero, false, fmt.Errorf("cache %q is corrupted: %v; delete failed: %w", key, unmarshalErr, delErr)
		}
		return loadAndCache(ctx, client, key, ttl, nullTTL, loader)
	}
	if !errors.Is(err, redis.Nil) {
		return zero, false, fmt.Errorf("get cache %q: %w", key, err)
	}
	return loadAndCache(ctx, client, key, ttl, nullTTL, loader)
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
			val, found, loadErr := func() (resVal T, resFound bool, resErr error) {
				defer func(k string, v interface{}) {
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
				return loadAndCache(ctx, client, key, ttl, nullTTL, loader)
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

	if err := SetWithLogicalExpire(client.rdb, ctx, key, data, logicalTTL); err != nil {
		return zero, false, fmt.Errorf("set logical cache %q: %w", key, err)
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

		if err = SetWithLogicalExpire(client.rdb, bgCtx, key, data, logicalTTL); err != nil {
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

// // 策略1: 缓存空值 -> 避免缓存穿透
// // 查缓存 -> 命中（含空值标记）直接返回 -> 不命中查数据库
// // -> 有数据写缓存返回 / 无数据写空值标记（短TTL）返回
// func (c *CacheClient) QueryWithPassThrough(
// 	ctx context.Context,
// 	key string,
// 	result any,
// 	ttl time.Duration,
// 	nullTTL time.Duration,
// 	dbFunc func() (any, error),
// ) error {
// 	val, err := c.rdb.Get(ctx, key).Result()
// 	if err == nil {
// 		if val == "" {
// 			return ErrDataNotFound
// 		}
// 		return json.Unmarshal([]byte(val), result)
// 	}
// 	if !errors.Is(err, redis.Nil) {
// 		return err
// 	}

// 	// 缓存未命中，查询数据库
// 	data, err := dbFunc()
// 	if err != nil {
// 		return err
// 	}
// 	if data == nil {
// 		// 数据库无数据，缓存空值标记
// 		c.rdb.Set(ctx, key, "", nullTTL)
// 		return ErrDataNotFound
// 	}

// 	// 数据库有数据，序列化写入缓存
// 	bytes, _ := json.Marshal(data)
// 	c.rdb.Set(ctx, key, bytes, ttl)

// 	// 反序列化结果，保证和直接从缓存读取一致
// 	return json.Unmarshal(bytes, result)
// }

// // 策略2: 互斥锁 -> 避免缓存击穿
// // 查缓存 -> 命中直接返回 -> 不命中尝试获取互斥锁
// // -> 获取成功查数据库 -> 有数据写缓存返回 / 无数据写空值标记（短TTL）返回 -> 释放锁
// // -> 获取失败等待重试
// func (c *CacheClient) QueryWithMutex(
// 	ctx context.Context,
// 	key string,
// 	result any,
// 	dbFunc func() (any, error),
// ) error {
// 	val, err := c.rdb.Get(ctx, key).Result()
// 	if err == nil {
// 		return json.Unmarshal([]byte(val), result)
// 	}
// 	if !errors.Is(err, redis.Nil) {
// 		return err
// 	}

// 	lockKey := mutexKey(key)
// 	// 尝试获取互斥锁
// 	ok, err := c.rdb.SetNX(ctx, lockKey, "1", 10*time.Second).Result()
// 	if err != nil {
// 		return err
// 	}
// 	if ok {
// 		val, err := c.rdb.Get(ctx, key).Result()
// 		if err == nil {
// 			c.rdb.Del(ctx, lockKey) // 释放锁
// 			return json.Unmarshal([]byte(val), result)
// 		}

// 		data, err := dbFunc()
// 		if err != nil {
// 			c.rdb.Del(ctx, lockKey) // 释放锁
// 			return err
// 		}
// 		// TODO: Waiting for fix
// 		if data == nil {
// 			c.rdb.Del(ctx, lockKey) // 释放锁
// 			return ErrDataNotFound
// 		}

// 		bytes, _ := json.Marshal(data)
// 		c.rdb.Set(ctx, key, bytes, 30*time.Minute)
// 		c.rdb.Del(ctx, lockKey) // 释放锁

// 		return json.Unmarshal(bytes, result)
// 	}

// 	time.Sleep(50 * time.Millisecond) // 等待重试
// 	return c.QueryWithMutex(ctx, key, result, dbFunc)
// }

// 策略3: 逻辑过期 -> 避免缓存击穿
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
	return rdb.Set(ctx, key, data, 0).Err()
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
	jitter := time.Duration(rand.Int63n(int64(float64(baseTTL) * c.jitterRatio)))
	return c.SetWithLogicalExpire(ctx, key, value, baseTTL+jitter)
}

func genUniqueID() string {
	return fmt.Sprintf("%d_%d", time.Now().UnixNano(), rand.Int63())
}

// // QueryWithLogicalExpire 查询时判断逻辑过期时间，过期则异步更新缓存
// func (c *CacheClient) QueryWithLogicalExpire(
// 	ctx context.Context,
// 	key string,
// 	result any,
// 	LogicalExpire time.Duration,
// 	dbFunc func() (any, error),
// ) error {
// 	val, err := c.rdb.Get(ctx, key).Result()
// 	if err != nil {
// 		if errors.Is(err, redis.Nil) {
// 			data, dbErr := dbFunc()
// 			if dbErr != nil {
// 				return dbErr
// 			}
// 			if data == nil {
// 				return ErrDataNotFound
// 			}
// 			if err := c.setWithJitter(ctx, key, data, LogicalExpire); err != nil {
// 				return err
// 			}
// 			bytes, _ := json.Marshal(data)
// 			return json.Unmarshal(bytes, result)
// 		}
// 		return err
// 	}

// 	var rd RedisData
// 	if err := json.Unmarshal([]byte(val), &rd); err != nil {
// 		return fmt.Errorf("Cache: Unmarshal RedisData Failed")
// 	}
// 	if err := json.Unmarshal(rd.Data, result); err != nil {
// 		return err
// 	}
// 	if time.Now().Before(rd.ExpireAt) {
// 		return nil
// 	}

// 	// 逻辑过期，尝试获取互斥锁更新缓存
// 	lockKey := mutexKey(key)
// 	lockValue := genUniqueID()
// 	ok, err := c.rdb.SetNX(ctx, lockKey, lockValue, 15*time.Second).Result()
// 	if err != nil || !ok {
// 		return nil
// 	}
// 	if c.refreshPool != nil {
// 		job := refreshJob{
// 			key:       key,
// 			lockKey:   lockKey,
// 			lockValue: lockValue,
// 			baseTTL:   LogicalExpire,
// 			dbFunc:    dbFunc,
// 		}
// 		if !c.refreshPool.Submit(job) {
// 			_ = c.rdb.Eval(ctx, releaseLua, []string{lockKey}, lockValue).Err()
// 		}
// 		return nil
// 	}
// 	go func() {
// 		bgCtx := context.Background()
// 		defer c.rdb.Eval(bgCtx, releaseLua, []string{lockKey}, lockValue).Err()
// 		data, err := dbFunc()
// 		if err == nil || data != nil {
// 			c.setWithJitter(bgCtx, key, data, LogicalExpire)
// 		}
// 	}()

// 	return nil
// }

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

func (c *CacheClient) GeoSearch(ctx context.Context, key string, lon, lat, radius float64, count int) ([]GeoSearchResult, error) {
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
	if err != nil {
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
