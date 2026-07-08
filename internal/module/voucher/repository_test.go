//go:build integration
package voucher

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"dianping/internal/module/seckillvoucher"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setUpVoucherRepo(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()

	ctx := context.Background()

	ctr, err := tcmysql.Run(ctx,
		"mysql:8.0.36",
		tcmysql.WithDatabase("dianping_test"),
		tcmysql.WithUsername("test"),
		tcmysql.WithPassword("test"),
		tcmysql.WithScripts(filepath.Join("testdata", "schema.sql")),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, tc.TerminateContainer(ctr))
	})

	dsn, err := ctr.ConnectionString(ctx, "parseTime=true", "loc=Local", "charset=utf8mb4")
	require.NoError(t, err)
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	tx := db.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() {
		require.NoError(t, tx.Rollback().Error)
	})

	return NewRepository(tx), tx
}

func TestRepository_CreateVoucher(t *testing.T) {
	t.Run("create voucher successfully", func(t *testing.T) {
		repo, db := setUpVoucherRepo(t)
		ctx := context.Background()

		voucher := &Voucher{
			ShopID:      1,
			Title:       "Test Voucher",
			SubTitle:    "A test voucher",
			Rules:       "No rules",
			PayValue:    100,
			ActualValue: 200,
			Type:        0,
			Status:      1,
			Stock:       50,
			BeginTime:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndTime:     time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		}
		err := repo.CreateVoucher(ctx, voucher)
		require.NoError(t, err)
		require.NotZero(t, voucher.ID)

		var got Voucher
		err = db.WithContext(ctx).Where("id = ?", voucher.ID).First(&got).Error
		require.NoError(t, err)
		require.Equal(t, voucher.Title, got.Title)
		require.Equal(t, voucher.ShopID, got.ShopID)
		require.Equal(t, voucher.Type, got.Type)
	})
}

func TestRepository_GetVoucherByID(t *testing.T) {
	t.Run("get voucher by ID successfully", func(t *testing.T) {
		repo, db := setUpVoucherRepo(t)
		ctx := context.Background()

		seed := &Voucher{
			ShopID: 1, Title: "Seed Voucher", SubTitle: "S", Rules: "R",
			PayValue: 10, ActualValue: 20, Type: 0, Status: 1, Stock: 5,
			BeginTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		got, err := repo.GetVoucherByID(ctx, seed.ID)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, seed.ID, got.ID)
		require.Equal(t, seed.Title, got.Title)
		require.Equal(t, seed.ShopID, got.ShopID)
	})

	t.Run("get voucher by non-existent ID returns nil", func(t *testing.T) {
		repo, _ := setUpVoucherRepo(t)
		ctx := context.Background()

		got, err := repo.GetVoucherByID(ctx, 999999)
		require.NoError(t, err)
		require.Nil(t, got)
	})
}

func TestRepository_GetByShopID(t *testing.T) {
	t.Run("get vouchers by shop ID successfully", func(t *testing.T) {
		repo, db := setUpVoucherRepo(t)
		ctx := context.Background()

		now := time.Now()
		vouchers := []Voucher{
			{ShopID: 10, Title: "V1", SubTitle: "S1", Rules: "R1", PayValue: 10, ActualValue: 20, Stock: 5, Status: 1, BeginTime: now, EndTime: now.Add(24 * time.Hour)},
			{ShopID: 10, Title: "V2", SubTitle: "S2", Rules: "R2", PayValue: 30, ActualValue: 50, Stock: 10, Status: 1, BeginTime: now, EndTime: now.Add(24 * time.Hour)},
			{ShopID: 20, Title: "V3", SubTitle: "S3", Rules: "R3", PayValue: 1, ActualValue: 2, Stock: 1, Status: 1, BeginTime: now, EndTime: now.Add(24 * time.Hour)},
		}
		err := db.WithContext(ctx).Create(&vouchers).Error
		require.NoError(t, err)

		got, err := repo.GetByShopID(ctx, 10)
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, "V2", got[0].Title) // newest first (DESC by create_time)
		require.Equal(t, "V1", got[1].Title)
	})

	t.Run("get vouchers for shop with no vouchers returns empty slice", func(t *testing.T) {
		repo, _ := setUpVoucherRepo(t)
		ctx := context.Background()

		got, err := repo.GetByShopID(ctx, 999)
		require.NoError(t, err)
		require.Len(t, got, 0)
	})
}

func TestRepository_CreateSeckillVoucher(t *testing.T) {
	t.Run("create seckill voucher successfully in transaction", func(t *testing.T) {
		repo, db := setUpVoucherRepo(t)
		ctx := context.Background()

		v := &Voucher{
			ShopID: 1, Title: "Seckill Deal", SubTitle: "S", Rules: "R",
			PayValue: 50, ActualValue: 200, Type: 1, Status: 1, Stock: 10,
			BeginTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		}
		sv := &seckillvoucher.SeckillVoucher{
			Stock:     10,
			BeginTime: v.BeginTime,
			EndTime:   v.EndTime,
		}

		err := repo.CreateSeckillVoucher(ctx, v, sv)
		require.NoError(t, err)
		require.NotZero(t, v.ID)
		require.Equal(t, v.ID, sv.VoucherID)

		// Verify voucher was created
		var gotVoucher Voucher
		err = db.WithContext(ctx).Where("id = ?", v.ID).First(&gotVoucher).Error
		require.NoError(t, err)
		require.Equal(t, v.Title, gotVoucher.Title)
		require.Equal(t, uint(1), gotVoucher.Type)

		// Verify seckill voucher was created
		var gotSeckill seckillvoucher.SeckillVoucher
		err = db.WithContext(ctx).Where("voucher_id = ?", v.ID).First(&gotSeckill).Error
		require.NoError(t, err)
		require.Equal(t, uint(10), gotSeckill.Stock)
	})

	t.Run("duplicate seckill voucher ID returns error", func(t *testing.T) {
		repo, db := setUpVoucherRepo(t)
		ctx := context.Background()

		// First creation should succeed
		v1 := &Voucher{
			ShopID: 1, Title: "S1", SubTitle: "S", Rules: "R",
			PayValue: 10, ActualValue: 20, Type: 1, Status: 1, Stock: 5,
			BeginTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		}
		sv1 := &seckillvoucher.SeckillVoucher{
			Stock: 5, BeginTime: v1.BeginTime, EndTime: v1.EndTime,
		}
		err := repo.CreateSeckillVoucher(ctx, v1, sv1)
		require.NoError(t, err)

		// Second creation with same voucher_id linking
		// Create a voucher first, then try to create a seckill with same voucher_id
		v2 := &Voucher{
			ShopID: 1, Title: "S2", SubTitle: "S", Rules: "R",
			PayValue: 10, ActualValue: 20, Type: 1, Status: 1, Stock: 5,
			BeginTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		}
		err = db.WithContext(ctx).Create(v2).Error
		require.NoError(t, err)

		// Now try to create seckill voucher with voucher_id = v1.ID (already exists in tb_seckill_voucher)
		svDup := &seckillvoucher.SeckillVoucher{
			VoucherID: v1.ID, Stock: 5, BeginTime: v1.BeginTime, EndTime: v1.EndTime,
		}
		err = db.WithContext(ctx).Create(svDup).Error
		require.Error(t, err)
		var mysqlErr *mysql.MySQLError
		require.True(t, errors.As(err, &mysqlErr))
		require.Equal(t, uint16(1062), mysqlErr.Number) // duplicate entry
	})
}
