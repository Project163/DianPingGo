package voucherorder

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = db.AutoMigrate(&VoucherOrder{})
	require.NoError(t, err)
	return db
}

func TestCreateVoucherOrder_Repo(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		order := &VoucherOrder{
			ID:        1,
			UserID:    100,
			VoucherID: 200,
		}
		err := repo.CreateVoucherOrder(context.Background(), order)
		require.NoError(t, err)

		var count int64
		db.Model(&VoucherOrder{}).Count(&count)
		require.Equal(t, int64(1), count)
	})

	t.Run("database error", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.Close()

		err = repo.CreateVoucherOrder(context.Background(), &VoucherOrder{
			ID: 1, UserID: 100, VoucherID: 200,
		})
		require.Error(t, err)
	})
}

func TestCountByUserAndVoucher(t *testing.T) {
	t.Run("no orders returns zero", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		count, err := repo.CountByUserAndVoucher(context.Background(), 100, 200)
		require.NoError(t, err)
		require.Equal(t, int64(0), count)
	})

	t.Run("counts only matching user and voucher", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		// matching order
		db.Create(&VoucherOrder{ID: 1, UserID: 100, VoucherID: 200})
		// same user, different voucher
		db.Create(&VoucherOrder{ID: 2, UserID: 100, VoucherID: 300})
		// different user, same voucher
		db.Create(&VoucherOrder{ID: 3, UserID: 999, VoucherID: 200})

		count, err := repo.CountByUserAndVoucher(context.Background(), 100, 200)
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})

	t.Run("database error", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.Close()

		_, err = repo.CountByUserAndVoucher(context.Background(), 100, 200)
		require.Error(t, err)
	})
}
