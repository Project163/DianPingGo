package user

import (
	"context"
	"dianping/pkg/errmsg"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type mockUserRepo struct {
	getUserByPhoneFunc func(ctx context.Context, phone string) (*User, error)
	createUserFunc     func(ctx context.Context, user *User) error
	getUserByIDFunc    func(ctx context.Context, userID uint64) (*User, error)
	listUsersByIDsFunc func(ctx context.Context, userIDs []uint64) ([]User, error)
}

func (m *mockUserRepo) GetUserByPhone(ctx context.Context, phone string) (*User, error) {
	if m.getUserByPhoneFunc != nil {
		return m.getUserByPhoneFunc(ctx, phone)
	}
	return nil, nil
}

func (m *mockUserRepo) CreateUser(ctx context.Context, user *User) error {
	if m.createUserFunc != nil {
		return m.createUserFunc(ctx, user)
	}
	return nil
}

func (m *mockUserRepo) GetUserByID(ctx context.Context, userID uint64) (*User, error) {
	if m.getUserByIDFunc != nil {
		return m.getUserByIDFunc(ctx, userID)
	}
	return nil, nil
}

func (m *mockUserRepo) ListUsersByIDs(ctx context.Context, userIDs []uint64) ([]User, error) {
	if m.listUsersByIDsFunc != nil {
		return m.listUsersByIDsFunc(ctx, userIDs)
	}
	return nil, nil
}

func setupService(t *testing.T) (*Service, *mockUserRepo, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	repo := new(mockUserRepo)
	svc := NewService(repo, rdb)
	return svc, repo, mr
}

func TestLogin(t *testing.T) {
	hashed, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	require.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		srv, repo, mr := setupService(t)
		repo.getUserByPhoneFunc = func(ctx context.Context, phone string) (*User, error) {
			require.Equal(t, "1234567890", phone)
			return &User{
				ID:       1,
				Phone:    phone,
				NickName: "TestUser",
				Icon:     "http://example.com/icon.png",
				Password: string(hashed),
			}, nil
		}
		resp, err := srv.Login(context.Background(), &LoginReq{
			Phone:    "1234567890",
			Password: "password123",
		})
		require.NoError(t, err)
		require.NotEmpty(t, resp.Token)
		require.Equal(t, "TestUser", resp.NickName)

		tokenKey := BizUserToken + resp.Token
		require.True(t, mr.Exists(tokenKey))
		require.Equal(t, "1", mr.HGet(tokenKey, "id"))
		require.Equal(t, "TestUser", mr.HGet(tokenKey, "nickname"))
		require.Equal(t, "http://example.com/icon.png", mr.HGet(tokenKey, "icon"))
	})

	t.Run("user not found", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.getUserByPhoneFunc = func(ctx context.Context, phone string) (*User, error) {
			return nil, nil
		}
		_, err := srv.Login(context.Background(), &LoginReq{
			Phone:    "1234567890",
			Password: "password123",
		})
		require.ErrorIs(t, err, &errmsg.ErrUserNotFound)
	})

	t.Run("wrong password", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.getUserByPhoneFunc = func(ctx context.Context, phone string) (*User, error) {
			return &User{
				ID:       1,
				Phone:    phone,
				NickName: "TestUser",
				Icon:     "http://example.com/icon.png",
				Password: string(hashed),
			}, nil
		}
		_, err := srv.Login(context.Background(), &LoginReq{
			Phone:    "1234567890",
			Password: "wrongpassword",
		})
		require.ErrorIs(t, err, &errmsg.ErrInvalidPassword)
	})

	t.Run("repository error", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.getUserByPhoneFunc = func(ctx context.Context, phone string) (*User, error) {
			return nil, fmt.Errorf("db lost")
		}
		_, err := srv.Login(context.Background(), &LoginReq{
			Phone:    "1234567890",
			Password: "password123",
		})
		require.Error(t, err)
		require.NotErrorIs(t, err, &errmsg.ErrUserNotFound)
		require.Contains(t, err.Error(), "db lost")
	})
}

func TestSendCode(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv, _, mr := setupService(t)
		resp, err := srv.SendCode(context.Background(), &SendCodeReq{Phone: "12345678901"})
		require.NoError(t, err)
		require.Equal(t, "发送成功", resp.Message)

		codeKey := BizUserLoginCode + "12345678901"
		code, err := mr.Get(codeKey)
		require.NoError(t, err)
		require.Len(t, code, 6)
		for _, c := range code {
			require.Contains(t, "0123456789", string(c))
		}

		lockKey := BizUserLockCode + "12345678901"
		require.True(t, mr.Exists(lockKey))
	})

	t.Run("too many request", func(t *testing.T) {
		srv, _, mr := setupService(t)
		lockKey := BizUserLockCode + "12345678901"
		mr.Set(lockKey, "1")
		_, err := srv.SendCode(context.Background(), &SendCodeReq{Phone: "12345678901"})
		require.ErrorIs(t, err, &errmsg.ErrTooManyRequests)
	})

	t.Run("lock set success but code set fails cleans up lock", func(t *testing.T) {

	})
}

func TestCodeLogin(t *testing.T) {
	t.Run("code correct and user exists", func(t *testing.T) {
		srv, repo, mr := setupService(t)

		codeKey := BizUserLoginCode + "12345678901"
		mr.Set(codeKey, "654321")
		mr.SetTTL(codeKey, 5*time.Minute)

		repo.getUserByPhoneFunc = func(ctx context.Context, phone string) (*User, error) {
			return &User{
				ID:       1,
				Phone:    phone,
				NickName: "TestUser",
			}, nil
		}

		resp, err := srv.CodeLogin(context.Background(), &CodeLoginReq{
			Phone: "12345678901",
			Code:  "654321",
		})
		require.NoError(t, err)
		require.NotEmpty(t, resp.Token)
		require.Equal(t, "TestUser", resp.NickName)

		require.False(t, mr.Exists(codeKey))
		require.True(t, mr.Exists(BizUserToken+resp.Token))
	})

	t.Run("code correct and user not exist = auto register", func(t *testing.T) {
		srv, repo, mr := setupService(t)

		mr.Set(BizUserLoginCode+"12345678901", "654321")
		searchCount := 0
		repo.getUserByPhoneFunc = func(ctx context.Context, phone string) (*User, error) {
			searchCount++
			if searchCount == 1 {
				return nil, nil
			}
			return &User{
				ID:       2,
				Phone:    phone,
				NickName: "user_a1b2c",
			}, nil
		}
		repo.createUserFunc = func(_ context.Context, user *User) error {
			require.Equal(t, "12345678901", user.Phone)
			require.Contains(t, user.NickName, "user_")
			return nil
		}

		resp, err := srv.CodeLogin(context.Background(), &CodeLoginReq{
			Phone: "12345678901",
			Code:  "654321",
		})
		require.NoError(t, err)
		require.NotEmpty(t, resp.Token)

		require.Equal(t, 1, searchCount)
	})

	t.Run("code expired", func(t *testing.T) {
		srv, _, _ := setupService(t)
		_, err := srv.CodeLogin(context.Background(), &CodeLoginReq{
			Phone: "12345678901",
			Code:  "654321",
		})
		require.ErrorIs(t, err, &errmsg.ErrCodeExpired)
	})

	t.Run("code wrong", func(t *testing.T) {
		srv, _, mr := setupService(t)
		mr.Set(BizUserLoginCode+"12345678901", "654321")
		_, err := srv.CodeLogin(context.Background(), &CodeLoginReq{
			Phone: "12345678901",
			Code:  "123456",
		})

		require.ErrorIs(t, err, &errmsg.ErrInvalidCode)
	})
}

func TestRegister(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.createUserFunc = func(ctx context.Context, user *User) error {
			require.Equal(t, "1234567890", user.Phone)
			require.Contains(t, user.NickName, "user_")
			require.Empty(t, user.Password)
			return nil
		}
		user, err := srv.Register(context.Background(), &CreateUserReq{
			Phone: "1234567890",
		})
		require.NoError(t, err)
		require.Equal(t, "1234567890", user.Phone)
	})

	t.Run("with name", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.createUserFunc = func(ctx context.Context, user *User) error {
			require.Equal(t, "1234567890", user.Phone)
			require.Contains(t, user.NickName, "TestUser")
			require.Empty(t, user.Password)
			return nil
		}
		user, err := srv.Register(context.Background(), &CreateUserReq{
			Phone:    "1234567890",
			NickName: "TestUser",
		})
		require.NoError(t, err)
		require.Equal(t, "1234567890", user.Phone)
		require.Equal(t, "TestUser", user.NickName)
	})

	t.Run("with password", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		var savedPassword string
		repo.createUserFunc = func(ctx context.Context, user *User) error {
			savedPassword = user.Password
			require.NotEmpty(t, user.Password)
			require.NotEqual(t, "password123", user.Password, "密码应被哈希")
			return nil
		}
		user, err := srv.Register(context.Background(), &CreateUserReq{
			Phone:    "1234567890",
			Password: "password123",
		})
		require.NoError(t, err)
		require.Equal(t, "1234567890", user.Phone)

		err = bcrypt.CompareHashAndPassword([]byte(savedPassword), []byte("password123"))
		require.NoError(t, err)
	})
}
