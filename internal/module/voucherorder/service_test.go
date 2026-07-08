package voucherorder

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"dianping/internal/module/seckillvoucher"
	"dianping/internal/tx"
	"dianping/pkg/errmsg"
	"dianping/pkg/idgen"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Mock repositories and dependencies
// =============================================================================

type mockVoucherOrderRepo struct {
	createVoucherOrderFunc    func(ctx context.Context, order *VoucherOrder) error
	countByUserAndVoucherFunc func(ctx context.Context, userID uint64, voucherID uint64) (int64, error)
	getVoucherOrderByIDFunc   func(ctx context.Context, orderID uint64) (*VoucherOrder, error)
}

func (m *mockVoucherOrderRepo) CreateVoucherOrder(ctx context.Context, order *VoucherOrder) error {
	if m.createVoucherOrderFunc != nil {
		return m.createVoucherOrderFunc(ctx, order)
	}
	return nil
}

func (m *mockVoucherOrderRepo) CountByUserAndVoucher(ctx context.Context, userID uint64, voucherID uint64) (int64, error) {
	if m.countByUserAndVoucherFunc != nil {
		return m.countByUserAndVoucherFunc(ctx, userID, voucherID)
	}
	return 0, nil
}

func (m *mockVoucherOrderRepo) GetVoucherOrderByID(ctx context.Context, orderID uint64) (*VoucherOrder, error) {
	if m.getVoucherOrderByIDFunc != nil {
		return m.getVoucherOrderByIDFunc(ctx, orderID)
	}
	return nil, nil
}

type mockSeckillVoucherRepo struct {
	getSeckillVoucherByIDFunc func(ctx context.Context, voucherID uint64) (*seckillvoucher.SeckillVoucher, error)
	deductStockFunc           func(ctx context.Context, voucherID uint64) (bool, error)
}

func (m *mockSeckillVoucherRepo) GetSeckillVoucherByID(ctx context.Context, voucherID uint64) (*seckillvoucher.SeckillVoucher, error) {
	if m.getSeckillVoucherByIDFunc != nil {
		return m.getSeckillVoucherByIDFunc(ctx, voucherID)
	}
	return nil, nil
}

func (m *mockSeckillVoucherRepo) DeductStock(ctx context.Context, voucherID uint64) (bool, error) {
	if m.deductStockFunc != nil {
		return m.deductStockFunc(ctx, voucherID)
	}
	return true, nil
}

type mockTxManager struct {
	transactionFunc func(ctx context.Context, fn func(ctx context.Context) error) error
}

func (m *mockTxManager) Transaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if m.transactionFunc != nil {
		return m.transactionFunc(ctx, fn)
	}
	return fn(ctx)
}

// =============================================================================
// Setup
// =============================================================================

func setUpVoucherOrderService(t *testing.T) (*Service, *mockVoucherOrderRepo, *mockSeckillVoucherRepo, *mockTxManager, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() {
		require.NoError(t, rdb.Close())
	})

	orderRepo := new(mockVoucherOrderRepo)
	seckillRepo := new(mockSeckillVoucherRepo)
	txMgr := new(mockTxManager)
	idWorker := idgen.NewRedisIDWorker(rdb)

	svc := NewService(orderRepo, seckillRepo, rdb, idWorker, txMgr)
	return svc, orderRepo, seckillRepo, txMgr, mr
}

// =============================================================================
// SeckillVoucher
// =============================================================================

func TestService_SeckillVoucher(t *testing.T) {
	t.Run("seckill successfully", func(t *testing.T) {
		svc, _, _, _, mr := setUpVoucherOrderService(t)
		ctx := context.Background()

		// Pre-set stock for the voucher in Redis
		stockKey := "seckill:stock:100"
		mr.Set(stockKey, "10")

		orderID, err := svc.SeckillVoucher(ctx, 100, 1)
		require.NoError(t, err)
		require.NotZero(t, orderID)

		// Stock should be decremented by 1
		stockAfter, _ := mr.Get(stockKey)
		require.Equal(t, "9", stockAfter)

		// User should be in the order set
		orderKey := fmt.Sprintf("seckill:order:100:%d", 1)
		isMember, _ := mr.SIsMember(orderKey, "1")
		require.True(t, isMember)
	})

	t.Run("seckill with no stock returns ErrNoStock", func(t *testing.T) {
		svc, _, _, _, mr := setUpVoucherOrderService(t)
		ctx := context.Background()

		// Pre-set stock to zero
		stockKey := "seckill:stock:100"
		mr.Set(stockKey, "0")

		orderID, err := svc.SeckillVoucher(ctx, 100, 1)
		require.Error(t, err)
		require.Equal(t, int64(0), orderID)
		require.Equal(t, &errmsg.ErrNoStock, err)
	})

	t.Run("seckill with no stock key returns ErrNoStock", func(t *testing.T) {
		svc, _, _, _, _ := setUpVoucherOrderService(t)
		ctx := context.Background()

		// No stock key set at all
		orderID, err := svc.SeckillVoucher(ctx, 100, 1)
		require.Error(t, err)
		require.Equal(t, int64(0), orderID)
		require.Equal(t, &errmsg.ErrNoStock, err)
	})

	t.Run("repeated order returns ErrRepeatedOrder", func(t *testing.T) {
		svc, _, _, _, mr := setUpVoucherOrderService(t)
		ctx := context.Background()

		stockKey := "seckill:stock:100"
		mr.Set(stockKey, "10")

		// First order succeeds
		firstID, err := svc.SeckillVoucher(ctx, 100, 1)
		require.NoError(t, err)
		require.NotZero(t, firstID)

		// Second order with same user and voucher should fail
		orderID, err := svc.SeckillVoucher(ctx, 100, 1)
		require.Error(t, err)
		require.Equal(t, int64(0), orderID)
		require.Equal(t, &errmsg.ErrRepeatedOrder, err)
	})

	t.Run("different users can seckill same voucher", func(t *testing.T) {
		svc, _, _, _, mr := setUpVoucherOrderService(t)
		ctx := context.Background()

		stockKey := "seckill:stock:100"
		mr.Set(stockKey, "5")

		// User 1 orders
		id1, err := svc.SeckillVoucher(ctx, 100, 1)
		require.NoError(t, err)
		require.NotZero(t, id1)

		// User 2 orders
		id2, err := svc.SeckillVoucher(ctx, 100, 2)
		require.NoError(t, err)
		require.NotZero(t, id2)

		stockAfter, _ := mr.Get(stockKey)
		require.Equal(t, "3", stockAfter)
	})
}

// =============================================================================
// CreateVoucherOrder
// =============================================================================

func TestService_CreateVoucherOrder(t *testing.T) {
	t.Run("create order successfully in transaction", func(t *testing.T) {
		svc, orderRepo, seckillRepo, txMgr, _ := setUpVoucherOrderService(t)
		ctx := context.Background()

		// Mock txManager to execute the transaction function directly
		txMgr.transactionFunc = func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		}

		seckillRepo.deductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
			require.Equal(t, uint64(100), voucherID)
			return true, nil
		}

		orderRepo.createVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			require.Equal(t, uint64(100), order.VoucherID)
			require.Equal(t, uint64(1), order.UserID)
			return nil
		}

		order := &VoucherOrder{ID: 12345, UserID: 1, VoucherID: 100, PayType: 1, Status: 0}
		err := svc.CreateVoucherOrder(ctx, order)
		require.NoError(t, err)
	})

	t.Run("create order with stock deduction failure returns ErrNoStock", func(t *testing.T) {
		svc, orderRepo, seckillRepo, txMgr, _ := setUpVoucherOrderService(t)
		ctx := context.Background()

		txMgr.transactionFunc = func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		}

		seckillRepo.deductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
			return false, nil // no stock
		}

		orderRepo.createVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			t.Fatalf("CreateVoucherOrder should not be called when stock deduction fails")
			return nil
		}

		order := &VoucherOrder{ID: 1, UserID: 1, VoucherID: 100}
		err := svc.CreateVoucherOrder(ctx, order)
		require.Error(t, err)
		require.Equal(t, &errmsg.ErrNoStock, err)
	})

	t.Run("create order with DB error during deduction", func(t *testing.T) {
		svc, _, seckillRepo, txMgr, _ := setUpVoucherOrderService(t)
		ctx := context.Background()

		txMgr.transactionFunc = func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		}

		dbErr := errors.New("database error")
		seckillRepo.deductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
			return false, dbErr
		}

		order := &VoucherOrder{ID: 1, UserID: 1, VoucherID: 100}
		err := svc.CreateVoucherOrder(ctx, order)
		require.Error(t, err)
		require.Equal(t, dbErr, err)
	})

	t.Run("create order with order insertion failure", func(t *testing.T) {
		svc, orderRepo, seckillRepo, txMgr, _ := setUpVoucherOrderService(t)
		ctx := context.Background()

		txMgr.transactionFunc = func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		}

		seckillRepo.deductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
			return true, nil
		}

		orderRepo.createVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			return errors.New("insert failed")
		}

		order := &VoucherOrder{ID: 1, UserID: 1, VoucherID: 100}
		err := svc.CreateVoucherOrder(ctx, order)
		require.Error(t, err)
	})
}

// =============================================================================
// GetVoucherOrderByID
// =============================================================================

func TestService_GetVoucherOrderByID(t *testing.T) {
	t.Run("get order by ID successfully", func(t *testing.T) {
		svc, orderRepo, _, _, _ := setUpVoucherOrderService(t)
		ctx := context.Background()

		expected := &VoucherOrder{ID: 12345, UserID: 1, VoucherID: 100, PayType: 1, Status: 0}
		orderRepo.getVoucherOrderByIDFunc = func(ctx context.Context, orderID uint64) (*VoucherOrder, error) {
			require.Equal(t, uint64(12345), orderID)
			return expected, nil
		}

		order, err := svc.GetVoucherOrderByID(ctx, 12345)
		require.NoError(t, err)
		require.NotNil(t, order)
		require.Equal(t, expected.ID, order.ID)
		require.Equal(t, expected.UserID, order.UserID)
	})

	t.Run("order not found returns ErrOrderNotFound", func(t *testing.T) {
		svc, orderRepo, _, _, _ := setUpVoucherOrderService(t)
		ctx := context.Background()

		orderRepo.getVoucherOrderByIDFunc = func(ctx context.Context, orderID uint64) (*VoucherOrder, error) {
			return nil, nil
		}

		order, err := svc.GetVoucherOrderByID(ctx, 99999)
		require.Error(t, err)
		require.Nil(t, order)
		require.Equal(t, &errmsg.ErrOrderNotFound, err)
	})

	t.Run("repository DB error is propagated", func(t *testing.T) {
		svc, orderRepo, _, _, _ := setUpVoucherOrderService(t)
		ctx := context.Background()

		dbErr := errors.New("db connection lost")
		orderRepo.getVoucherOrderByIDFunc = func(ctx context.Context, orderID uint64) (*VoucherOrder, error) {
			return nil, dbErr
		}

		order, err := svc.GetVoucherOrderByID(ctx, 12345)
		require.Error(t, err)
		require.Nil(t, order)
	})
}

// =============================================================================
// HandleVoucherOrder
// =============================================================================

func TestService_HandleVoucherOrder(t *testing.T) {
	t.Run("handle order successfully with distributed lock", func(t *testing.T) {
		svc, orderRepo, seckillRepo, txMgr, _ := setUpVoucherOrderService(t)
		ctx := context.Background()

		txMgr.transactionFunc = func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		}

		seckillRepo.deductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
			return true, nil
		}

		orderRepo.createVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			return nil
		}

		order := &VoucherOrder{ID: 12345, UserID: 1, VoucherID: 100}
		err := svc.HandleVoucherOrder(ctx, order)
		require.NoError(t, err)
	})

	t.Run("repeated order with same userID fails at lock level", func(t *testing.T) {
		svc, _, _, txMgr, mr := setUpVoucherOrderService(t)
		ctx := context.Background()

		// Pre-set the lock for this user
		lockKey := LockOrderKey + strconv.FormatUint(1, 10)
		mr.Set(lockKey, "1")

		// These should not be called since lock acquisition fails
		txMgr.transactionFunc = func(ctx context.Context, fn func(ctx context.Context) error) error {
			t.Fatalf("transaction should not be called when lock fails")
			return nil
		}

		order := &VoucherOrder{ID: 12345, UserID: 1, VoucherID: 100}
		err := svc.HandleVoucherOrder(ctx, order)
		require.Error(t, err)
		require.Equal(t, &errmsg.ErrRepeatedOrder, err)
	})

	t.Run("different users can obtain independent locks", func(t *testing.T) {
		svc, orderRepo, seckillRepo, txMgr, _ := setUpVoucherOrderService(t)
		ctx := context.Background()

		txMgr.transactionFunc = func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		}

		seckillRepo.deductStockFunc = func(ctx context.Context, voucherID uint64) (bool, error) {
			return true, nil
		}

		callCount := 0
		orderRepo.createVoucherOrderFunc = func(ctx context.Context, order *VoucherOrder) error {
			callCount++
			return nil
		}

		// User 1
		err := svc.HandleVoucherOrder(ctx, &VoucherOrder{ID: 1, UserID: 1, VoucherID: 100})
		require.NoError(t, err)

		// User 2 — should succeed with independent lock
		err = svc.HandleVoucherOrder(ctx, &VoucherOrder{ID: 2, UserID: 2, VoucherID: 100})
		require.NoError(t, err)

		require.Equal(t, 2, callCount)
	})
}

// =============================================================================
// ParseMessage
// =============================================================================

func TestParseMessage(t *testing.T) {
	t.Run("parse valid message", func(t *testing.T) {
		values := map[string]interface{}{
			"user_id":    "1",
			"voucher_id": "100",
			"order_id":   "12345",
		}
		order := ParseMessage(values)
		require.NotNil(t, order)
		require.Equal(t, uint64(12345), order.ID)
		require.Equal(t, uint64(1), order.UserID)
		require.Equal(t, uint64(100), order.VoucherID)
	})

	t.Run("parse nil values returns nil", func(t *testing.T) {
		order := ParseMessage(nil)
		require.Nil(t, order)
	})

	t.Run("parse missing user_id returns nil", func(t *testing.T) {
		values := map[string]interface{}{
			"voucher_id": "100",
			"order_id":   "12345",
		}
		order := ParseMessage(values)
		require.Nil(t, order)
	})

	t.Run("parse missing voucher_id returns nil", func(t *testing.T) {
		values := map[string]interface{}{
			"user_id":  "1",
			"order_id": "12345",
		}
		order := ParseMessage(values)
		require.Nil(t, order)
	})

	t.Run("parse missing order_id returns nil", func(t *testing.T) {
		values := map[string]interface{}{
			"user_id":    "1",
			"voucher_id": "100",
		}
		order := ParseMessage(values)
		require.Nil(t, order)
	})

	t.Run("parse zero values returns nil", func(t *testing.T) {
		values := map[string]interface{}{
			"user_id":    "0",
			"voucher_id": "100",
			"order_id":   "12345",
		}
		order := ParseMessage(values)
		require.Nil(t, order)
	})

	t.Run("parse non-string values", func(t *testing.T) {
		values := map[string]interface{}{
			"user_id":    1,
			"voucher_id": 100,
			"order_id":   12345,
		}
		// Non-string values convert to empty strings via strVal
		order := ParseMessage(values)
		require.Nil(t, order)
	})
}

// Ensure mocks satisfy interfaces
var _ VoucherOrderRepository = (*mockVoucherOrderRepo)(nil)
var _ SeckillVoucherRepository = (*mockSeckillVoucherRepo)(nil)
var _ tx.Manager = (*mockTxManager)(nil)

// Ensure idgen is importable
var _ = idgen.NewRedisIDWorker
