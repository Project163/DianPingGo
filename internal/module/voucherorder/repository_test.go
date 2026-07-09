//go:build integration

package voucherorder

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setUpVoucherOrderRepo(t *testing.T) (*Repository, *gorm.DB) {
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

func TestRepository_CreateVoucherOrder(t *testing.T) {
	t.Run("create order successfully", func(t *testing.T) {
		repo, db := setUpVoucherOrderRepo(t)
		ctx := context.Background()

		order := &VoucherOrder{
			ID:        12345,
			UserID:    1,
			VoucherID: 100,
			PayType:   1,
			Status:    0,
		}
		err := repo.CreateVoucherOrder(ctx, order)
		require.NoError(t, err)

		// Verify order was inserted
		var got VoucherOrder
		err = db.WithContext(ctx).Where("id = ?", 12345).First(&got).Error
		require.NoError(t, err)
		require.Equal(t, order.ID, got.ID)
		require.Equal(t, order.UserID, got.UserID)
		require.Equal(t, order.VoucherID, got.VoucherID)
		require.Equal(t, order.PayType, got.PayType)
		require.Equal(t, order.Status, got.Status)
	})

	t.Run("duplicate order ID returns error", func(t *testing.T) {
		repo, _ := setUpVoucherOrderRepo(t)
		ctx := context.Background()

		order1 := &VoucherOrder{ID: 10001, UserID: 1, VoucherID: 100, PayType: 1, Status: 0}
		err := repo.CreateVoucherOrder(ctx, order1)
		require.NoError(t, err)

		// Duplicate primary key
		order2 := &VoucherOrder{ID: 10001, UserID: 2, VoucherID: 200, PayType: 1, Status: 0}
		err = repo.CreateVoucherOrder(ctx, order2)
		require.Error(t, err)
	})
}

func TestRepository_CountByUserAndVoucher(t *testing.T) {
	t.Run("count returns correct number", func(t *testing.T) {
		repo, db := setUpVoucherOrderRepo(t)
		ctx := context.Background()

		// Insert 3 orders for same user+voucher combo
		orders := []VoucherOrder{
			{ID: 1, UserID: 10, VoucherID: 50, PayType: 1, Status: 0},
			{ID: 2, UserID: 10, VoucherID: 50, PayType: 1, Status: 0},
			{ID: 3, UserID: 10, VoucherID: 50, PayType: 1, Status: 0},
			{ID: 4, UserID: 10, VoucherID: 99, PayType: 1, Status: 0},
			{ID: 5, UserID: 20, VoucherID: 50, PayType: 1, Status: 0},
		}
		err := db.WithContext(ctx).Create(&orders).Error
		require.NoError(t, err)

		// User 10, Voucher 50 -> 3 orders
		count, err := repo.CountByUserAndVoucher(ctx, 10, 50)
		require.NoError(t, err)
		require.Equal(t, int64(3), count)

		// User 10, Voucher 99 -> 1 order
		count, err = repo.CountByUserAndVoucher(ctx, 10, 99)
		require.NoError(t, err)
		require.Equal(t, int64(1), count)

		// User 20, Voucher 50 -> 1 order
		count, err = repo.CountByUserAndVoucher(ctx, 20, 50)
		require.NoError(t, err)
		require.Equal(t, int64(1), count)

		// User 999, Voucher 50 -> 0 orders
		count, err = repo.CountByUserAndVoucher(ctx, 999, 50)
		require.NoError(t, err)
		require.Equal(t, int64(0), count)
	})

	t.Run("count with no matching records returns zero", func(t *testing.T) {
		repo, _ := setUpVoucherOrderRepo(t)
		ctx := context.Background()

		count, err := repo.CountByUserAndVoucher(ctx, 999, 999)
		require.NoError(t, err)
		require.Equal(t, int64(0), count)
	})
}

func TestRepository_GetVoucherOrderByID(t *testing.T) {
	t.Run("get order by ID successfully", func(t *testing.T) {
		repo, db := setUpVoucherOrderRepo(t)
		ctx := context.Background()

		seed := &VoucherOrder{ID: 54321, UserID: 5, VoucherID: 200, PayType: 1, Status: 0}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		got, err := repo.GetVoucherOrderByID(ctx, 54321)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, seed.ID, got.ID)
		require.Equal(t, seed.UserID, got.UserID)
		require.Equal(t, seed.VoucherID, got.VoucherID)
		require.Equal(t, seed.PayType, got.PayType)
	})

	t.Run("get order by non-existent ID returns nil", func(t *testing.T) {
		repo, _ := setUpVoucherOrderRepo(t)
		ctx := context.Background()

		got, err := repo.GetVoucherOrderByID(ctx, 999999)
		require.NoError(t, err)
		require.Nil(t, got)
	})
}
