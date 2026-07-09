//go:build integration

package shoptype

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

func TestRepository_CreateShopType(t *testing.T) {
	t.Run("create shop type successfully", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		shopType := &ShopType{
			Name: "美食",
			Icon: "/types/ms.png",
			Sort: 1,
		}
		err := repo.CreateShopType(ctx, shopType)
		require.NoError(t, err)
		require.NotZero(t, shopType.ID)

		// Verify inserted in DB
		var got ShopType
		err = db.WithContext(ctx).Where("id = ?", shopType.ID).First(&got).Error
		require.NoError(t, err)
		require.Equal(t, "美食", got.Name)
		require.Equal(t, "/types/ms.png", got.Icon)
		require.Equal(t, uint(1), got.Sort)
	})
}

func TestRepository_UpdateShopType(t *testing.T) {
	t.Run("update shop type successfully", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		seed := &ShopType{Name: "Original", Icon: "/types/old.png", Sort: 1}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		seed.Name = "Updated"
		seed.Icon = "/types/new.png"
		seed.Sort = 2

		err = repo.UpdateShopType(ctx, seed)
		require.NoError(t, err)

		// Verify update in DB
		var got ShopType
		err = db.WithContext(ctx).Where("id = ?", seed.ID).First(&got).Error
		require.NoError(t, err)
		require.Equal(t, "Updated", got.Name)
		require.Equal(t, "/types/new.png", got.Icon)
		require.Equal(t, uint(2), got.Sort)
	})
}

func TestRepository_GetShopTypeByID(t *testing.T) {
	t.Run("get shop type by ID successfully", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		seed := &ShopType{Name: "美食", Icon: "/types/ms.png", Sort: 1}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		got, err := repo.GetShopTypeByID(ctx, seed.ID)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, seed.ID, got.ID)
		require.Equal(t, "美食", got.Name)
		require.Equal(t, "/types/ms.png", got.Icon)
		require.Equal(t, uint(1), got.Sort)
	})

	t.Run("get shop type by non-existent ID returns nil", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		got, err := repo.GetShopTypeByID(ctx, 999999)
		require.NoError(t, err)
		require.Nil(t, got)
	})
}

func TestRepository_GetShopTypeAll(t *testing.T) {
	t.Run("get all shop types successfully", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		types := []ShopType{
			{Name: "美食", Icon: "/types/ms.png", Sort: 1},
			{Name: "KTV", Icon: "/types/KTV.png", Sort: 2},
			{Name: "健身运动", Icon: "/types/jsyd.png", Sort: 10},
		}
		err := db.WithContext(ctx).Create(&types).Error
		require.NoError(t, err)

		got, err := repo.GetShopTypeAll(ctx)
		require.NoError(t, err)
		require.Len(t, got, 3)
	})

	t.Run("get all shop types empty table returns empty slice", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		got, err := repo.GetShopTypeAll(ctx)
		require.NoError(t, err)
		require.Empty(t, got)
	})
}
