package shop

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = db.AutoMigrate(&Shop{})
	require.NoError(t, err)
	return db
}

func TestGetShopByID(t *testing.T) {
	t.Run("shop exists", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		shop := &Shop{
			Name:    "Test Shop",
			TypeID:  1,
			Area:    "Test Area",
			Address: "123 Test Street",
		}
		db.Create(shop)

		got, err := repo.GetShopByID(context.Background(), shop.ID)

		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, shop.Name, got.Name)
		require.Equal(t, shop.TypeID, got.TypeID)
		require.Equal(t, shop.Area, got.Area)
		require.Equal(t, shop.Address, got.Address)
	})

	t.Run("shop does not exist", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		got, err := repo.GetShopByID(context.Background(), 999)

		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("database error", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		// Close the database to simulate an error
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.Close()

		got, err := repo.GetShopByID(context.Background(), 1)

		require.Error(t, err)
		require.Nil(t, got)
	})
}

func TestUpdateShop(t *testing.T) {
	t.Run("update existing shop", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		shop := &Shop{
			Name:    "Test Shop",
			TypeID:  1,
			Area:    "Test Area",
			Address: "123 Test Street",
		}
		db.Create(shop)

		shop.Name = "Updated Shop"
		shop.Area = "Updated Area"
		err := repo.UpdateShop(context.Background(), shop)
		require.NoError(t, err)

		var updatedShop Shop
		db.First(&updatedShop, shop.ID)
		require.Equal(t, "Updated Shop", updatedShop.Name)
		require.Equal(t, "Updated Area", updatedShop.Area)
		require.Equal(t, "123 Test Street", updatedShop.Address)
	})

	t.Run("database error", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		shop := &Shop{
			Name:    "Test Shop",
			TypeID:  1,
			Area:    "Test Area",
			Address: "123 Test Street",
		}
		db.Create(shop)

		// Close the database to simulate an error
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.Close()

		err = repo.UpdateShop(context.Background(), shop)
		require.Error(t, err)
	})
}

func TestGetShopsByType(t *testing.T) {
	t.Run("pagination", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		for i := range 6 {
			db.Create(&Shop{
				Name:    fmt.Sprintf("Shop%d", i),
				TypeID:  1,
				Area:    fmt.Sprintf("Area%d", i),
				Address: fmt.Sprintf("Address%d", i),
			})
		}

		page1, err := repo.GetShopsByType(context.Background(), 1, 0, 5)
		require.NoError(t, err)
		require.Len(t, page1, 5)

		page2, err := repo.GetShopsByType(context.Background(), 1, 5, 5)
		require.NoError(t, err)
		require.Len(t, page2, 1)
	})

	t.Run("empty result", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		shops, err := repo.GetShopsByType(context.Background(), 999, 0, 5)
		require.NoError(t, err)
		require.Empty(t, shops)
	})

	t.Run("filter by type", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		db.Create(&Shop{
			Name:    "Shop1",
			TypeID:  1,
			Area:    "Area1",
			Address: "Address1",
		})
		db.Create(&Shop{
			Name:    "Shop2",
			TypeID:  2,
			Area:    "Area2",
			Address: "Address2",
		})

		type1Shops, err := repo.GetShopsByType(context.Background(), 1, 0, 5)
		require.NoError(t, err)
		require.Len(t, type1Shops, 1)
		require.Equal(t, "Shop1", type1Shops[0].Name)

		type2Shops, err := repo.GetShopsByType(context.Background(), 2, 0, 5)
		require.NoError(t, err)
		require.Len(t, type2Shops, 1)
		require.Equal(t, "Shop2", type2Shops[0].Name)
	})
}

func TestGetShopsByIDs(t *testing.T) {
	t.Run("get shops by IDs", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		shop1 := &Shop{
			Name:    "Shop1",
			TypeID:  1,
			Area:    "Area1",
			Address: "Address1",
		}
		shop2 := &Shop{
			Name:    "Shop2",
			TypeID:  2,
			Area:    "Area2",
			Address: "Address2",
		}
		db.Create(shop1)
		db.Create(shop2)

		got, err := repo.GetShopsByIDs(context.Background(), []uint64{shop1.ID, shop2.ID})
		require.NoError(t, err)
		require.Len(t, got, 2)
	})
}
