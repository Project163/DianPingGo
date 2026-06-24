package voucher

import (
	"context"
	"dianping/internal/module/seckillvoucher"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	db.AutoMigrate(&Voucher{}, &seckillvoucher.SeckillVoucher{})
	return db
}

func TestCreateVoucher(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		voucher := &Voucher{
			ShopID:      1,
			Title:       "测试优惠券",
			SubTitle:    "满100减20",
			Rules:       "满100元可用",
			PayValue:    80,
			ActualValue: 100,
			Type:        0,
			Status:      1,
			Stock:       100,
			BeginTime:   time.Now(),
			EndTime:     time.Now().Add(24 * time.Hour),
		}
		err := repo.CreateVoucher(context.Background(), voucher)
		require.NoError(t, err)
		require.NotZero(t, voucher.ID)

		var count int64
		db.Model(&Voucher{}).Count(&count)
		require.Equal(t, int64(1), count)
	})
}

func TestGetVoucherByID(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		voucher := &Voucher{
			ShopID:      1,
			Title:       "测试优惠券",
			SubTitle:    "满100减20",
			Rules:       "满100元可用",
			PayValue:    80,
			ActualValue: 100,
			Type:        0,
			Status:      1,
			Stock:       100,
			BeginTime:   time.Now(),
			EndTime:     time.Now().Add(24 * time.Hour),
		}
		db.Create(voucher)

		result, err := repo.GetVoucherByID(context.Background(), voucher.ID)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "测试优惠券", result.Title)
		require.Equal(t, uint64(1), result.ShopID)
	})

	t.Run("not found", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		result, err := repo.GetVoucherByID(context.Background(), 999)
		require.NoError(t, err)
		require.Nil(t, result)
	})

	t.Run("database error", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		voucher := &Voucher{
			ShopID:      1,
			Title:       "测试优惠券",
			SubTitle:    "满100减20",
			Rules:       "满100元可用",
			PayValue:    80,
			ActualValue: 100,
			Type:        0,
			Status:      1,
			Stock:       100,
			BeginTime:   time.Now(),
			EndTime:     time.Now().Add(24 * time.Hour),
		}
		db.Create(voucher)

		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.Close()

		result, err := repo.GetVoucherByID(context.Background(), voucher.ID)
		require.Error(t, err)
		require.Nil(t, result)
	})
}

func TestGetByShopID(t *testing.T) {
	t.Run("success with vouchers", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		now := time.Now()
		db.Create(&Voucher{
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
		})
		db.Create(&Voucher{
			ShopID:      1,
			Title:       "优惠券B",
			SubTitle:    "副标题B",
			Rules:       "规则B",
			PayValue:    40,
			ActualValue: 50,
			Type:        1,
			Status:      1,
			Stock:       30,
			BeginTime:   now,
			EndTime:     now.Add(48 * time.Hour),
		})
		db.Create(&Voucher{
			ShopID:      2,
			Title:       "其他店铺券",
			SubTitle:    "副标题C",
			Rules:       "规则C",
			PayValue:    10,
			ActualValue: 20,
			Type:        0,
			Status:      1,
			Stock:       10,
			BeginTime:   now,
			EndTime:     now.Add(24 * time.Hour),
		})

		vouchers, err := repo.GetByShopID(context.Background(), 1)
		require.NoError(t, err)
		require.Len(t, vouchers, 2)
		require.Equal(t, "优惠券B", vouchers[0].Title) // ORDER BY create_time DESC
		require.Equal(t, "优惠券A", vouchers[1].Title)
	})

	t.Run("empty list", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		vouchers, err := repo.GetByShopID(context.Background(), 999)
		require.NoError(t, err)
		require.Empty(t, vouchers)
	})

	t.Run("database error", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.Close()

		vouchers, err := repo.GetByShopID(context.Background(), 1)
		require.Error(t, err)
		require.Nil(t, vouchers)
	})
}

func TestCreateSeckillVoucher(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		now := time.Now()
		v := &Voucher{
			ShopID:      1,
			Title:       "秒杀券",
			SubTitle:    "限时秒杀",
			Rules:       "秒杀规则",
			PayValue:    50,
			ActualValue: 100,
			Type:        1,
			Status:      1,
			Stock:       10,
			BeginTime:   now,
			EndTime:     now.Add(1 * time.Hour),
		}
		sv := &seckillvoucher.SeckillVoucher{
			Stock:     10,
			BeginTime: now,
			EndTime:   now.Add(1 * time.Hour),
		}

		err := repo.CreateSeckillVoucher(context.Background(), v, sv)
		require.NoError(t, err)
		require.NotZero(t, v.ID)
		require.Equal(t, v.ID, sv.VoucherID)

		var voucherCount int64
		db.Model(&Voucher{}).Count(&voucherCount)
		require.Equal(t, int64(1), voucherCount)

		var seckillCount int64
		db.Model(&seckillvoucher.SeckillVoucher{}).Count(&seckillCount)
		require.Equal(t, int64(1), seckillCount)
	})
}
