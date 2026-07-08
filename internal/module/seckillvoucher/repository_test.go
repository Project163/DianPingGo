//go:build integration
package seckillvoucher

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setUpSeckillVoucherRepo(t *testing.T) (*Repository, *gorm.DB) {
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

func TestRepository_CreateSeckillVoucher(t *testing.T) {
	t.Run("create seckill voucher successfully", func(t *testing.T) {
		repo, db := setUpSeckillVoucherRepo(t)
		ctx := context.Background()

		sv := &SeckillVoucher{
			VoucherID: 100,
			Stock:     50,
			BeginTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC),
		}
		err := repo.CreateSeckillVoucher(ctx, sv)
		require.NoError(t, err)

		// Verify seckill voucher was inserted
		var got SeckillVoucher
		err = db.WithContext(ctx).Where("voucher_id = ?", 100).First(&got).Error
		require.NoError(t, err)
		require.Equal(t, uint64(100), got.VoucherID)
		require.Equal(t, uint(50), got.Stock)
	})

	t.Run("duplicate voucher_id returns error", func(t *testing.T) {
		repo, _ := setUpSeckillVoucherRepo(t)
		ctx := context.Background()

		sv1 := &SeckillVoucher{
			VoucherID: 200,
			Stock:     10,
			BeginTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		}
		err := repo.CreateSeckillVoucher(ctx, sv1)
		require.NoError(t, err)

		sv2 := &SeckillVoucher{
			VoucherID: 200,
			Stock:     5,
			BeginTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		}
		err = repo.CreateSeckillVoucher(ctx, sv2)
		require.Error(t, err)
	})
}

func TestRepository_GetSeckillVoucherByID(t *testing.T) {
	t.Run("get seckill voucher by ID successfully", func(t *testing.T) {
		repo, db := setUpSeckillVoucherRepo(t)
		ctx := context.Background()

		seed := &SeckillVoucher{
			VoucherID: 300,
			Stock:     25,
			BeginTime: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		got, err := repo.GetSeckillVoucherByID(ctx, 300)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, uint64(300), got.VoucherID)
		require.Equal(t, uint(25), got.Stock)
	})

	t.Run("get seckill voucher by non-existent ID returns nil", func(t *testing.T) {
		repo, _ := setUpSeckillVoucherRepo(t)
		ctx := context.Background()

		got, err := repo.GetSeckillVoucherByID(ctx, 999999)
		require.NoError(t, err)
		require.Nil(t, got)
	})
}

func TestRepository_DeductStock(t *testing.T) {
	t.Run("deduct stock successfully", func(t *testing.T) {
		repo, db := setUpSeckillVoucherRepo(t)
		ctx := context.Background()

		seed := &SeckillVoucher{
			VoucherID: 400,
			Stock:     10,
			BeginTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		ok, err := repo.DeductStock(ctx, 400)
		require.NoError(t, err)
		require.True(t, ok)

		// Verify stock is decremented
		var got SeckillVoucher
		err = db.WithContext(ctx).Where("voucher_id = ?", 400).First(&got).Error
		require.NoError(t, err)
		require.Equal(t, uint(9), got.Stock)
	})

	t.Run("deduct stock multiple times until zero", func(t *testing.T) {
		repo, db := setUpSeckillVoucherRepo(t)
		ctx := context.Background()

		seed := &SeckillVoucher{
			VoucherID: 500,
			Stock:     3,
			BeginTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		// Deduct 3 times — all should succeed
		for i := 0; i < 3; i++ {
			ok, err := repo.DeductStock(ctx, 500)
			require.NoError(t, err)
			require.True(t, ok)
		}

		// Verify stock is 0
		var got SeckillVoucher
		err = db.WithContext(ctx).Where("voucher_id = ?", 500).First(&got).Error
		require.NoError(t, err)
		require.Equal(t, uint(0), got.Stock)

		// 4th deduction should fail (stock is 0)
		ok, err := repo.DeductStock(ctx, 500)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("deduct stock for non-existent voucher returns false", func(t *testing.T) {
		repo, _ := setUpSeckillVoucherRepo(t)
		ctx := context.Background()

		ok, err := repo.DeductStock(ctx, 999999)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("deduct stock with zero initial stock returns false", func(t *testing.T) {
		repo, db := setUpSeckillVoucherRepo(t)
		ctx := context.Background()

		seed := &SeckillVoucher{
			VoucherID: 600,
			Stock:     0,
			BeginTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		ok, err := repo.DeductStock(ctx, 600)
		require.NoError(t, err)
		require.False(t, ok)
	})
}
