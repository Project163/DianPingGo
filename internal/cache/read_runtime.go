package cache

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"golang.org/x/sync/singleflight"
)

var ErrDBFallbackOverloaded = errors.New("cache: db fallback overloaded")

// ReadPolicy 读取降级路径中的参数
type ReadPolicy struct {
	RedisReadTimeout  time.Duration // 读取型Redis能容忍的最大消耗时间
	RedisWriteTimeout time.Duration // 异步写回时Redis能容忍的最大消耗时间

	LoaderTimeout    time.Duration // 整个singleflight leader回源的最大时间
	DBAcquireTimeout time.Duration // DB回源信号量满时，新的回源的等待时间

	MaxDBConcurrency int // 当前进程内缓存回源 DB 的最大并发数
}

// ReadRuntime 全局共享的读取型缓存保护组件
type ReadRuntime struct {
	flights singleflight.Group // 同一进程中的相同Key的请求合并负责者
	dbSlots chan struct{}      // 缓冲channel用于计数信号量

	refreshPool *RefreshPool  // 异步回写任务池
	breaker     *RedisBreaker // 所有的读取缓存共享的Redis熔断状态
	logger      *slog.Logger  // 日志记录器
	metrics     Metrics       // 监控系统接口
	policy      ReadPolicy    // 参数
}

func NewReadRuntime(
	pool *RefreshPool,
	breaker *RedisBreaker,
	logger *slog.Logger,
	metrics Metrics,
	policy ReadPolicy,
) *ReadRuntime {
	if policy.MaxDBConcurrency <= 0 {
		policy.MaxDBConcurrency = 20
	}
	if policy.RedisReadTimeout <= 0 {
		policy.RedisReadTimeout = 100 * time.Millisecond
	}
	if policy.RedisWriteTimeout <= 0 {
		policy.RedisWriteTimeout = 200 * time.Millisecond
	}
	if policy.LoaderTimeout <= 0 {
		policy.LoaderTimeout = 2 * time.Second
	}
	if policy.DBAcquireTimeout <= 0 {
		policy.DBAcquireTimeout = 300 * time.Millisecond
	}
	if logger == nil {
		logger = slog.Default()
	}
	if metrics == nil {
		metrics = NoopMetrics{}
	}

	return &ReadRuntime{
		dbSlots:     make(chan struct{}, policy.MaxDBConcurrency),
		refreshPool: pool,
		breaker:     breaker,
		logger:      logger,
		metrics:     metrics,
		policy:      policy,
	}
}

// DB信号量的获取
func (r *ReadRuntime) acquireDB(ctx context.Context) error {
	timer := time.NewTimer(r.policy.DBAcquireTimeout)
	defer timer.Stop()

	select {
	// DB 回源并发未到上限，成功获得槽位
	case r.dbSlots <- struct{}{}:
		return nil
	// loader context已到期，停止等待并返回错误
	case <-ctx.Done():
		return ctx.Err()
	// 信号量超时，停止等待并返回自定义错误
	case <-timer.C:
		r.metrics.IncDBFallbackRejected("semaphore_timeout")
		return ErrDBFallbackOverloaded
	}
}

// DB信号量的释放
func (r *ReadRuntime) releaseDB() {
	<-r.dbSlots
}
