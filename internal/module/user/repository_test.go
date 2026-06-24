package user

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	db.AutoMigrate(&User{})
	return db
}

func TestCreateUser(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		user := &User{
			Phone:    "1234567890",
			Password: "hashedpassword",
			NickName: "TestUser1",
			Icon:     "http://example.com/icon.png",
		}
		err := repo.CreateUser(context.Background(), user)
		require.NoError(t, err)

		var count int64
		db.Model(&User{}).Count(&count)
		require.Equal(t, int64(1), count)
	})

	t.Run("duplicate phone returns error", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		user1 := &User{
			Phone:    "1234567890",
			Password: "hashedpassword",
			NickName: "TestUser1",
			Icon:     "http://example.com/icon.png",
		}

		err := repo.CreateUser(context.Background(), user1)
		require.NoError(t, err)

		user2 := &User{
			Phone:    "1234567890", // same phone as user1
			Password: "anotherhashedpassword",
			NickName: "TestUser2",
			Icon:     "http://example.com/icon2.png",
		}

		err = repo.CreateUser(context.Background(), user2)
		require.Error(t, err)

		if !errors.Is(err, gorm.ErrDuplicatedKey) &&
			!strings.Contains(err.Error(), "UNIQUE constraint failed") {
			t.Fatalf("expected duplicate key error, got: %v", err)
		}
	})
}

func TestGetUserByPhone(t *testing.T) {
	t.Run("user exist", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		db.Create(&User{
			Phone:    "1234567890",
			NickName: "TestUser",
			Icon:     "http://example.com/icon2.png",
		})

		u, err := repo.GetUserByPhone(context.Background(), "1234567890")
		require.NoError(t, err)
		require.NotNil(t, u)
		require.Equal(t, "TestUser", u.NickName)
		require.Equal(t, "1234567890", u.Phone)
	})

	t.Run("user does not exist", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		u, err := repo.GetUserByPhone(context.Background(), "10000000000")
		require.NoError(t, err)
		require.Nil(t, u)
	})

	t.Run("database error", func(t *testing.T) {
		db := setupTestDB(t)
		repo := NewRepository(db)

		user := &User{
			Phone:    "1234567890",
			NickName: "TestUser",
			Icon:     "http://example.com/icon.png",
		}
		db.Create(user)

		// Close the database to simulate an error
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.Close()

		u, err := repo.GetUserByPhone(context.Background(), "1234567890")
		require.Error(t, err)
		require.Nil(t, u)
	})
}
