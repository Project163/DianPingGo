package idgen

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisIDWorker_NextID_MonotonicIncreasing(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	worker := NewRedisIDWorker(rdb)
	ctx := context.Background()

	id1, err := worker.NextID(ctx, "voucher_order_test")
	if err != nil {
		t.Fatalf("Failed to generate first ID: %v", err)
	}
	id2, err := worker.NextID(ctx, "voucher_order_test")
	if err != nil {
		t.Fatalf("Failed to generate second ID: %v", err)
	}

	if id2 <= id1 {
		t.Errorf("Expected second ID to be greater than first ID, got %d and %d", id1, id2)
	}
}

func TestRedisIDWorker_NextID_DifferentPrefixIsolated(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	worker := NewRedisIDWorker(rdb)
	ctx := context.Background()

	id1, err := worker.NextID(ctx, "voucher_order_test")
	if err != nil {
		t.Fatalf("Failed to generate ID for voucher_order_test: %v", err)
	}
	id2, err := worker.NextID(ctx, "payment_order_test")
	if err != nil {
		t.Fatalf("Failed to generate ID for payment_order_test: %v", err)
	}

	// 不同前缀的计数独立，所以前32位（时间戳部分）相同的情况下，
	// 第一个 ID 的低位是 1，第二个也是 1
	// 但由于时间戳相同，应该 id1 == id2
	// 实际上不同前缀计数独立，所以两个都是 count=1
	if id1 != id2 {
		t.Errorf("Expected IDs with different prefixes to be independent, got %d and %d", id1, id2)
	}
}
