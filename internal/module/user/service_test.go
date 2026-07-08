package user

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"dianping/pkg/errmsg"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type mockUserRepo struct {
	createUserFunc     func(ctx context.Context, user *User) error
	getUserByPhoneFunc func(ctx context.Context, phone string) (*User, error)
	getUserByIDFunc    func(ctx context.Context, userID uint64) (*User, error)
	listUsersByIDsFunc func(ctx context.Context, userIDs []uint64) ([]User, error)
}

func (m *mockUserRepo) CreateUser(ctx context.Context, user *User) error {
	if m.createUserFunc != nil {
		return m.createUserFunc(ctx, user)
	}
	return nil
}

func (m *mockUserRepo) GetUserByPhone(ctx context.Context, phone string) (*User, error) {
	if m.getUserByPhoneFunc != nil {
		return m.getUserByPhoneFunc(ctx, phone)
	}
	return nil, nil
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

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{}
}

func setUpUserService(t *testing.T) (*Service, *mockUserRepo, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() {
		require.NoError(t, rdb.Close())
	})
	repo := newMockUserRepo()
	return NewService(repo, rdb, nil), repo, mr
}

func TestService_Login(t *testing.T) {
	t.Run("login with correct password", func(t *testing.T) {
		service, repo, mr := setUpUserService(t)
		ctx := context.Background()

		hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
		require.NoError(t, err)

		// 设置模拟仓库返回的用户数据
		repo.getUserByPhoneFunc = func(ctx context.Context, phone string) (*User, error) {
			require.Equal(t, "18888888888", phone)
			return &User{
				ID:       1001,
				Phone:    phone,
				Password: string(hash),
				NickName: "Alice",
				Icon:     "/imgs/icon.png",
			}, nil
		}

		// 测试登录请求
		req := &LoginReq{
			Phone:    "18888888888",
			Password: "password123",
		}
		resp, err := service.Login(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.Token)
		require.Equal(t, "Alice", resp.NickName)

		// 验证Redis中是否存储了token
		tokenKey := BizUserToken + resp.Token
		require.Equal(t, "1001", mr.HGet(tokenKey, "id"))
		require.Equal(t, "Alice", mr.HGet(tokenKey, "nick_name"))
		require.Equal(t, "/imgs/icon.png", mr.HGet(tokenKey, "icon"))
	})
	t.Run("login with incorrect password", func(t *testing.T) {
		service, repo, _ := setUpUserService(t)
		ctx := context.Background()

		hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
		require.NoError(t, err)

		// 设置模拟仓库返回的用户数据
		repo.getUserByPhoneFunc = func(ctx context.Context, phone string) (*User, error) {
			require.Equal(t, "18888888888", phone)
			return &User{
				ID:       1001,
				Phone:    phone,
				Password: string(hash),
				NickName: "Alice",
				Icon:     "/imgs/icon.png",
			}, nil
		}

		// 测试登录请求，使用错误的密码
		req := &LoginReq{
			Phone:    "18888888888",
			Password: "wrongpassword",
		}
		resp, err := service.Login(ctx, req)
		require.Error(t, err)
		require.Nil(t, resp)
		require.Equal(t, &errmsg.ErrInvalidPassword, err)
	})
	t.Run("login with non-existent user", func(t *testing.T) {
		service, repo, _ := setUpUserService(t)
		ctx := context.Background()

		// 设置模拟仓库返回nil，表示用户不存在
		repo.getUserByPhoneFunc = func(ctx context.Context, phone string) (*User, error) {
			require.Equal(t, "18888888888", phone)
			return nil, nil
		}

		// 测试登录请求，使用不存在的手机号
		req := &LoginReq{
			Phone:    "18888888888",
			Password: "password123",
		}
		resp, err := service.Login(ctx, req)
		require.Error(t, err)
		require.Nil(t, resp)
		require.Equal(t, &errmsg.ErrUserNotFound, err)
	})
}

func TestService_CodeLogin(t *testing.T) {
	t.Run("code login with correct user", func(t *testing.T) {
		service, repo, mr := setUpUserService(t)
		ctx := context.Background()

		key := BizUserLoginCode + "18888888888"
		mr.Set(key, "123456")

		// 设置模拟仓库返回的用户数据
		repo.getUserByPhoneFunc = func(ctx context.Context, phone string) (*User, error) {
			require.Equal(t, "18888888888", phone)
			return &User{
				ID:       1001,
				Phone:    phone,
				NickName: "Alice",
				Icon:     "/imgs/icon.png",
			}, nil
		}

		// 测试验证码登录请求
		req := &CodeLoginReq{
			Phone: "18888888888",
			Code:  "123456",
		}
		resp, err := service.CodeLogin(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.Token)
		require.Equal(t, "Alice", resp.NickName)

		// 验证Redis中是否存储了token
		tokenKey := BizUserToken + resp.Token
		require.Equal(t, "1001", mr.HGet(tokenKey, "id"))
		require.Equal(t, "Alice", mr.HGet(tokenKey, "nick_name"))
		require.Equal(t, "/imgs/icon.png", mr.HGet(tokenKey, "icon"))
	})
	t.Run("code login with wrong code", func(t *testing.T) {
		service, _, mr := setUpUserService(t)
		ctx := context.Background()

		key := BizUserLoginCode + "18888888888"
		mr.Set(key, "123456")

		// 测试验证码登录请求
		req := &CodeLoginReq{
			Phone: "18888888888",
			Code:  "223456",
		}
		resp, err := service.CodeLogin(ctx, req)
		require.Error(t, err)
		require.Nil(t, resp)
		require.Equal(t, &errmsg.ErrInvalidCode, err)
	})
	t.Run("code login with non-existent user auto-registers", func(t *testing.T) {
		service, repo, mr := setUpUserService(t)
		ctx := context.Background()

		key := BizUserLoginCode + "18888888888"
		mr.Set(key, "123456")

		// 设置模拟仓库：用户不存在，但创建成功
		repo.getUserByPhoneFunc = func(ctx context.Context, phone string) (*User, error) {
			require.Equal(t, "18888888888", phone)
			return nil, nil
		}
		repo.createUserFunc = func(ctx context.Context, user *User) error {
			user.ID = 2001
			return nil
		}

		req := &CodeLoginReq{
			Phone: "18888888888",
			Code:  "123456",
		}
		resp, err := service.CodeLogin(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.Token)

		tokenKey := BizUserToken + resp.Token
		require.Equal(t, "2001", mr.HGet(tokenKey, "id"))
	})
	t.Run("code login with expired code", func(t *testing.T) {
		service, _, _ := setUpUserService(t)
		ctx := context.Background()

		// 不预先设置验证码，模拟验证码已过期（Redis 中不存在）
		req := &CodeLoginReq{
			Phone: "18888888888",
			Code:  "123456",
		}
		resp, err := service.CodeLogin(ctx, req)
		require.Error(t, err)
		require.Nil(t, resp)
		require.Equal(t, &errmsg.ErrCodeExpired, err)
	})
}

func TestService_Logout(t *testing.T) {
	t.Run("logout successfully deletes token", func(t *testing.T) {
		service, _, mr := setUpUserService(t)
		ctx := context.Background()

		token := "test-token-uuid"
		tokenKey := BizUserToken + token
		mr.HSet(tokenKey, "id", "1001")
		mr.HSet(tokenKey, "nick_name", "Alice")

		err := service.Logout(ctx, 1001, token)
		require.NoError(t, err)
		// 验证 token 已从 Redis 中删除
		require.False(t, mr.Exists(tokenKey))
	})

	t.Run("logout with non-existent token returns nil error", func(t *testing.T) {
		service, _, _ := setUpUserService(t)
		ctx := context.Background()

		// token 不存在时应静默成功，不报错
		err := service.Logout(ctx, 1001, "non-existent-token")
		require.NoError(t, err)
	})

	t.Run("logout with mismatched user ID returns ErrUnauthorized", func(t *testing.T) {
		service, _, mr := setUpUserService(t)
		ctx := context.Background()

		token := "test-token-uuid"
		tokenKey := BizUserToken + token
		mr.HSet(tokenKey, "id", "1001")

		err := service.Logout(ctx, 9999, token)
		require.Error(t, err)
		require.Equal(t, &errmsg.ErrUnauthorized, err)
		// 验证 token 未被误删
		require.True(t, mr.Exists(tokenKey))
	})
}

func TestService_SendCode(t *testing.T) {
	t.Run("send code successfully stores code and lock", func(t *testing.T) {
		service, _, mr := setUpUserService(t)
		ctx := context.Background()

		req := &SendCodeReq{Phone: "18888888888"}
		resp, err := service.SendCode(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "发送成功", resp.Message)

		// 验证分布式锁已设置
		lockKey := BizUserLockCode + "18888888888"
		lockVal, _ := mr.Get(lockKey)
		require.Equal(t, "1", lockVal)

		// 验证验证码已存储且为 6 位数字
		codeKey := BizUserLoginCode + "18888888888"
		code, _ := mr.Get(codeKey)
		require.NotEmpty(t, code)
		require.Len(t, code, 6)
	})

	t.Run("send code too frequently returns ErrTooManyRequests", func(t *testing.T) {
		service, _, mr := setUpUserService(t)
		ctx := context.Background()

		// 预置锁，模拟 1 分钟内已发送过验证码
		lockKey := BizUserLockCode + "18888888888"
		mr.Set(lockKey, "1")

		req := &SendCodeReq{Phone: "18888888888"}
		resp, err := service.SendCode(ctx, req)
		require.Error(t, err)
		require.Nil(t, resp)
		require.Equal(t, &errmsg.ErrTooManyRequests, err)

		// 验证没有生成新的验证码
		codeKey := BizUserLoginCode + "18888888888"
		code, _ := mr.Get(codeKey)
		require.Empty(t, code)
	})
}

func TestService_Register(t *testing.T) {
	t.Run("register successfully with password and nickname", func(t *testing.T) {
		service, repo, _ := setUpUserService(t)
		ctx := context.Background()

		repo.createUserFunc = func(ctx context.Context, user *User) error {
			user.ID = 1001 // 模拟数据库自增 ID
			// 密码应为 bcrypt 哈希而非明文
			require.NotEqual(t, "password123", user.Password)
			err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte("password123"))
			require.NoError(t, err)
			require.Equal(t, "18888888888", user.Phone)
			require.Equal(t, "Alice", user.NickName)
			return nil
		}

		req := &CreateUserReq{
			Phone:    "18888888888",
			Password: "password123",
			NickName: "Alice",
		}
		user, err := service.Register(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, user)
		require.Equal(t, uint64(1001), user.ID)
		require.Equal(t, "Alice", user.NickName)
	})

	t.Run("register without nickname auto-generates nickname", func(t *testing.T) {
		service, repo, _ := setUpUserService(t)
		ctx := context.Background()

		repo.createUserFunc = func(ctx context.Context, user *User) error {
			user.ID = 1002
			// 验证自动生成的昵称以 "user_" 开头
			require.Contains(t, user.NickName, "user_")
			return nil
		}

		req := &CreateUserReq{
			Phone:    "18888888888",
			Password: "password123",
			NickName: "", // 空昵称触发自动生成
		}
		user, err := service.Register(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, user)
	})

	t.Run("register without password stores empty password", func(t *testing.T) {
		service, repo, _ := setUpUserService(t)
		ctx := context.Background()

		repo.createUserFunc = func(ctx context.Context, user *User) error {
			user.ID = 1003
			require.Empty(t, user.Password)
			return nil
		}

		req := &CreateUserReq{
			Phone:    "18888888888",
			Password: "",
			NickName: "Bob",
		}
		user, err := service.Register(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, user)
	})

	t.Run("duplicate phone returns existing user", func(t *testing.T) {
		service, repo, _ := setUpUserService(t)
		ctx := context.Background()

		existingUser := &User{
			ID:       1001,
			Phone:    "18888888888",
			NickName: "Alice",
		}
		repo.createUserFunc = func(ctx context.Context, user *User) error {
			return &mysql.MySQLError{Number: 1062, Message: "Duplicate entry"}
		}
		repo.getUserByPhoneFunc = func(ctx context.Context, phone string) (*User, error) {
			return existingUser, nil
		}

		req := &CreateUserReq{
			Phone:    "18888888888",
			Password: "password123",
			NickName: "Alice",
		}
		user, err := service.Register(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, user)
		require.Equal(t, existingUser.ID, user.ID)
	})

	t.Run("duplicate phone with fallback query failure returns ErrUserAlreadyExists", func(t *testing.T) {
		service, repo, _ := setUpUserService(t)
		ctx := context.Background()

		repo.createUserFunc = func(ctx context.Context, user *User) error {
			return &mysql.MySQLError{Number: 1062, Message: "Duplicate entry"}
		}
		repo.getUserByPhoneFunc = func(ctx context.Context, phone string) (*User, error) {
			return nil, errors.New("db connection lost")
		}

		req := &CreateUserReq{
			Phone:    "18888888888",
			Password: "password123",
			NickName: "Alice",
		}
		user, err := service.Register(ctx, req)
		require.Error(t, err)
		require.Nil(t, user)
		require.Equal(t, &errmsg.ErrUserAlreadyExists, err)
	})
}

func TestService_GetUserByID(t *testing.T) {
	t.Run("cache hit returns user from cache without querying DB", func(t *testing.T) {
		service, _, mr := setUpUserService(t)
		ctx := context.Background()

		// 预热缓存：将用户数据 JSON 序列化后存入 Redis
		cachedUser := User{ID: 1001, NickName: "Alice", Icon: "/imgs/icon.png"}
		cacheKey := CacheUserKey + strconv.FormatUint(1001, 10)
		bytes, err := json.Marshal(cachedUser)
		require.NoError(t, err)
		mr.Set(cacheKey, string(bytes))

		// 不设置 repo mock —— 如果走了 DB 路径会 nil panic，验证缓存命中
		dto, err := service.GetUserByID(ctx, 1001)
		require.NoError(t, err)
		require.NotNil(t, dto)
		require.Equal(t, uint64(1001), dto.ID)
		require.Equal(t, "Alice", dto.NickName)
		require.Equal(t, "/imgs/icon.png", dto.Icon)
	})

	t.Run("cache miss queries DB and caches result", func(t *testing.T) {
		service, repo, mr := setUpUserService(t)
		ctx := context.Background()

		repo.getUserByIDFunc = func(ctx context.Context, userID uint64) (*User, error) {
			require.Equal(t, uint64(1001), userID)
			return &User{ID: 1001, NickName: "Alice", Icon: "/imgs/icon.png"}, nil
		}

		dto, err := service.GetUserByID(ctx, 1001)
		require.NoError(t, err)
		require.NotNil(t, dto)
		require.Equal(t, "Alice", dto.NickName)

		// 验证结果已被回写缓存
		cacheKey := CacheUserKey + strconv.FormatUint(1001, 10)
		cached, _ := mr.Get(cacheKey)
		require.NotEmpty(t, cached)
	})

	t.Run("cache miss with no DB record returns zero-value DTO", func(t *testing.T) {
		service, repo, mr := setUpUserService(t)
		ctx := context.Background()

		repo.getUserByIDFunc = func(ctx context.Context, userID uint64) (*User, error) {
			return nil, nil
		}

		// 已知行为：GetUserByID 返回 *User(nil) 装箱到 any 后不为 nil，
		// QueryWithPassThrough 会将其 JSON 序列化为 "null" 写入缓存，
		// 并反序列化得到零值 User 后返回 UserDTO，不报错。
		dto, err := service.GetUserByID(ctx, 9999)
		require.NoError(t, err)
		require.NotNil(t, dto)
		require.Equal(t, uint64(0), dto.ID)
		require.Equal(t, "", dto.NickName)

		// 验证缓存中存储了 "null"（非空值标记 ""）
		cacheKey := CacheUserKey + strconv.FormatUint(9999, 10)
		nullVal, _ := mr.Get(cacheKey)
		require.Equal(t, "null", nullVal)
	})
}

func TestService_ListUsersByIDs(t *testing.T) {
	t.Run("list users by IDs returns DTOs in order", func(t *testing.T) {
		service, repo, _ := setUpUserService(t)
		ctx := context.Background()

		repo.listUsersByIDsFunc = func(ctx context.Context, userIDs []uint64) ([]User, error) {
			require.Equal(t, []uint64{1001, 1002}, userIDs)
			return []User{
				{ID: 1001, NickName: "Alice", Icon: "/imgs/a.png"},
				{ID: 1002, NickName: "Bob", Icon: "/imgs/b.png"},
			}, nil
		}

		dtos, err := service.ListUsersByIDs(ctx, []uint64{1001, 1002})
		require.NoError(t, err)
		require.Len(t, dtos, 2)
		require.Equal(t, uint64(1001), dtos[0].ID)
		require.Equal(t, "Alice", dtos[0].NickName)
		require.Equal(t, uint64(1002), dtos[1].ID)
		require.Equal(t, "Bob", dtos[1].NickName)
	})

	t.Run("empty user IDs returns empty slice", func(t *testing.T) {
		service, repo, _ := setUpUserService(t)
		ctx := context.Background()

		repo.listUsersByIDsFunc = func(ctx context.Context, userIDs []uint64) ([]User, error) {
			return []User{}, nil
		}

		dtos, err := service.ListUsersByIDs(ctx, []uint64{})
		require.NoError(t, err)
		require.Empty(t, dtos)
	})

	t.Run("repository error is propagated", func(t *testing.T) {
		service, repo, _ := setUpUserService(t)
		ctx := context.Background()

		dbErr := errors.New("db connection lost")
		repo.listUsersByIDsFunc = func(ctx context.Context, userIDs []uint64) ([]User, error) {
			return nil, dbErr
		}

		dtos, err := service.ListUsersByIDs(ctx, []uint64{1001})
		require.Error(t, err)
		require.Nil(t, dtos)
		require.Equal(t, dbErr, err)
	})
}

func TestService_Sign(t *testing.T) {
	t.Run("sign successfully sets bitmap for today", func(t *testing.T) {
		service, _, mr := setUpUserService(t)
		ctx := context.Background()

		err := service.Sign(ctx, 1001)
		require.NoError(t, err)

		// 验证 bitmap 中有数据写入：查找 sign key
		keys := mr.Keys()
		require.NotEmpty(t, keys)
		// 至少有一个 key 以 BizUserSignKey 前缀开头
		found := false
		for _, k := range keys {
			if len(k) > len(BizUserSignKey) && k[:len(BizUserSignKey)] == BizUserSignKey {
				found = true
				break
			}
		}
		require.True(t, found, "expected a sign bitmap key to exist")
	})
}

func TestService_SignCount(t *testing.T) {
	// miniredis v2 不支持 BITFIELD 命令，SignCount 依赖此命令，
	// 需要在集成测试中覆盖（连接真实 Redis 或使用支持 BITFIELD 的 mock）。
	t.Skip("miniredis v2 does not support the BITFIELD command")
}

func TestService_toUserDTOs(t *testing.T) {
	t.Run("converts users to DTOs preserving all fields", func(t *testing.T) {
		users := []User{
			{ID: 1, NickName: "Alice", Icon: "/a.png"},
			{ID: 2, NickName: "Bob", Icon: "/b.png"},
		}
		dtos := toUserDTOs(users)
		require.Len(t, dtos, 2)
		require.Equal(t, uint64(1), dtos[0].ID)
		require.Equal(t, "Alice", dtos[0].NickName)
		require.Equal(t, "/a.png", dtos[0].Icon)
		require.Equal(t, uint64(2), dtos[1].ID)
		require.Equal(t, "Bob", dtos[1].NickName)
		require.Equal(t, "/b.png", dtos[1].Icon)
	})

	t.Run("empty input returns empty slice", func(t *testing.T) {
		dtos := toUserDTOs([]User{})
		require.Empty(t, dtos)
	})

	t.Run("nil input returns empty slice", func(t *testing.T) {
		dtos := toUserDTOs(nil)
		require.Empty(t, dtos)
	})
}
