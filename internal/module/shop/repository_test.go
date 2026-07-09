//go:build integration

package shop

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

func TestRepository_CreateShop(t *testing.T) {
	t.Run("create shop successfully", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		shop := &Shop{
			Name:      "Test Shop",
			TypeID:    1,
			Images:    "img.jpg",
			Area:      "Test Area",
			Address:   "Test Address",
			Longitude: 120.15,
			Latitude:  30.32,
			AvgPrice:  80,
			Sold:      100,
			Comments:  50,
			Score:     47,
			OpenTime:  "10:00-22:00",
		}
		err := repo.CreateShop(ctx, shop)
		require.NoError(t, err)
		require.NotZero(t, shop.ID)

		// Verify inserted in DB
		var got Shop
		err = db.WithContext(ctx).Where("id = ?", shop.ID).First(&got).Error
		require.NoError(t, err)
		require.Equal(t, "Test Shop", got.Name)
		require.Equal(t, uint64(1), got.TypeID)
		require.Equal(t, "img.jpg", got.Images)
		require.Equal(t, "Test Area", got.Area)
		require.Equal(t, "Test Address", got.Address)
		require.Equal(t, 120.15, got.Longitude)
		require.Equal(t, 30.32, got.Latitude)
		require.Equal(t, uint64(80), got.AvgPrice)
		require.Equal(t, "10:00-22:00", got.OpenTime)
	})
}

func TestRepository_GetShopByID(t *testing.T) {
	t.Run("get shop by ID successfully", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		seed := &Shop{
			Name: "Seed Shop", TypeID: 1, Images: "img.jpg",
			Area: "Area", Address: "Address",
			Longitude: 120.15, Latitude: 30.32,
			OpenTime: "10:00-22:00",
		}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		got, err := repo.GetShopByID(ctx, seed.ID)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, seed.ID, got.ID)
		require.Equal(t, "Seed Shop", got.Name)
		require.Equal(t, uint64(1), got.TypeID)
	})

	t.Run("get shop by non-existent ID returns nil", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		got, err := repo.GetShopByID(ctx, 999999)
		require.NoError(t, err)
		require.Nil(t, got)
	})
}

func TestRepository_UpdateShop(t *testing.T) {
	t.Run("update shop successfully", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		seed := &Shop{
			Name: "Original Name", TypeID: 1, Images: "old.jpg",
			Area: "Old Area", Address: "Old Address",
			Longitude: 120.15, Latitude: 30.32,
			OpenTime: "10:00-22:00",
		}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		seed.Name = "Updated Name"
		seed.TypeID = 2
		seed.Images = "new.jpg"
		seed.Area = "New Area"
		seed.Address = "New Address"
		seed.OpenTime = "09:00-21:00"

		err = repo.UpdateShop(ctx, seed)
		require.NoError(t, err)

		// Verify update in DB
		var got Shop
		err = db.WithContext(ctx).Where("id = ?", seed.ID).First(&got).Error
		require.NoError(t, err)
		require.Equal(t, "Updated Name", got.Name)
		require.Equal(t, uint64(2), got.TypeID)
		require.Equal(t, "new.jpg", got.Images)
		require.Equal(t, "New Area", got.Area)
		require.Equal(t, "New Address", got.Address)
		require.Equal(t, "09:00-21:00", got.OpenTime)
	})
}

func TestRepository_GetShopsByType(t *testing.T) {
	t.Run("get shops by type successfully", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		// Seed both type 1 and type 2 shops
		shops := []Shop{
			{Name: "Type1 Shop A", TypeID: 1, Images: "a.jpg", Area: "Area", Address: "Addr", Longitude: 120.15, Latitude: 30.32, OpenTime: "10:00"},
			{Name: "Type1 Shop B", TypeID: 1, Images: "b.jpg", Area: "Area", Address: "Addr", Longitude: 120.16, Latitude: 30.33, OpenTime: "11:00"},
			{Name: "Type2 Shop C", TypeID: 2, Images: "c.jpg", Area: "Area", Address: "Addr", Longitude: 120.17, Latitude: 30.34, OpenTime: "12:00"},
		}
		err := db.WithContext(ctx).Create(&shops).Error
		require.NoError(t, err)

		// Query only type 1 shops
		got, err := repo.GetShopsByType(ctx, 1, 0, 10)
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, "Type1 Shop A", got[0].Name)
		require.Equal(t, "Type1 Shop B", got[1].Name)
	})

	t.Run("get shops by type with pagination", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		shops := []Shop{
			{Name: "Shop 1", TypeID: 1, Images: "img.jpg", Area: "Area", Address: "Addr", Longitude: 120.15, Latitude: 30.32, OpenTime: "10:00"},
			{Name: "Shop 2", TypeID: 1, Images: "img.jpg", Area: "Area", Address: "Addr", Longitude: 120.15, Latitude: 30.32, OpenTime: "10:00"},
			{Name: "Shop 3", TypeID: 1, Images: "img.jpg", Area: "Area", Address: "Addr", Longitude: 120.15, Latitude: 30.32, OpenTime: "10:00"},
		}
		err := db.WithContext(ctx).Create(&shops).Error
		require.NoError(t, err)

		// Page 1: offset 0, limit 2
		got, err := repo.GetShopsByType(ctx, 1, 0, 2)
		require.NoError(t, err)
		require.Len(t, got, 2)

		// Page 2: offset 2, limit 2
		got, err = repo.GetShopsByType(ctx, 1, 2, 2)
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("get shops by type with no results returns empty slice", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		got, err := repo.GetShopsByType(ctx, 999, 0, 10)
		require.NoError(t, err)
		require.Empty(t, got)
	})
}

func TestRepository_GetShopsByIDs(t *testing.T) {
	t.Run("get shops by IDs successfully", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		shops := []Shop{
			{Name: "Shop A", TypeID: 1, Images: "a.jpg", Area: "Area", Address: "Addr", Longitude: 120.15, Latitude: 30.32, OpenTime: "10:00"},
			{Name: "Shop B", TypeID: 1, Images: "b.jpg", Area: "Area", Address: "Addr", Longitude: 120.16, Latitude: 30.33, OpenTime: "11:00"},
			{Name: "Shop C", TypeID: 2, Images: "c.jpg", Area: "Area", Address: "Addr", Longitude: 120.17, Latitude: 30.34, OpenTime: "12:00"},
		}
		err := db.WithContext(ctx).Create(&shops).Error
		require.NoError(t, err)

		got, err := repo.GetShopsByIDs(ctx, []uint64{shops[0].ID, shops[2].ID})
		require.NoError(t, err)
		require.Len(t, got, 2)
		// Verify IDs match requested set
		ids := make(map[uint64]bool)
		for _, s := range got {
			ids[s.ID] = true
		}
		require.True(t, ids[shops[0].ID])
		require.True(t, ids[shops[2].ID])
	})

	t.Run("get shops by empty IDs returns empty slice", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		got, err := repo.GetShopsByIDs(ctx, []uint64{})
		require.NoError(t, err)
		require.Empty(t, got)
	})
}

func TestRepository_GetShopsByName(t *testing.T) {
	t.Run("get shops by name with fuzzy match", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		shops := []Shop{
			{Name: "茶餐厅", TypeID: 1, Images: "a.jpg", Area: "Area", Address: "Addr", Longitude: 120.15, Latitude: 30.32, OpenTime: "10:00"},
			{Name: "茶馆", TypeID: 1, Images: "b.jpg", Area: "Area", Address: "Addr", Longitude: 120.16, Latitude: 30.33, OpenTime: "11:00"},
			{Name: "咖啡店", TypeID: 2, Images: "c.jpg", Area: "Area", Address: "Addr", Longitude: 120.17, Latitude: 30.34, OpenTime: "12:00"},
		}
		err := db.WithContext(ctx).Create(&shops).Error
		require.NoError(t, err)

		// Search with "茶" should match first two
		got, err := repo.GetShopsByName(ctx, "茶", 0, 10)
		require.NoError(t, err)
		require.Len(t, got, 2)
	})

	t.Run("get shops by name with no match returns empty", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		got, err := repo.GetShopsByName(ctx, "nonexistent", 0, 10)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("get shops by name with pagination", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		for i := 0; i < 3; i++ {
			s := Shop{
				Name: "茶", TypeID: 1, Images: "img.jpg", Area: "Area", Address: "Addr",
				Longitude: 120.15, Latitude: 30.32, OpenTime: "10:00",
			}
			err := db.WithContext(ctx).Create(&s).Error
			require.NoError(t, err)
		}

		got, err := repo.GetShopsByName(ctx, "茶", 0, 2)
		require.NoError(t, err)
		require.Len(t, got, 2)
	})
}
