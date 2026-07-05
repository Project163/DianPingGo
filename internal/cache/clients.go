package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrDataNotFound = errors.New("cache: data not found in database")

type RedisData struct {
	Data     json.RawMessage `json:"data"`
	ExpireAt time.Time       `json:"expire_at"`
}

type CacheClient struct {
	rdb redis.Cmdable
}

func NewCacheClient(rdb redis.Cmdable) *CacheClient {
	return &CacheClient{rdb: rdb}
}

func mutexKey(key string) string {
	return "mutex:" + key
}

func (c *CacheClient) Del(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

// 策略1: 缓存空值 -> 避免缓存穿透
// 查缓存 -> 命中（含空值标记）直接返回 -> 不命中查数据库
// -> 有数据写缓存返回 / 无数据写空值标记（短TTL）返回
func (c *CacheClient) QueryWithPassThrough(
	ctx context.Context,
	key string,
	result any,
	ttl time.Duration,
	nullTTL time.Duration,
	dbFunc func() (any, error),
) error {
	val, err := c.rdb.Get(ctx, key).Result()
	if err == nil {
		if val == "" {
			return ErrDataNotFound
		}
		return json.Unmarshal([]byte(val), result)
	}
	if !errors.Is(err, redis.Nil) {
		return err
	}

	// 缓存未命中，查询数据库
	data, err := dbFunc()
	if err != nil {
		return err
	}
	if data == nil {
		// 数据库无数据，缓存空值标记
		c.rdb.Set(ctx, key, "", nullTTL)
		return ErrDataNotFound
	}

	// 数据库有数据，序列化写入缓存
	bytes, _ := json.Marshal(data)
	c.rdb.Set(ctx, key, bytes, ttl)

	// 反序列化结果，保证和直接从缓存读取一致
	return json.Unmarshal(bytes, result)
}

// 策略2: 互斥锁 -> 避免缓存击穿
// 查缓存 -> 命中直接返回 -> 不命中尝试获取互斥锁
// -> 获取成功查数据库 -> 有数据写缓存返回 / 无数据写空值标记（短TTL）返回 -> 释放锁
// -> 获取失败等待重试
func (c *CacheClient) QueryWithMutex(
	ctx context.Context,
	key string,
	result any,
	dbFunc func() (any, error),
) error {
	val, err := c.rdb.Get(ctx, key).Result()
	if err == nil {
		return json.Unmarshal([]byte(val), result)
	}
	if !errors.Is(err, redis.Nil) {
		return err
	}

	lockKey := mutexKey(key)
	// 尝试获取互斥锁
	ok, err := c.rdb.SetNX(ctx, lockKey, "1", 10*time.Second).Result()
	if err != nil {
		return err
	}
	if ok {
		val, err := c.rdb.Get(ctx, key).Result()
		if err == nil {
			c.rdb.Del(ctx, lockKey) // 释放锁
			return json.Unmarshal([]byte(val), result)
		}

		data, err := dbFunc()
		if err != nil {
			c.rdb.Del(ctx, lockKey) // 释放锁
			return err
		}
		if data == nil {
			c.rdb.Del(ctx, lockKey) // 释放锁
			return ErrDataNotFound
		}

		bytes, _ := json.Marshal(data)
		c.rdb.Set(ctx, key, bytes, 30*time.Minute)
		c.rdb.Del(ctx, lockKey) // 释放锁

		return json.Unmarshal(bytes, result)
	}

	time.Sleep(50 * time.Millisecond) // 等待重试
	return c.QueryWithMutex(ctx, key, result, dbFunc)
}

// 策略3: 逻辑过期 -> 避免缓存击穿
// SetWithLogicalExpire 写入数据时附加逻辑过期时间
// 物理上数据永不过期，查询时判断逻辑过期时间
func (c *CacheClient) SetWithLogicalExpire(
	ctx context.Context,
	key string,
	value any,
	expire time.Duration,
) error {
	bytes, err := json.Marshal(value)
	if err != nil {
		return err
	}
	rd := RedisData{
		Data:     bytes,
		ExpireAt: time.Now().Add(expire),
	}
	data, err := json.Marshal(rd)
	return c.rdb.Set(ctx, key, data, 0).Err()
}

// QueryWithLogicalExpire 查询时判断逻辑过期时间，过期则异步更新缓存
func (c *CacheClient) QueryWithLogicalExpire(
	ctx context.Context,
	key string,
	result any,
	LogicalExpire time.Duration,
	dbFunc func() (any, error),
) error {
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			data, dbErr := dbFunc()
			if dbErr != nil {
				return dbErr
			}
			if data == nil {
				return ErrDataNotFound
			}
			if err := c.SetWithLogicalExpire(ctx, key, data, LogicalExpire); err != nil {
				return err
			}
			bytes, _ := json.Marshal(data)
			return json.Unmarshal(bytes, result)
		}
		return err
	}

	var rd RedisData
	if err := json.Unmarshal([]byte(val), &rd); err != nil {
		return fmt.Errorf("Cache: Unmarshal RedisData Failed")
	}
	if err := json.Unmarshal(rd.Data, result); err != nil {
		return err
	}
	if time.Now().Before(rd.ExpireAt) {
		return nil
	}

	// 逻辑过期，尝试获取互斥锁更新缓存
	lockKey := mutexKey(key)
	ok, err := c.rdb.SetNX(ctx, lockKey, "1", 10*time.Second).Result()
	if err != nil {
		return err
	}
	if ok {
		go func() {
			bgCtx := context.Background()
			defer c.rdb.Del(bgCtx, lockKey) // 释放锁
			data, err := dbFunc()
			if err != nil {
				return
			}
			c.SetWithLogicalExpire(bgCtx, key, data, LogicalExpire)
		}()
	}
	return nil
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
