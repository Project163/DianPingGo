package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRedisBreaker_FailureThresholdReached_OpenCircuit(t *testing.T) {
	breaker := newTestRedisBreaker(2, 0.5, time.Minute)

	first, allowed := breaker.Allow()
	assert.True(t, allowed)
	first.Done(true)

	second, allowed := breaker.Allow()
	assert.True(t, allowed)
	second.Done(false)

	permit, allowed := breaker.Allow()
	assert.False(t, allowed)
	assert.Nil(t, permit)
	assert.Equal(t, uint64(2), breaker.total)
	assert.Equal(t, uint64(1), breaker.failures)
}

func TestRedisBreaker_OrdinaryPermitIgnored_DoNotRecordResult(t *testing.T) {
	breaker := newTestRedisBreaker(1, 1, time.Minute)
	permit, allowed := breaker.Allow()
	assert.True(t, allowed)

	permit.Ignore()

	assert.Equal(t, uint64(0), breaker.total)
	assert.Equal(t, uint64(0), breaker.failures)
	next, allowed := breaker.Allow()
	assert.True(t, allowed)
	assert.NotNil(t, next)
}

func TestRedisBreaker_HalfOpenProbeIgnored_ReleaseProbeWithoutClosingCircuit(t *testing.T) {
	breaker := newTestRedisBreaker(1, 1, time.Minute)
	initial, allowed := breaker.Allow()
	assert.True(t, allowed)
	initial.Done(false)
	breaker.openUntil = time.Now().Add(-time.Millisecond)

	probe, allowed := breaker.Allow()
	assert.True(t, allowed)
	assert.True(t, probe.probe)
	assert.True(t, breaker.probing)
	probe.Ignore()

	assert.False(t, breaker.probing)
	assert.False(t, breaker.openUntil.IsZero())
	nextProbe, allowed := breaker.Allow()
	assert.True(t, allowed)
	assert.True(t, nextProbe.probe)
	nextProbe.Done(true)
	assert.True(t, breaker.openUntil.IsZero())
	assert.False(t, breaker.probing)
}

func TestRedisBreaker_PermitCompletedMoreThanOnce_RecordOnlyFirstResult(t *testing.T) {
	breaker := newTestRedisBreaker(10, 1, time.Minute)
	permit, allowed := breaker.Allow()
	assert.True(t, allowed)

	permit.Ignore()
	permit.Done(false)
	permit.Ignore()

	assert.Equal(t, uint64(0), breaker.total)
	assert.Equal(t, uint64(0), breaker.failures)
}

func newTestRedisBreaker(
	minimumRequests uint64,
	failureRatio float64,
	openDuration time.Duration,
) *RedisBreaker {
	return NewRedisBreaker(BreakerConfig{
		Window:          time.Minute,
		MinimumRequests: minimumRequests,
		FailureRatio:    failureRatio,
		OpenDuration:    openDuration,
	})
}
