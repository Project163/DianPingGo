//go:build integration

package user

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setUpRepository(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()

	ctx := context.Background()

	// 启动 MySQL 容器，使用 testcontainers-go 库
	ctr, err := tcmysql.Run(ctx,
		"mysql:8.0.36",
		tcmysql.WithDatabase("dianping_test"),
		tcmysql.WithUsername("test"),
		tcmysql.WithPassword("test"),
		tcmysql.WithScripts(filepath.Join("testdata", "schema.sql")),
	)
	require.NoError(t, err)
	// 在测试结束时终止容器
	t.Cleanup(func() {
		require.NoError(t, tc.TerminateContainer(ctr))
	})

	// 连接到 MySQL 数据库，使用 GORM 库
	dsn, err := ctr.ConnectionString(ctx, "parseTime=true", "loc=Local", "charset=utf8mb4")
	require.NoError(t, err)
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	// 开启事务，确保每个测试用例的数据隔离
	tx := db.Begin()
	require.NoError(t, tx.Error)

	// 在测试结束时回滚事务，确保数据库状态不受影响
	t.Cleanup(func() {
		require.NoError(t, tx.Rollback().Error)
	})

	return NewRepository(tx), tx
}

func TestRepository_CreateUser(t *testing.T) {
	t.Run("created successfully",
		func(t *testing.T) {
			repo, db := setUpRepository(t)
			ctx := context.Background()

			user := &User{
				Phone:    "12345678901",
				Password: "hash",
				NickName: "Alice",
				Icon:     "/imgs/icon.png",
			}
			err := repo.CreateUser(ctx, user)
			require.NoError(t, err)
			require.NotZero(t, user.ID)

			// 验证用户是否已插入数据库
			var got User
			err = db.WithContext(ctx).Where("id = ?", user.ID).First(&got).Error
			require.NoError(t, err)
			require.Equal(t, user.Phone, got.Phone)
			require.Equal(t, user.Password, got.Password)
			require.Equal(t, user.NickName, got.NickName)
			require.Equal(t, user.Icon, got.Icon)
		},
	)
	t.Run("duplicate phone",
		func(t *testing.T) {
			repo, _ := setUpRepository(t)
			ctx := context.Background()

			user1 := &User{
				Phone:    "12345678901",
				Password: "hash1",
				NickName: "Alice",
				Icon:     "/imgs/icon1.png",
			}
			err := repo.CreateUser(ctx, user1)
			require.NoError(t, err)

			user2 := &User{
				Phone:    "12345678901", // 重复的手机号
				Password: "hash2",
				NickName: "Bob",
				Icon:     "/imgs/icon2.png",
			}
			err = repo.CreateUser(ctx, user2)
			require.Error(t, err)
			var mysqlErr *mysql.MySQLError
			require.True(t, errors.As(err, &mysqlErr))
			require.Equal(t, uint16(1062), mysqlErr.Number) // 1062 是 MySQL 的唯一键冲突错误代码
		},
	)

}

func TestRepository_GetUserByPhone(t *testing.T) {
	t.Run("get user by phone", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		// 先插入一个用户
		seed := &User{
			Phone:    "12345678901",
			Password: "hash",
			NickName: "Alice",
			Icon:     "/imgs/icon.png",
		}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		// 测试根据手机号查询用户
		got, err := repo.GetUserByPhone(ctx, seed.Phone)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, seed.ID, got.ID)
		require.Equal(t, seed.Phone, got.Phone)
		require.Equal(t, seed.Password, got.Password)
		require.Equal(t, seed.NickName, got.NickName)
		require.Equal(t, seed.Icon, got.Icon)
	})

	t.Run("get user by phone with no record", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		// 测试查询不存在的手机号
		got, err := repo.GetUserByPhone(ctx, "00000000000")
		require.NoError(t, err)
		require.Nil(t, got)
	})
}

func TestRepository_GetUserByID(t *testing.T) {
	t.Run("get user by ID", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		// 先插入一个用户
		seed := &User{
			Phone:    "12345678901",
			Password: "hash",
			NickName: "Alice",
			Icon:     "/imgs/icon.png",
		}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		// 测试根据ID查询用户
		got, err := repo.GetUserByID(ctx, seed.ID)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, seed.ID, got.ID)
		require.Equal(t, seed.Phone, got.Phone)
		require.Equal(t, seed.Password, got.Password)
		require.Equal(t, seed.NickName, got.NickName)
		require.Equal(t, seed.Icon, got.Icon)

	})
	t.Run("get user by ID with no record", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()
		// 测试查询不存在的ID
		got, err := repo.GetUserByID(ctx, 999999)
		require.NoError(t, err)
		require.Nil(t, got)
	})
}

func TestRepository_ListUsersByIDs(t *testing.T) {
	t.Run("list users by IDs", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		// 先插入多个用户
		users := []User{
			{Phone: "12345678901", Password: "hash1", NickName: "Alice", Icon: "/imgs/icon1.png"},
			{Phone: "12345678902", Password: "hash2", NickName: "Bob", Icon: "/imgs/icon2.png"},
			{Phone: "12345678903", Password: "hash3", NickName: "Charlie", Icon: "/imgs/icon3.png"},
		}
		err := db.WithContext(ctx).Create(&users).Error
		require.NoError(t, err)

		// 测试根据ID列表查询用户
		userIDs := []uint64{users[0].ID, users[1].ID}
		got, err := repo.ListUsersByIDs(ctx, userIDs)
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, users[0].ID, got[0].ID)
		require.Equal(t, users[1].ID, got[1].ID)
	})
	t.Run("no records", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()
		got, err := repo.ListUsersByIDs(ctx, []uint64{})
		require.NoError(t, err)
		require.Len(t, got, 0)
	})
}
