package cache

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type refreshJob struct {
	key       string
	lockKey   string
	lockValue string
	baseTTL   time.Duration
	dbFunc    func() (any, error)
}

type RefreshPool struct {
	rdb         redis.Cmdable
	jobs        chan refreshJob
	wg          sync.WaitGroup
	jitterRatio float64
	rand        *rand.Rand
	mu          sync.Mutex
}

func NewRefreshPool(rdb redis.Cmdable, workers, queueSize int, jitterRatio float64) *RefreshPool {
	if workers <= 0 {
		workers = 10
	}
	if queueSize <= 0 {
		queueSize = workers * 2
	}
	if jitterRatio < 0 {
		jitterRatio = 0
	}
	if jitterRatio > 0.5 {
		jitterRatio = 0.5
	}

	p := &RefreshPool{
		rdb:         rdb,
		jobs:        make(chan refreshJob, queueSize),
		jitterRatio: jitterRatio,
	}

	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.worker()
	}
	return p
}

func (p *RefreshPool) worker() {
	defer p.wg.Done()
	for job := range p.jobs {
		p.execute(job)
	}
}

const releaseLua = `
if redis.call('get', KEYS[1]) == ARGV[1] then
	return redis.call('del', KEYS[1])
else
	return 0
end`

func (p *RefreshPool) execute(job refreshJob) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("cache refresh panic: key=%s, panic=%v", job.key, r)
		}
		bgCtx := context.Background()
		_ = p.rdb.Eval(bgCtx, releaseLua, []string{job.lockKey}, job.lockValue).Err()
	}()

	data, err := job.dbFunc()
	if err != nil || data == nil {
		return
	}

	effectiveTTL := p.jitteredTTL(job.baseTTL)
	_ = SetWithLogicalExpire(p.rdb, context.Background(), job.key, data, effectiveTTL)
}

func (p *RefreshPool) jitteredTTL(baseTTL time.Duration) time.Duration {
	if p.jitterRatio <= 0 {
		return baseTTL
	}

	p.mu.Lock()
	jitterMax := int64(float64(baseTTL) * p.jitterRatio)
	var jitter time.Duration
	if jitterMax > 0 {
		jitter = time.Duration(p.rand.Int63n(jitterMax))
	}
	p.mu.Unlock()

	return jitter
}

func (p *RefreshPool) Submit(job refreshJob) bool {
	select {
	case p.jobs <- job:
		return true
	default:
		return false
	}
}

func (p *RefreshPool) Shutdown() {
	close(p.jobs)
	p.wg.Wait()
}
