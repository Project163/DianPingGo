package idgen

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	beginTimeStamp int64 = 1640995200
	countBits      uint8 = 32
)

type RedisIDWorker struct {
	client redis.Cmdable
}

func NewRedisIDWorker(client redis.Cmdable) *RedisIDWorker {
	return &RedisIDWorker{client: client}
}

// NextID 生成一个全局唯一的 ID，格式为：
// [timestamp (32 bits)][counter (32 bits)]
// 其中 timestamp 是从 beginTimeStamp 开始的秒数，counter 是当天的自增计数器
func (w *RedisIDWorker) NextID(ctx context.Context, keyPrefix string) (int64, error) {
	now := time.Now()
	timeSecond := now.Unix()
	timeStamp := timeSecond - beginTimeStamp

	today := now.Format("20060102")
	key := fmt.Sprintf("icr:%s:%s", keyPrefix, today)
	count, err := w.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	return (timeStamp << countBits) | count, nil
}

// NextIDWithUUID 生成一个全局唯一的 UUID 字符串
// UUID 是一个 128 位的值，通常表示为 36 字符的字符串（包括连字符），具有非常高的唯一性
// 但是 UUID（标准v4） 是基于密码学的完全随机，不具有递增性
// 可以选择 UUID v7 来获得递增性，但目前还没有广泛支持的库直接实现 v7
// 而且 UUID 的长度较长，可能不适合某些场景（如数据库主键）
// 该函数暂时不使用
func (w *RedisIDWorker) NextIDWithUUID(ctx context.Context, keyPrefix string) (string, error) {
	id := uuid.New()
	return id.String(), nil
}

const (
	snowflakeEpoch int64 = 1640995200000
	workerBits     uint8 = 10
	sequenceBits   uint8 = 12
	workerMax      int64 = -1 ^ (-1 << workerBits)   // 1023
	sequenceMax    int64 = -1 ^ (-1 << sequenceBits) // 4095

	workerShift    uint8 = sequenceBits
	timestampShift uint8 = workerBits + sequenceBits
)

type SnowflakeIDWorker struct {
	client    *redis.Client
	workerKey string
	workerID  int64
	sequence  int64
	lastTime  int64
	mu        sync.Mutex
}

// NextIDWithSnowflake 生成一个全局唯一的 ID，格式为：
// [timestamp (41 bits)][workerID (10 bits)][counter (12 bits)]
// 其中 timestamp 是从 beginTimeStamp 开始的毫秒数，workerID 是机器 ID，counter 是同一毫秒内的自增计数器
// 雪花算法的经典结构是[unused (1 bit)][timestamp (41 bits)][datacenter (5 bits)][workerID (5 bits)][counter (12 bits)]
func NewSnowflakeIDWorker(ctx context.Context, client *redis.Client, workerKey string) (*SnowflakeIDWorker, error) {
	w := &SnowflakeIDWorker{
		client:    client,
		workerKey: fmt.Sprintf("snowflake:worker:%s", workerKey),
	}
	id, err := client.Incr(ctx, "snowflake:worker:counter").Result()
	if err != nil {
		return nil, fmt.Errorf("register worker failed: %w", err)
	}
	w.workerID = (id - 1) % (workerMax + 1)
	go w.heartbeat(ctx)

	return w, nil
}

func (w *SnowflakeIDWorker) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.client.Set(ctx, w.workerKey, w.workerID, 10*time.Second)
		}
	}
}

func (w *SnowflakeIDWorker) NextID(ctx context.Context) (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	ts := time.Now().UnixMilli()

	switch {
	case ts == w.lastTime:
		w.sequence = (w.sequence + 1) & sequenceMax
		if w.sequence == 0 {
			ts = w.waitNextMillis(ts)
		}
	case ts < w.lastTime:
		if w.lastTime-ts < 5*time.Second.Milliseconds() {
			return 0, fmt.Errorf("clock moved backwards, waiting until %d", w.lastTime)
		}
		ts = w.waitNextMillis(w.lastTime)
	default:
		w.sequence = 0
	}

	w.lastTime = ts
	id := ((ts - snowflakeEpoch) << timestampShift) | (w.workerID << workerShift) | w.sequence
	return id, nil
}

func (w *SnowflakeIDWorker) waitNextMillis(lastTime int64) int64 {
	ts := time.Now().UnixMilli()
	for ts <= lastTime {
		time.Sleep(time.Microsecond * 100) // 避免忙等待
		ts = time.Now().UnixMilli()
	}
	return ts
}
