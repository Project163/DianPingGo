//go:build integration

package userinfo

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

func setUpRepository(t *testing.T) (*Repository, *gorm.DB) {
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

// =============================================================================
// CreateUserInfo
// =============================================================================

func TestRepository_CreateUserInfo(t *testing.T) {
	t.Run("creates user info successfully", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		birthday := time.Date(1995, 6, 15, 0, 0, 0, 0, time.UTC)
		dto := &UserInfoDTO{
			UserID:    1001,
			City:      "Shanghai",
			Introduce: "Hello world",
			Fans:      50,
			Followee:  30,
			Gender:    1,
			Birthday:  birthday,
			Credits:   150,
			Level:     5,
		}

		err := repo.CreateUserInfo(ctx, dto)
		require.NoError(t, err)

		var got UserInfo
		err = db.WithContext(ctx).Where("user_id = ?", 1001).First(&got).Error
		require.NoError(t, err)
		require.Equal(t, uint64(1001), got.UserID)
		require.Equal(t, "Shanghai", got.City)
		require.Equal(t, "Hello world", got.Introduce)
		require.Equal(t, uint32(50), got.Fans)
		require.Equal(t, uint32(30), got.Followee)
		require.Equal(t, uint8(1), got.Gender)
		require.Equal(t, uint32(150), got.Credits)
		require.Equal(t, uint8(5), got.Level)
	})

	t.Run("create with partial fields fills defaults", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		dto := &UserInfoDTO{
			UserID: 2001,
			City:   "Beijing",
		}

		err := repo.CreateUserInfo(ctx, dto)
		require.NoError(t, err)

		var got UserInfo
		err = db.WithContext(ctx).Where("user_id = ?", 2001).First(&got).Error
		require.NoError(t, err)
		require.Equal(t, uint64(2001), got.UserID)
		require.Equal(t, "Beijing", got.City)
		require.Equal(t, "", got.Introduce)
		require.Equal(t, uint32(0), got.Fans)
		require.Equal(t, uint32(0), got.Followee)
	})

	t.Run("duplicate primary key returns error", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		dto1 := &UserInfoDTO{
			UserID: 3001,
			City:   "First",
		}
		err := repo.CreateUserInfo(ctx, dto1)
		require.NoError(t, err)

		dto2 := &UserInfoDTO{
			UserID: 3001,
			City:   "Second",
		}
		err = repo.CreateUserInfo(ctx, dto2)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Duplicate")
	})
}

// =============================================================================
// GetUserInfoByUserID
// =============================================================================

func TestRepository_GetUserInfoByUserID(t *testing.T) {
	t.Run("returns user info when record exists", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		birthday := time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)
		seed := &UserInfo{
			UserID:    1001,
			City:      "Shenzhen",
			Introduce: "Developer",
			Fans:      100,
			Followee:  50,
			Gender:    1,
			Birthday:  birthday,
			Credits:   300,
			Level:     3,
		}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		got, err := repo.GetUserInfoByUserID(ctx, 1001)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, seed.UserID, got.UserID)
		require.Equal(t, seed.City, got.City)
		require.Equal(t, seed.Introduce, got.Introduce)
		require.Equal(t, seed.Fans, got.Fans)
		require.Equal(t, seed.Followee, got.Followee)
		require.Equal(t, seed.Gender, got.Gender)
		require.Equal(t, seed.Credits, got.Credits)
		require.Equal(t, seed.Level, got.Level)
	})

	t.Run("returns nil when no record found", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		got, err := repo.GetUserInfoByUserID(ctx, 9999)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("handles zero user ID", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		got, err := repo.GetUserInfoByUserID(ctx, 0)
		require.NoError(t, err)
		require.Nil(t, got)
	})
}
