package voucher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"dianping/internal/cache"
	"dianping/internal/module/seckillvoucher"
	"dianping/pkg/errmsg"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type mockVoucherRepo struct {
	createVoucherFunc        func(ctx context.Context, voucher *Voucher) error
	getVoucherByIDFunc       func(ctx context.Context, id uint64) (*Voucher, error)
	getByShopIDFunc          func(ctx context.Context, shopID uint64) ([]Voucher, error)
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

func newMockVoucherRepo() *mockVoucherRepo {
	return &mockVoucherRepo{}
}

func setUpVoucherService(t *testing.T) (*Service, *mockVoucherRepo, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() {
		require.NoError(t, rdb.Close())
	})

	repo := newMockVoucherRepo()
	svc := NewService(repo, rdb, nil)
	return svc, repo, mr
}

// =============================================================================
// CreateVoucher
// =============================================================================

func TestService_CreateVoucher(t *testing.T) {
	t.Run("create voucher successfully", func(t *testing.T) {
		svc, repo, _ := setUpVoucherService(t)
		ctx := context.Background()

		repo.createVoucherFunc = func(ctx context.Context, voucher *Voucher) error {
			voucher.ID = 100 // 模拟数据库自增
			require.Equal(t, uint64(1), voucher.ShopID)
			require.Equal(t, "Test Voucher", voucher.Title)
			require.Equal(t, uint(0), voucher.Type)
			return nil
		}

		id, err := svc.CreateVoucher(ctx, &Voucher{
			ShopID:      1,
			Title:       "Test Voucher",
			SubTitle:    "Sub",
			Rules:       "Rule",
			PayValue:    100,
			ActualValue: 200,
			Type:        0,
			Stock:       50,
		})
		require.NoError(t, err)
		require.Equal(t, uint64(100), id)
	})

	t.Run("create voucher with repository error", func(t *testing.T) {
		svc, repo, _ := setUpVoucherService(t)
		ctx := context.Background()

		dbErr := errors.New("database connection lost")
		repo.createVoucherFunc = func(ctx context.Context, voucher *Voucher) error {
			return dbErr
		}

		id, err := svc.CreateVoucher(ctx, &Voucher{
			ShopID: 1, Title: "Test", SubTitle: "S", Rules: "R", PayValue: 10, ActualValue: 20, Stock: 5,
		})
		require.Error(t, err)
		require.Equal(t, uint64(0), id)
		require.Equal(t, dbErr, err)
	})
}

// =============================================================================
// CreateSeckillVoucher
// =============================================================================

func TestService_CreateSeckillVoucher(t *testing.T) {
	t.Run("create seckill voucher successfully and sets stock in Redis", func(t *testing.T) {
		svc, repo, mr := setUpVoucherService(t)
		ctx := context.Background()

		repo.createSeckillVoucherFunc = func(ctx context.Context, v *Voucher, sv *seckillvoucher.SeckillVoucher) error {
			v.ID = 200
			require.Equal(t, uint(1), v.Type)
			require.Equal(t, uint(30), v.Stock)
			return nil
		}

		id, err := svc.CreateSeckillVoucher(ctx, &Voucher{
			ShopID:      1,
			Title:       "Seckill",
			SubTitle:    "S",
			Rules:       "R",
			PayValue:    50,
			ActualValue: 200,
			Type:        1,
			Stock:       30,
		})
		require.NoError(t, err)
		require.Equal(t, uint64(200), id)

		// Verify Redis stock key was set
		stockKey := fmt.Sprintf("%s%d", SeckillStockKey, 200)
		stockVal, _ := mr.Get(stockKey)
		require.Equal(t, "30", stockVal)
	})

	t.Run("create seckill voucher with repository error", func(t *testing.T) {
		svc, repo, _ := setUpVoucherService(t)
		ctx := context.Background()

		repo.createSeckillVoucherFunc = func(ctx context.Context, v *Voucher, sv *seckillvoucher.SeckillVoucher) error {
			return errors.New("tx rollback")
		}

		id, err := svc.CreateSeckillVoucher(ctx, &Voucher{
			ShopID: 1, Title: "S", SubTitle: "S", Rules: "R", PayValue: 10, ActualValue: 20, Type: 1, Stock: 5,
		})
		require.Error(t, err)
		require.Equal(t, uint64(0), id)
	})
}

// =============================================================================
// GetVoucherByID
// =============================================================================

func TestService_GetVoucherByID(t *testing.T) {
	t.Run("cache miss queries DB and caches result", func(t *testing.T) {
		svc, repo, mr := setUpVoucherService(t)
		ctx := context.Background()

		repo.getVoucherByIDFunc = func(ctx context.Context, id uint64) (*Voucher, error) {
			require.Equal(t, uint64(100), id)
			return &Voucher{
				ID:          100,
				ShopID:      1,
				Title:       "Test",
				SubTitle:    "Sub",
				Rules:       "Rule",
				PayValue:    50,
				ActualValue: 100,
				Type:        0,
				Status:      1,
				Stock:       20,
				BeginTime:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				EndTime:     time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
			}, nil
		}

		resp, err := svc.GetVoucherByID(ctx, 100)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, uint64(100), resp.ID)
		require.Equal(t, "Test", resp.Title)

		// Verify result was cached
		cacheKey := fmt.Sprintf("%s%d", CacheVoucherKey, 100)
		cached, _ := mr.Get(cacheKey)
		require.NotEmpty(t, cached)
	})

	t.Run("cache hit returns from cache without querying DB", func(t *testing.T) {
		svc, _, mr := setUpVoucherService(t)
		ctx := context.Background()

		cachedVoucher := Voucher{
			ID: 200, ShopID: 2, Title: "Cached", SubTitle: "Sub",
			Rules: "R", PayValue: 10, ActualValue: 20,
			Type: 0, Status: 1, Stock: 5,
		}
		cacheKey := fmt.Sprintf("%s%d", CacheVoucherKey, 200)
		bytes, _ := json.Marshal(cachedVoucher)
		mr.Set(cacheKey, string(bytes))

		resp, err := svc.GetVoucherByID(ctx, 200)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, uint64(200), resp.ID)
		require.Equal(t, "Cached", resp.Title)
	})

	t.Run("cache miss with null marker returns nil", func(t *testing.T) {
		svc, _, mr := setUpVoucherService(t)
		ctx := context.Background()

		// Set empty string as null marker
		cacheKey := fmt.Sprintf("%s%d", CacheVoucherKey, 999)
		mr.Set(cacheKey, "")

		resp, err := svc.GetVoucherByID(ctx, 999)
		require.NoError(t, err)
		require.Nil(t, resp)
	})

	t.Run("DB returns nil record writes null marker and returns nil", func(t *testing.T) {
		svc, repo, mr := setUpVoucherService(t)
		ctx := context.Background()

		repo.getVoucherByIDFunc = func(ctx context.Context, id uint64) (*Voucher, error) {
			return nil, nil
		}

		resp, err := svc.GetVoucherByID(ctx, 999)
		require.NoError(t, err)
		require.Nil(t, resp)

		// Verify null marker was cached
		cacheKey := fmt.Sprintf("%s%d", CacheVoucherKey, 999)
		nullVal, _ := mr.Get(cacheKey)
		require.Equal(t, "", nullVal)
	})

	t.Run("repository DB error is propagated", func(t *testing.T) {
		svc, repo, _ := setUpVoucherService(t)
		ctx := context.Background()

		dbErr := errors.New("db connection lost")
		repo.getVoucherByIDFunc = func(ctx context.Context, id uint64) (*Voucher, error) {
			return nil, dbErr
		}

		resp, err := svc.GetVoucherByID(ctx, 100)
		require.Error(t, err)
		require.Nil(t, resp)
	})
}

// =============================================================================
// GetVoucherByShopID
// =============================================================================

func TestService_GetVoucherByShopID(t *testing.T) {
	t.Run("cache miss queries DB and caches result", func(t *testing.T) {
		svc, repo, mr := setUpVoucherService(t)
		ctx := context.Background()

		repo.getByShopIDFunc = func(ctx context.Context, shopID uint64) ([]Voucher, error) {
			require.Equal(t, uint64(1), shopID)
			return []Voucher{
				{ID: 1, ShopID: 1, Title: "V1", SubTitle: "S1", Rules: "R1", PayValue: 10, ActualValue: 20, Status: 1},
				{ID: 2, ShopID: 1, Title: "V2", SubTitle: "S2", Rules: "R2", PayValue: 30, ActualValue: 50, Status: 1},
			}, nil
		}

		resps, err := svc.GetVoucherByShopID(ctx, 1)
		require.NoError(t, err)
		require.Len(t, resps, 2)
		require.Equal(t, uint64(1), resps[0].ID)
		require.Equal(t, "V1", resps[0].Title)
		require.Equal(t, uint64(2), resps[1].ID)
		require.Equal(t, "V2", resps[1].Title)

		// Verify result was cached
		cacheKey := fmt.Sprintf("%s%d", CacheShopVoucherKey, 1)
		cached, _ := mr.Get(cacheKey)
		require.NotEmpty(t, cached)
	})

	t.Run("cache hit returns from cache", func(t *testing.T) {
		svc, _, mr := setUpVoucherService(t)
		ctx := context.Background()

		cachedVouchers := []Voucher{
			{ID: 10, ShopID: 5, Title: "CachedV", SubTitle: "S", Rules: "R", PayValue: 1, ActualValue: 2, Status: 1},
		}
		cacheKey := fmt.Sprintf("%s%d", CacheShopVoucherKey, 5)
		bytes, _ := json.Marshal(cachedVouchers)
		mr.Set(cacheKey, string(bytes))

		resps, err := svc.GetVoucherByShopID(ctx, 5)
		require.NoError(t, err)
		require.Len(t, resps, 1)
		require.Equal(t, "CachedV", resps[0].Title)
	})

	t.Run("no vouchers found returns empty slice", func(t *testing.T) {
		svc, repo, _ := setUpVoucherService(t)
		ctx := context.Background()

		repo.getByShopIDFunc = func(ctx context.Context, shopID uint64) ([]Voucher, error) {
			return []Voucher{}, nil
		}

		resps, err := svc.GetVoucherByShopID(ctx, 999)
		require.NoError(t, err)
		require.Len(t, resps, 0)
	})

	t.Run("repository DB error is propagated via cache layer", func(t *testing.T) {
		svc, repo, _ := setUpVoucherService(t)
		ctx := context.Background()

		dbErr := errors.New("db connection lost")
		repo.getByShopIDFunc = func(ctx context.Context, shopID uint64) ([]Voucher, error) {
			return nil, dbErr
		}

		// The cache QueryWithPassThrough returns the db error directly
		// when the dbFunc returns an error (not ErrDataNotFound)
		resps, err := svc.GetVoucherByShopID(ctx, 1)
		require.Error(t, err)
		require.Nil(t, resps)
	})
}

// =============================================================================
// toVoucherRespList
// =============================================================================

func TestService_toVoucherRespList(t *testing.T) {
	t.Run("converts vouchers to response DTOs", func(t *testing.T) {
		vouchers := []Voucher{
			{
				ID: 1, ShopID: 10, Title: "V1", SubTitle: "S1", Rules: "R1",
				PayValue: 100, ActualValue: 200, Type: 0, Status: 1, Stock: 50,
			},
			{
				ID: 2, ShopID: 10, Title: "V2", SubTitle: "S2", Rules: "R2",
				PayValue: 300, ActualValue: 500, Type: 1, Status: 1, Stock: 10,
			},
		}

		resps := toVoucherRespList(vouchers)
		require.Len(t, resps, 2)
		require.Equal(t, uint64(1), resps[0].ID)
		require.Equal(t, "V1", resps[0].Title)
		require.Equal(t, uint64(200), resps[0].ActualValue)
		require.Equal(t, uint64(2), resps[1].ID)
		require.Equal(t, "V2", resps[1].Title)
		require.Equal(t, uint(1), resps[1].Type)
	})

	t.Run("empty slice returns empty slice", func(t *testing.T) {
		resps := toVoucherRespList([]Voucher{})
		require.Len(t, resps, 0)
	})
}

// Ensure the mock satisfies the VoucherRepository interface
var _ VoucherRepository = (*mockVoucherRepo)(nil)

// Ensure cache.ErrDataNotFound is importable
var _ = cache.ErrDataNotFound

// Ensure errmsg is importable
var _ = errmsg.ErrInternalSec
