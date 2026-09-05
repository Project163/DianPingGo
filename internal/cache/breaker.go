package cache

import (
	"sync"
	"time"
)

// 统计窗口（Window时间内至少收到MinimumRequests次请求后失败了超过FailureRatio比率时会触发熔断，熔断持续OpenDuration时间）
type BreakerConfig struct {
	Window          time.Duration
	MinimumRequests uint64
	FailureRatio    float64
	OpenDuration    time.Duration
}

type RedisBreaker struct {
	mu sync.Mutex // 全局熔断器，需要一个mutex防止数据竞争

	cfg BreakerConfig

	windowStarted time.Time // 记录窗口何时开始
	total         uint64    // 记录当前窗口计入统计的Redis操作总数
	failures      uint64    // 当前窗口内Redis依赖失败次数

	openUntil time.Time // 熔断器打开状态的截止时间
	probing   bool      // 保证半开状态造成回复瞬间的流量冲击
}

// 熔断器判断状态后执行操作的结果记录
type BreakerPermit struct {
	breaker *RedisBreaker
	probe   bool      // 是否是半开的单独探针
	once    sync.Once // 保证Done只执行一次，防止多次记录
}

func NewRedisBreaker(cfg BreakerConfig) *RedisBreaker {
	if cfg.Window <= 0 {
		cfg.Window = 10 * time.Second
	}
	if cfg.MinimumRequests == 0 {
		cfg.MinimumRequests = 20
	}
	if cfg.FailureRatio <= 0 || cfg.FailureRatio > 1 {
		cfg.FailureRatio = 0.5
	}
	if cfg.OpenDuration <= 0 {
		cfg.OpenDuration = 3 * time.Second
	}

	return &RedisBreaker{
		cfg:           cfg,
		windowStarted: time.Now(),
	}
}

// 判断当前熔断器状态决定后续操作该如何执行
func (b *RedisBreaker) Allow() (*BreakerPermit, bool) {
	now := time.Now()

	b.mu.Lock()
	defer b.mu.Unlock()

	// 熔断期间直接拒绝 Redis 操作。
	if now.Before(b.openUntil) {
		return nil, false
	}

	// 冷却期结束，进入 half-open，只放一个探测请求。
	if !b.openUntil.IsZero() {
		if b.probing {
			return nil, false
		}
		b.probing = true
		return &BreakerPermit{breaker: b, probe: true}, true
	}

	// 未处于熔断期，检查并更新统计窗口
	if now.Sub(b.windowStarted) >= b.cfg.Window {
		b.resetWindowLocked(now)
	}

	return &BreakerPermit{breaker: b}, true
}

// 判断当前熔断状态决定后续熔断器状态
func (p *BreakerPermit) Done(success bool) {
	if p == nil || p.breaker == nil {
		return
	}

	p.once.Do(func() {
		p.breaker.record(success, p.probe)
	})
}

func (b *RedisBreaker) record(success, probe bool) {
	now := time.Now()

	b.mu.Lock()
	defer b.mu.Unlock()

	// 半开状态的请求
	if probe {
		b.probing = false
		// 请求成功回复Closed状态并更新窗口
		if success {
			b.openUntil = time.Time{}
			b.resetWindowLocked(now)
			return
		}
		// 请求失败重新设置进入熔断
		b.openUntil = now.Add(b.cfg.OpenDuration)
		return
	}

	// 某些请求可能在熔断打开前已经发出，此时不再污染新窗口。
	if !b.openUntil.IsZero() {
		return
	}

	if now.Sub(b.windowStarted) >= b.cfg.Window {
		b.resetWindowLocked(now)
	}

	// 熔断判断
	b.total++
	if !success {
		b.failures++
	}

	if b.total < b.cfg.MinimumRequests {
		return
	}

	ratio := float64(b.failures) / float64(b.total)
	if ratio >= b.cfg.FailureRatio {
		b.openUntil = now.Add(b.cfg.OpenDuration)
	}
}

func (b *RedisBreaker) resetWindowLocked(now time.Time) {
	b.windowStarted = now
	b.total = 0
	b.failures = 0
}

// Redis操作结果无法用于判断健康状态（ctx错误，请求已取消等）
func (p *BreakerPermit) Ignore() {
	if p == nil || p.breaker == nil {
		return
	}

	p.once.Do(func() {
		p.breaker.ignore(p.probe)
	})
}

// 对于普通请求，不增加total/failures
// 对于半开探针、只释放probing，允许下一个请求继续重新检测
func (b *RedisBreaker) ignore(probe bool) {
	if !probe {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.probing = false
}
