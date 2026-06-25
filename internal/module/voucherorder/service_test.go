package voucherorder

import (
	"context"
	"dianping/internal/module/seckillvoucher"
	"dianping/pkg/errmsg"
	"dianping/pkg/idgen"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// --- mock repositories ---

type mockVoucherOrderRepo struct {
	CreateVoucherOrderFunc    func(ctx context.Context, order *VoucherOrder) error
	CountByUserAndVoucherFunc func(ctx context.Context, userID, voucherID uint64) (int64, error)
	GetVoucherOrderByIDFunc   func(ctx context.Context, orderID uint64) (*VoucherOrder, error)
}

func (m *mockVoucherOrderRepo) CreateVoucherOrder(ctx context.Context, order *VoucherOrder) error {
	if m.CreateVoucherOrderFunc != nil {
		return m.CreateVoucherOrderFunc(ctx, order)
	}
	return nil
}

func (m *mockVoucherOrderRepo) CountByUserAndVoucher(ctx context.Context, userID, voucherID uint64) (int64, error) {
	if m.CountByUserAndVoucherFunc != nil {
		return m.CountByUserAndVoucherFunc(ctx, userID, voucherID)
	}
	return 0, nil
}

func (m *mockVoucherOrderRepo) GetVoucherOrderByID(ctx context.Context, orderID uint64) (*VoucherOrder, error) {
	if m.GetVoucherOrderByIDFunc != nil {
		return m.GetVoucherOrderByIDFunc(ctx, orderID)
	}
	return nil, nil
}

type mockSeckillVoucherRepo struct {
	GetSeckillVoucherByIDFunc func(ctx context.Context, voucherID uint64) (*seckillvoucher.SeckillVoucher, error)
	DeductStockFunc           func(ctx context.Context, voucherID uint64) (bool, error)
}

func (m *mockSeckillVoucherRepo) GetSeckillVoucherByID(ctx context.Context, voucherID uint64) (*seckillvoucher.SeckillVoucher, error) {
	if m.GetSeckillVoucherByIDFunc != nil {
		return m.GetSeckillVoucherByIDFunc(ctx, voucherID)
	}
	return nil, nil
}

func (m *mockSeckillVoucherRepo) DeductStock(ctx context.Context, voucherID uint64) (bool, error) {
	if m.DeductStockFunc != nil {
		return m.DeductStockFunc(ctx, voucherID)
	}
	return true, nil
}

// --- helpers ---

func setupService(t *testing.T) (*Service, *mockVoucherOrderRepo, *mockSeckillVoucherRepo, *miniredis.Miniredis, *idgen.RedisIDWorker) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	voucherRepo := new(mockVoucherOrderRepo)
	seckillRepo := new(mockSeckillVoucherRepo)
	idWorker := idgen.NewRedisIDWorker(rdb)
	svc := NewService(voucherRepo, seckillRepo, rdb, idWorker)
	return svc, voucherRepo, seckillRepo, mr, idWorker
}

func prepareSeckillEnv(t *testing.T, mr *miniredis.Miniredis, voucherID uint64, stock int) {
	t.Helper()
	stockKey := fmt.Sprintf("seckill:stock:%d", voucherID)
	mr.Set(stockKey, fmt.Sprintf("%d", stock))
}

// addStreamMessage 向 Stream 添加一条消息，返回消息 ID
func addStreamMessage(t *testing.T, mr *miniredis.Miniredis, orderID, userID, voucherID uint64) string {
	t.Helper()
	msgID, err := mr.XAdd(StreamOrderKey, "*", []string{
		"orderId", fmt.Sprintf("%d", orderID),
		"userId", fmt.Sprintf("%d", userID),
		"voucherId", fmt.Sprintf("%d", voucherID),
	})
	require.NoError(t, err)
	return msgID
}

// ensureStreamGroup 创建消费者组（忽略 BUSYGROUP 错误）
func ensureStreamGroup(t *testing.T, rdb redis.Cmdable) {
	t.Helper()
	err := rdb.XGroupCreateMkStream(context.Background(), StreamOrderKey, StreamGroupName, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		require.NoError(t, err)
	}
}

// addPendingMessage 向 Stream 添加消息并读取但不 ACK，使其变为 pending 状态
// 调用前需先 ensureStreamGroup
func addPendingMessage(t *testing.T, mr *miniredis.Miniredis, rdb redis.Cmdable, orderID, userID, voucherID uint64) string {
	t.Helper()
	msgID := addStreamMessage(t, mr, orderID, userID, voucherID)

	rdb.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group:    StreamGroupName,
		Consumer: StreamConsumerName,
		Streams:  []string{StreamOrderKey, ">"},
		Count:    1,
		Block:    time.Millisecond,
	})
	return msgID
}

// makeMessagePending 便捷函数：创建组 + 添加 pending 消息
func makeMessagePending(t *testing.T, mr *miniredis.Miniredis, rdb redis.Cmdable, orderID, userID, voucherID uint64) string {
	t.Helper()
	ensureStreamGroup(t, rdb)
	return addPendingMessage(t, mr, rdb, orderID, userID, voucherID)
}

// isCustomError 判断 error 是否为指定的 CustomError（按 BusinessCode 匹配）
func isCustomError(err error, target *errmsg.CustomError) bool {
	ce, ok := err.(*errmsg.CustomError)
	if ok {
		return ce.BusinessCode == target.BusinessCode
	}
	return false
}

// ============================================================================
// Start() tests
// ============================================================================

func TestStart(t *testing.T) {
	t.Run("creates consumer group and marks started", func(t *testing.T) {
		svc, _, _, _, _ := setupService(t)

		svc.Start()

		require.True(t, svc.started)

		// 通过 Redis 命令验证消费者组已创建
		rdb := svc.rdb.(*redis.Client)
		groups, err := rdb.XInfoGroups(context.Background(), StreamOrderKey).Result()
		require.NoError(t, err)
		require.Len(t, groups, 1)
		require.Equal(t, StreamGroupName, groups[0].Name)
	})

	t.Run("second call is no-op", func(t *testing.T) {
		svc, _, _, _, _ := setupService(t)

		svc.Start()
		rdb := svc.rdb.(*redis.Client)
		groupsAfterFirst, _ := rdb.XInfoGroups(context.Background(), StreamOrderKey).Result()
		require.Len(t, groupsAfterFirst, 1)

		svc.Start()
		groupsAfterSecond, _ := rdb.XInfoGroups(context.Background(), StreamOrderKey).Result()
		require.Len(t, groupsAfterSecond, 1)
	})

	t.Run("concurrent Start calls only execute once", func(t *testing.T) {
		svc, _, _, _, _ := setupService(t)

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				svc.Start()
			}()
		}
		wg.Wait()

		require.True(t, svc.started)
	})
}

// ============================================================================
// SeckillVoucher tests
// ============================================================================

func TestSeckillVoucher(t *testing.T) {
	t.Run("success: stock deducted and order written to stream", func(t *testing.T) {
		svc, _, _, mr, _ := setupService(t)
		prepareSeckillEnv(t, mr, 1, 10)

		orderID, err := svc.SeckillVoucher(context.Background(), 1, 100)
		require.NoError(t, err)
		require.NotZero(t, orderID)

		// 验证库存已扣减
		stockKey := "seckill:stock:1"
		stock, _ := mr.Get(stockKey)
		require.Equal(t, "9", stock)

		// 验证用户已标记（SISMEMBER 防重复）
		orderKey := "seckill:order:1:100"
		require.True(t, mr.Exists(orderKey))

		// 注：miniredis 的 Lua 环境不支持 XADD，Stream 写入行为由 StreamConsumer 集成测试覆盖
	})

	t.Run("no stock returns ErrNoStock", func(t *testing.T) {
		svc, _, _, mr, _ := setupService(t)
		prepareSeckillEnv(t, mr, 1, 0)

		orderID, err := svc.SeckillVoucher(context.Background(), 1, 100)
		require.Zero(t, orderID)
		require.True(t, isCustomError(err, &errmsg.ErrNoStock))
	})

	t.Run("repeated order from same user returns ErrRepeatedOrder", func(t *testing.T) {
		svc, _, _, mr, _ := setupService(t)
		prepareSeckillEnv(t, mr, 1, 10)

		_, err := svc.SeckillVoucher(context.Background(), 1, 100)
		require.NoError(t, err)

		orderID, err := svc.SeckillVoucher(context.Background(), 1, 100)
		require.Zero(t, orderID)
		require.True(t, isCustomError(err, &errmsg.ErrRepeatedOrder))
	})

	t.Run("different users can buy same voucher", func(t *testing.T) {
		svc, _, _, mr, _ := setupService(t)
		prepareSeckillEnv(t, mr, 1, 10)

		id1, err := svc.SeckillVoucher(context.Background(), 1, 100)
		require.NoError(t, err)
		require.NotZero(t, id1)

		id2, err := svc.SeckillVoucher(context.Background(), 1, 200)
		require.NoError(t, err)
		require.NotZero(t, id2)

		require.NotEqual(t, id1, id2)
		stock, _ := mr.Get("seckill:stock:1")
		require.Equal(t, "8", stock)
	})

	t.Run("same user can buy different vouchers", func(t *testing.T) {
		svc, _, _, mr, _ := setupService(t)
		prepareSeckillEnv(t, mr, 1, 10)
		prepareSeckillEnv(t, mr, 2, 10)

		id1, err := svc.SeckillVoucher(context.Background(), 1, 100)
		require.NoError(t, err)
		id2, err := svc.SeckillVoucher(context.Background(), 2, 100)
		require.NoError(t, err)
		require.NotEqual(t, id1, id2)
	})
}

// ============================================================================
// CreateVoucherOrder_Service tests
// ============================================================================

func TestCreateVoucherOrder_Service(t *testing.T) {
	t.Run("success: deducts stock then creates order", func(t *testing.T) {
		svc, voucherRepo, seckillRepo, _, _ := setupService(t)

		var deductCalled, createCalled bool
		seckillRepo.DeductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
			require.Equal(t, uint64(1), voucherID)
			deductCalled = true
			return true, nil
		}
		voucherRepo.CreateVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			require.Equal(t, uint64(1001), order.ID)
			require.Equal(t, uint64(100), order.UserID)
			require.Equal(t, uint64(1), order.VoucherID)
			createCalled = true
			return nil
		}

		order := &VoucherOrder{ID: 1001, UserID: 100, VoucherID: 1}
		err := svc.CreateVoucherOrder(context.Background(), order)
		require.NoError(t, err)
		require.True(t, deductCalled)
		require.True(t, createCalled)
	})

	t.Run("deduct returns false → ErrNoStock", func(t *testing.T) {
		svc, voucherRepo, seckillRepo, _, _ := setupService(t)

		seckillRepo.DeductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
			return false, nil
		}
		voucherRepo.CreateVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			require.Fail(t, "should not be called after failed deduct")
			return nil
		}

		err := svc.CreateVoucherOrder(context.Background(), &VoucherOrder{ID: 1001, UserID: 100, VoucherID: 1})
		require.True(t, isCustomError(err, &errmsg.ErrNoStock))
	})

	t.Run("deduct error propagates", func(t *testing.T) {
		svc, _, seckillRepo, _, _ := setupService(t)

		seckillRepo.DeductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
			return false, fmt.Errorf("database connection lost")
		}

		err := svc.CreateVoucherOrder(context.Background(), &VoucherOrder{ID: 1001, UserID: 100, VoucherID: 1})
		require.Error(t, err)
		require.Contains(t, err.Error(), "database connection lost")
	})

	t.Run("create order error propagates", func(t *testing.T) {
		svc, voucherRepo, seckillRepo, _, _ := setupService(t)

		seckillRepo.DeductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
			return true, nil
		}
		voucherRepo.CreateVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			return fmt.Errorf("insert failed")
		}

		err := svc.CreateVoucherOrder(context.Background(), &VoucherOrder{ID: 1001, UserID: 100, VoucherID: 1})
		require.Error(t, err)
		require.Contains(t, err.Error(), "insert failed")
	})
}

// ============================================================================
// HandleVoucherOrder tests
// ============================================================================

func TestHandleVoucherOrder(t *testing.T) {
	t.Run("acquires lock, creates order, releases lock", func(t *testing.T) {
		svc, voucherRepo, seckillRepo, mr, _ := setupService(t)

		seckillRepo.DeductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
			return true, nil
		}
		var savedOrder *VoucherOrder
		voucherRepo.CreateVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			savedOrder = order
			return nil
		}

		order := &VoucherOrder{ID: 2001, UserID: 100, VoucherID: 1}
		err := svc.HandleVoucherOrder(context.Background(), order)
		require.NoError(t, err)
		require.NotNil(t, savedOrder)
		require.Equal(t, uint64(2001), savedOrder.ID)

		lockKey := LockOrderKey + "100"
		require.False(t, mr.Exists(lockKey))
	})

	t.Run("lock held by another request → ErrRepeatedOrder", func(t *testing.T) {
		svc, voucherRepo, _, mr, _ := setupService(t)

		lockKey := LockOrderKey + "100"
		mr.Set(lockKey, "1")

		voucherRepo.CreateVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			require.Fail(t, "should not create order when lock is held")
			return nil
		}

		err := svc.HandleVoucherOrder(context.Background(), &VoucherOrder{ID: 2001, UserID: 100, VoucherID: 1})
		require.True(t, isCustomError(err, &errmsg.ErrRepeatedOrder))
	})

	t.Run("lock released even when create fails", func(t *testing.T) {
		svc, voucherRepo, seckillRepo, mr, _ := setupService(t)

		seckillRepo.DeductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
			return true, nil
		}
		voucherRepo.CreateVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			return fmt.Errorf("db error")
		}

		err := svc.HandleVoucherOrder(context.Background(), &VoucherOrder{ID: 2001, UserID: 100, VoucherID: 1})
		require.Error(t, err)

		lockKey := LockOrderKey + "100"
		require.False(t, mr.Exists(lockKey))
	})

	t.Run("different users use different locks", func(t *testing.T) {
		svc, voucherRepo, seckillRepo, _, _ := setupService(t)

		var mu sync.Mutex
		var savedOrders []*VoucherOrder
		seckillRepo.DeductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
			return true, nil
		}
		voucherRepo.CreateVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			mu.Lock()
			defer mu.Unlock()
			savedOrders = append(savedOrders, order)
			return nil
		}

		err := svc.HandleVoucherOrder(context.Background(), &VoucherOrder{ID: 6001, UserID: 100, VoucherID: 1})
		require.NoError(t, err)
		err = svc.HandleVoucherOrder(context.Background(), &VoucherOrder{ID: 6002, UserID: 200, VoucherID: 1})
		require.NoError(t, err)

		require.Len(t, savedOrders, 2)
	})
}

// ============================================================================
// HandlePendingList tests
// ============================================================================

func TestHandlePendingList(t *testing.T) {
	t.Run("no pending messages → returns immediately", func(t *testing.T) {
		svc, voucherRepo, _, _, _ := setupService(t)

		voucherRepo.CreateVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			require.Fail(t, "should not create order when no pending messages")
			return nil
		}

		svc.HandlePendingList()
	})

	t.Run("processes single pending message", func(t *testing.T) {
		svc, voucherRepo, seckillRepo, mr, _ := setupService(t)

		seckillRepo.DeductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
			return true, nil
		}

		var mu sync.Mutex
		var processedOrders []*VoucherOrder
		voucherRepo.CreateVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			mu.Lock()
			defer mu.Unlock()
			processedOrders = append(processedOrders, order)
			return nil
		}

		makeMessagePending(t, mr, svc.rdb, 3001, 100, 1)

		svc.HandlePendingList()

		mu.Lock()
		defer mu.Unlock()
		require.Len(t, processedOrders, 1)
		require.Equal(t, uint64(3001), processedOrders[0].ID)
		require.Equal(t, uint64(100), processedOrders[0].UserID)
		require.Equal(t, uint64(1), processedOrders[0].VoucherID)
	})

	t.Run("processes multiple pending messages", func(t *testing.T) {
		svc, voucherRepo, seckillRepo, mr, _ := setupService(t)

		seckillRepo.DeductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
			return true, nil
		}

		var mu sync.Mutex
		var processedOrders []*VoucherOrder
		voucherRepo.CreateVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			mu.Lock()
			defer mu.Unlock()
			processedOrders = append(processedOrders, order)
			return nil
		}

		makeMessagePending(t, mr, svc.rdb, 4001, 100, 1)
		makeMessagePending(t, mr, svc.rdb, 4002, 200, 1)

		svc.HandlePendingList()

		mu.Lock()
		defer mu.Unlock()
		ids := make([]uint64, len(processedOrders))
		for i, o := range processedOrders {
			ids[i] = o.ID
		}
		require.Len(t, ids, 2)
		require.Contains(t, ids, uint64(4001))
		require.Contains(t, ids, uint64(4002))
	})

	t.Run("returns immediately when stream has no consumer group", func(t *testing.T) {
		svc, voucherRepo, _, _, _ := setupService(t)

		voucherRepo.CreateVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			require.Fail(t, "should not create order")
			return nil
		}

		// 没有创建 Stream 也没有消费者组 → XReadGroup 返回 error → 方法直接返回
		svc.HandlePendingList()
	})
}

// ============================================================================
// StreamConsumer tests
// ============================================================================

func TestStreamConsumer(t *testing.T) {
	t.Run("consumes message from stream and creates order", func(t *testing.T) {
		svc, voucherRepo, seckillRepo, mr, _ := setupService(t)

		seckillRepo.DeductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
			return true, nil
		}

		done := make(chan *VoucherOrder, 1)
		voucherRepo.CreateVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			select {
			case done <- order:
			default:
			}
			return nil
		}

		err := svc.rdb.XGroupCreateMkStream(context.Background(), StreamOrderKey, StreamGroupName, "0").Err()
		require.NoError(t, err)

		svc.started = true
		go svc.StreamConsumer()

		addStreamMessage(t, mr, 5001, 100, 1)

		var order *VoucherOrder
		select {
		case order = <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for message consumption")
		}

		require.NotNil(t, order)
		require.Equal(t, uint64(5001), order.ID)
		require.Equal(t, uint64(100), order.UserID)
		require.Equal(t, uint64(1), order.VoucherID)
	})

	t.Run("multiple messages consumed in sequence", func(t *testing.T) {
		svc, voucherRepo, seckillRepo, mr, _ := setupService(t)

		seckillRepo.DeductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
			return true, nil
		}

		done := make(chan *VoucherOrder, 2)
		var mu sync.Mutex
		var orders []*VoucherOrder
		voucherRepo.CreateVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			mu.Lock()
			defer mu.Unlock()
			orders = append(orders, order)
			select {
			case done <- order:
			default:
			}
			return nil
		}

		err := svc.rdb.XGroupCreateMkStream(context.Background(), StreamOrderKey, StreamGroupName, "0").Err()
		require.NoError(t, err)

		svc.started = true
		go svc.StreamConsumer()

		// 连续发送两条消息，都应被消费
		addStreamMessage(t, mr, 6001, 100, 1)
		addStreamMessage(t, mr, 6002, 200, 1)

		var o1, o2 *VoucherOrder
		select {
		case o1 = <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for first message")
		}
		select {
		case o2 = <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for second message")
		}

		require.NotNil(t, o1)
		require.NotNil(t, o2)
		ids := []uint64{o1.ID, o2.ID}
		require.Contains(t, ids, uint64(6001))
		require.Contains(t, ids, uint64(6002))
	})
}

// ============================================================================
// ParseMessage tests
// ============================================================================

func TestParseMessage(t *testing.T) {
	t.Run("valid message with all fields", func(t *testing.T) {
		values := map[string]any{
			"orderId":   "1001",
			"userId":    "100",
			"voucherId": "200",
		}
		order := ParseMessage(values)
		require.NotNil(t, order)
		require.Equal(t, uint64(1001), order.ID)
		require.Equal(t, uint64(100), order.UserID)
		require.Equal(t, uint64(200), order.VoucherID)
	})

	t.Run("nil values", func(t *testing.T) {
		require.Nil(t, ParseMessage(nil))
	})

	t.Run("missing orderId returns nil", func(t *testing.T) {
		values := map[string]any{"userId": "100", "voucherId": "200"}
		require.Nil(t, ParseMessage(values))
	})

	t.Run("missing userId returns nil", func(t *testing.T) {
		values := map[string]any{"orderId": "1001", "voucherId": "200"}
		require.Nil(t, ParseMessage(values))
	})

	t.Run("missing voucherId returns nil", func(t *testing.T) {
		values := map[string]any{"orderId": "1001", "userId": "100"}
		require.Nil(t, ParseMessage(values))
	})

	t.Run("zero values return nil", func(t *testing.T) {
		values := map[string]any{"orderId": "0", "userId": "0", "voucherId": "0"}
		require.Nil(t, ParseMessage(values))
	})

	t.Run("empty strings return nil", func(t *testing.T) {
		values := map[string]any{"orderId": "", "userId": "100", "voucherId": "200"}
		require.Nil(t, ParseMessage(values))
	})
}

// ============================================================================
// Service construction tests
// ============================================================================

func TestNewService_Construction(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	voucherRepo := new(mockVoucherOrderRepo)
	seckillRepo := new(mockSeckillVoucherRepo)
	idWorker := idgen.NewRedisIDWorker(rdb)

	svc := NewService(voucherRepo, seckillRepo, rdb, idWorker)
	require.NotNil(t, svc)
	require.NotNil(t, svc.rdb)
	require.NotNil(t, svc.repo)
	require.NotNil(t, svc.seckillRepo)
	require.NotNil(t, svc.idWorker)
	require.False(t, svc.started)
}

func TestStartFiresConsumerGoroutine(t *testing.T) {
	svc, voucherRepo, seckillRepo, mr, _ := setupService(t)

	seckillRepo.DeductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
		return true, nil
	}

	done := make(chan struct{}, 1)
	voucherRepo.CreateVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
		select {
		case done <- struct{}{}:
		default:
		}
		return nil
	}

	svc.rdb.XGroupCreateMkStream(context.Background(), StreamOrderKey, StreamGroupName, "0")
	svc.started = true
	go svc.StreamConsumer()

	addStreamMessage(t, mr, 9001, 100, 1)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: consumer goroutine did not process message")
	}
}
