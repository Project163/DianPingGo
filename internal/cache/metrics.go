package cache

import "time"

// 监控系统接口（暂未接入实际监控系统）
type Metrics interface {
	IncCacheHit(cacheName string)
	IncCacheMiss(cacheName string)
	IncCacheCorrupted(cacheName string)

	IncRedisError(cacheName, operation string)
	IncRedisBypass(cacheName, reason string)

	IncSingleflightShared(cacheName string)

	IncDBFallback(cacheName, reason string)
	IncDBFallbackRejected(reason string)
	ObserveDBFallbackDuration(cacheName string, duration time.Duration)

	IncWritebackSuccess(cacheName, kind string)
	IncWritebackError(cacheName, kind, reason string)
	IncWritebackDropped(cacheName, reason string)
}

type NoopMetrics struct{}

// 保持空操作
func (NoopMetrics) IncCacheHit(string)                              {}
func (NoopMetrics) IncCacheMiss(string)                             {}
func (NoopMetrics) IncCacheCorrupted(string)                        {}
func (NoopMetrics) IncRedisError(string, string)                    {}
func (NoopMetrics) IncRedisBypass(string, string)                   {}
func (NoopMetrics) IncSingleflightShared(string)                    {}
func (NoopMetrics) IncDBFallback(string, string)                    {}
func (NoopMetrics) IncDBFallbackRejected(string)                    {}
func (NoopMetrics) ObserveDBFallbackDuration(string, time.Duration) {}
func (NoopMetrics) IncWritebackSuccess(string, string)              {}
func (NoopMetrics) IncWritebackError(string, string, string)        {}
func (NoopMetrics) IncWritebackDropped(string, string)              {}
