package voucher

import (
	"context"
	"dianping/internal/module/seckillvoucher"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type mockVoucherRepo struct {
	createVoucherFunc       func(ctx context.Context, voucher *Voucher) error
	getVoucherByIDFunc      func(ctx context.Context, id uint64) (*Voucher, error)
	getByShopIDFunc         func(ctx context.Context, shopID uint64) ([]Voucher, error)
	createSeckillVoucherFunc func(ctx context.Context, v *Voucher, sv *seckillvoucher.SeckillVoucher) error
}

func (m *mockVoucherRepo) CreateVoucher(ctx context.Context, voucher *Voucher) error {
	if m.createVoucherFunc != nil {
		return m.createVoucherFunc(ctx, voucher)
	}
	return nil
}

func (m *mockVoucherRepo) GetVoucherByID(ctx context.Context, id uint64) (*Voucher, error) {
	if m.getVoucherByIDFunc != nil {
		return m.getVoucherByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockVoucherRepo) GetByShopID(ctx context.Context, shopID uint64) ([]Voucher, error) {
	if m.getByShopIDFunc != nil {
		return m.getByShopIDFunc(ctx, shopID)
	}
	return nil, nil
}

func (m *mockVoucherRepo) CreateSeckillVoucher(ctx context.Context, v *Voucher, sv *seckillvoucher.SeckillVoucher) error {
	if m.createSeckillVoucherFunc != nil {
		return m.createSeckillVoucherFunc(ctx, v, sv)
	}
	return nil
}

func setupService(t *testing.T) (*Service, *mockVoucherRepo, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	repo := new(mockVoucherRepo)
	svc := NewService(repo, rdb)
	return svc, repo, mr
}

func TestCreateVoucher_Service(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.createVoucherFunc = func(ctx context.Context, v *Voucher) error {
			v.ID = 100
			return nil
		}

		id, err := srv.CreateVoucher(context.Background(), &CreateVoucherReq{
			ShopID:      1,
			Title:       "测试券",
			SubTitle:    "副标题",
			Rules:       "规则",
			PayValue:    80,
			ActualValue: 100,
			Type:        0,
			Stock:       50,
			BeginTime:   time.Now(),
			EndTime:     time.Now().Add(24 * time.Hour),
		})
		require.NoError(t, err)
		require.Equal(t, uint64(100), id)
	})

	t.Run("repository error", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.createVoucherFunc = func(ctx context.Context, v *Voucher) error {
			return fmt.Errorf("db insert failed")
		}

		id, err := srv.CreateVoucher(context.Background(), &CreateVoucherReq{
			ShopID:      1,
			Title:       "测试券",
			SubTitle:    "副标题",
			Rules:       "规则",
			PayValue:    80,
			ActualValue: 100,
			Type:        0,
			Stock:       50,
			BeginTime:   time.Now(),
			EndTime:     time.Now().Add(24 * time.Hour),
		})
		require.Error(t, err)
		require.Equal(t, uint64(0), id)
		require.Contains(t, err.Error(), "db insert failed")
	})
}

func TestCreateSeckillVoucher_Service(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv, repo, mr := setupService(t)
		repo.createSeckillVoucherFunc = func(ctx context.Context, v *Voucher, sv *seckillvoucher.SeckillVoucher) error {
			v.ID = 200
			sv.VoucherID = 200
			return nil
		}

		id, err := srv.CreateSeckillVoucher(context.Background(), &CreateVoucherReq{
			ShopID:      1,
			Title:       "秒杀券",
			SubTitle:    "限时秒杀",
			Rules:       "秒杀规则",
			PayValue:    50,
			ActualValue: 100,
			Type:        1,
			Stock:       10,
			BeginTime:   time.Now(),
			EndTime:     time.Now().Add(1 * time.Hour),
		})
		require.NoError(t, err)
		require.Equal(t, uint64(200), id)

		stockKey := fmt.Sprintf("%s%d", SeckillStockKey, 200)
		stock, err := mr.Get(stockKey)
		require.NoError(t, err)
		require.Equal(t, "10", stock)
	})

	t.Run("repository error", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.createSeckillVoucherFunc = func(ctx context.Context, v *Voucher, sv *seckillvoucher.SeckillVoucher) error {
			return fmt.Errorf("transaction failed")
		}

		id, err := srv.CreateSeckillVoucher(context.Background(), &CreateVoucherReq{
			ShopID:      1,
			Title:       "秒杀券",
			SubTitle:    "限时秒杀",
			Rules:       "秒杀规则",
			PayValue:    50,
			ActualValue: 100,
			Type:        1,
			Stock:       10,
			BeginTime:   time.Now(),
			EndTime:     time.Now().Add(1 * time.Hour),
		})
		require.Error(t, err)
		require.Equal(t, uint64(0), id)
		require.Contains(t, err.Error(), "transaction failed")
	})
}

func TestGetVoucherByShopID_Service(t *testing.T) {
	t.Run("cache miss queries database", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		now := time.Now()
		repo.getByShopIDFunc = func(ctx context.Context, shopID uint64) ([]Voucher, error) {
			require.Equal(t, uint64(1), shopID)
			return []Voucher{
				{
					ID:          1,
					ShopID:      1,
					Title:       "优惠券A",
					SubTitle:    "副标题A",
					Rules:       "规则A",
					PayValue:    80,
					ActualValue: 100,
					Type:        0,
					Status:      1,
					Stock:       50,
					BeginTime:   now,
					EndTime:     now.Add(24 * time.Hour),
				},
			}, nil
		}

		result, err := srv.GetVoucherByShopID(context.Background(), 1)
		require.NoError(t, err)
		require.Len(t, result, 1)
		require.Equal(t, "优惠券A", result[0].Title)
	})

	t.Run("cache hit returns cached data", func(t *testing.T) {
		srv, repo, mr := setupService(t)
		now := time.Now()

		// 确保 repo 不被调用 —— 命中缓存
		repo.getByShopIDFunc = func(ctx context.Context, shopID uint64) ([]Voucher, error) {
			t.Fatal("should not call database when cache hits")
			return nil, nil
		}

		vouchers := []Voucher{{
			ID:          2,
			ShopID:      1,
			Title:       "缓存券",
			SubTitle:    "缓存副标题",
			Rules:       "缓存规则",
			PayValue:    30,
			ActualValue: 50,
			Type:        1,
			Status:      1,
			Stock:       20,
			BeginTime:   now,
			EndTime:     now.Add(24 * time.Hour),
		}}
		key := fmt.Sprintf("%s%d", CacheShopVoucherKey, 1)
		bytes, _ := json.Marshal(vouchers)
		mr.Set(key, string(bytes))

		result, err := srv.GetVoucherByShopID(context.Background(), 1)
		require.NoError(t, err)
		require.Len(t, result, 1)
		require.Equal(t, "缓存券", result[0].Title)
	})

	t.Run("cache empty marker returns empty list", func(t *testing.T) {
		srv, repo, mr := setupService(t)

		repo.getByShopIDFunc = func(ctx context.Context, shopID uint64) ([]Voucher, error) {
			t.Fatal("should not call database when empty marker is cached")
			return nil, nil
		}

		key := fmt.Sprintf("%s%d", CacheShopVoucherKey, 999)
		mr.Set(key, "")

		result, err := srv.GetVoucherByShopID(context.Background(), 999)
		require.NoError(t, err)
		require.Empty(t, result)
	})

	t.Run("database returns nil result empty list", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.getByShopIDFunc = func(ctx context.Context, shopID uint64) ([]Voucher, error) {
			return nil, nil
		}

		result, err := srv.GetVoucherByShopID(context.Background(), 999)
		require.NoError(t, err)
		require.Empty(t, result)
	})

	t.Run("database error", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.getByShopIDFunc = func(ctx context.Context, shopID uint64) ([]Voucher, error) {
			return nil, fmt.Errorf("database connection lost")
		}

		result, err := srv.GetVoucherByShopID(context.Background(), 1)
		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "database connection lost")
	})
}

func TestToVoucherRespList(t *testing.T) {
	now := time.Now()
	vouchers := []Voucher{
		{
			ID:          1,
			ShopID:      1,
			Title:       "券A",
			SubTitle:    "副A",
			Rules:       "规则A",
			PayValue:    80,
			ActualValue: 100,
			Type:        0,
			Status:      1,
			Stock:       50,
			BeginTime:   now,
			EndTime:     now.Add(24 * time.Hour),
		},
		{
			ID:          2,
			ShopID:      1,
			Title:       "券B",
			SubTitle:    "副B",
			Rules:       "规则B",
			PayValue:    40,
			ActualValue: 50,
			Type:        1,
			Status:      1,
			Stock:       30,
			BeginTime:   now,
			EndTime:     now.Add(48 * time.Hour),
		},
	}

	result := toVoucherRespList(vouchers)
	require.Len(t, result, 2)
	require.Equal(t, "券A", result[0].Title)
	require.Equal(t, "券B", result[1].Title)
	require.Equal(t, uint64(1), result[0].ID)
	require.Equal(t, uint64(2), result[1].ID)
}

func TestToVoucherRespList_Empty(t *testing.T) {
	result := toVoucherRespList(nil)
	require.Empty(t, result)

	result = toVoucherRespList([]Voucher{})
	require.Empty(t, result)
}
