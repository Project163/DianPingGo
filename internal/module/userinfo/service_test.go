package userinfo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// mockUserRepository implements UserRepository for service testing.
type mockUserRepository struct {
	getUserInfoByUserIDFunc func(ctx context.Context, userID uint64) (*UserInfo, error)
	createUserInfoFunc      func(ctx context.Context, userInfoDTO *UserInfoDTO) error
}

func (m *mockUserRepository) GetUserInfoByUserID(ctx context.Context, userID uint64) (*UserInfo, error) {
	if m.getUserInfoByUserIDFunc != nil {
		return m.getUserInfoByUserIDFunc(ctx, userID)
	}
	return nil, nil
}

func (m *mockUserRepository) CreateUserInfo(ctx context.Context, userInfoDTO *UserInfoDTO) error {
	if m.createUserInfoFunc != nil {
		return m.createUserInfoFunc(ctx, userInfoDTO)
	}
	return nil
}

func newMockUserRepo() *mockUserRepository {
	return &mockUserRepository{}
}

// setUpUserInfoService creates a Service with a mock repo and miniredis instance.
func setUpUserInfoService(t *testing.T) (*Service, *mockUserRepository, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() {
		require.NoError(t, rdb.Close())
	})

	repo := newMockUserRepo()
	// userinfo service does not use Redis directly; rdb is provided for
	// consistency with the project-wide test pattern and future extensions.
	_ = rdb
	return NewService(repo), repo, mr
}

// =============================================================================
// GetUserInfoByUserID
// =============================================================================

func TestService_GetUserInfoByUserID(t *testing.T) {
	t.Run("returns DTO when user info exists", func(t *testing.T) {
		service, repo, _ := setUpUserInfoService(t)
		ctx := context.Background()

		birthday := time.Date(1995, 6, 15, 0, 0, 0, 0, time.UTC)
		repo.getUserInfoByUserIDFunc = func(ctx context.Context, userID uint64) (*UserInfo, error) {
			require.Equal(t, uint64(1001), userID)
			return &UserInfo{
				UserID:    1001,
				City:      "Shanghai",
				Introduce: "Hello world",
				Fans:      50,
				Followee:  30,
				Gender:    1,
				Birthday:  birthday,
				Credits:   150,
				Level:     5,
			}, nil
		}

		dto, err := service.GetUserInfoByUserID(ctx, 1001)
		require.NoError(t, err)
		require.NotNil(t, dto)
		require.Equal(t, uint64(1001), dto.UserID)
		require.Equal(t, "Shanghai", dto.City)
		require.Equal(t, "Hello world", dto.Introduce)
		require.Equal(t, uint32(50), dto.Fans)
		require.Equal(t, uint32(30), dto.Followee)
		require.Equal(t, uint8(1), dto.Gender)
		require.Equal(t, birthday, dto.Birthday)
		require.Equal(t, uint32(150), dto.Credits)
		require.Equal(t, uint8(5), dto.Level)
	})

	t.Run("returns nil when user info not found", func(t *testing.T) {
		service, repo, _ := setUpUserInfoService(t)
		ctx := context.Background()

		repo.getUserInfoByUserIDFunc = func(ctx context.Context, userID uint64) (*UserInfo, error) {
			return nil, nil
		}

		dto, err := service.GetUserInfoByUserID(ctx, 9999)
		require.NoError(t, err)
		require.Nil(t, dto)
	})

	t.Run("propagates repository error", func(t *testing.T) {
		service, repo, _ := setUpUserInfoService(t)
		ctx := context.Background()

		dbErr := errors.New("connection refused")
		repo.getUserInfoByUserIDFunc = func(ctx context.Context, userID uint64) (*UserInfo, error) {
			return nil, dbErr
		}

		dto, err := service.GetUserInfoByUserID(ctx, 1001)
		require.Error(t, err)
		require.Nil(t, dto)
		require.Equal(t, dbErr, err)
	})

	t.Run("handles zero userID", func(t *testing.T) {
		service, repo, _ := setUpUserInfoService(t)
		ctx := context.Background()

		repo.getUserInfoByUserIDFunc = func(ctx context.Context, userID uint64) (*UserInfo, error) {
			require.Equal(t, uint64(0), userID)
			return nil, nil
		}

		dto, err := service.GetUserInfoByUserID(ctx, 0)
		require.NoError(t, err)
		require.Nil(t, dto)
	})
}

// =============================================================================
// CreateUserInfo
// =============================================================================

func TestService_CreateUserInfo(t *testing.T) {
	t.Run("creates user info successfully", func(t *testing.T) {
		service, repo, _ := setUpUserInfoService(t)
		ctx := context.Background()

		var captured *UserInfoDTO
		repo.createUserInfoFunc = func(ctx context.Context, dto *UserInfoDTO) error {
			captured = dto
			return nil
		}

		birthday := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		input := &UserInfoDTO{
			UserID:    2001,
			City:      "Beijing",
			Introduce: "New here",
			Fans:      0,
			Followee:  0,
			Gender:    2,
			Birthday:  birthday,
			Credits:   10,
			Level:     1,
		}

		err := service.CreateUserInfo(ctx, input)
		require.NoError(t, err)
		require.NotNil(t, captured)
		require.Equal(t, uint64(2001), captured.UserID)
		require.Equal(t, "Beijing", captured.City)
		require.Equal(t, "New here", captured.Introduce)
		require.Equal(t, uint8(2), captured.Gender)
	})

	t.Run("propagates repository error on create", func(t *testing.T) {
		service, repo, _ := setUpUserInfoService(t)
		ctx := context.Background()

		dbErr := errors.New("duplicate entry")
		repo.createUserInfoFunc = func(ctx context.Context, dto *UserInfoDTO) error {
			return dbErr
		}

		input := &UserInfoDTO{
			UserID: 1001,
			City:   "Shanghai",
		}
		err := service.CreateUserInfo(ctx, input)
		require.Error(t, err)
		require.Equal(t, dbErr, err)
	})

	t.Run("creates with empty fields", func(t *testing.T) {
		service, repo, _ := setUpUserInfoService(t)
		ctx := context.Background()

		var captured *UserInfoDTO
		repo.createUserInfoFunc = func(ctx context.Context, dto *UserInfoDTO) error {
			captured = dto
			return nil
		}

		input := &UserInfoDTO{
			UserID: 3001,
		}

		err := service.CreateUserInfo(ctx, input)
		require.NoError(t, err)
		require.NotNil(t, captured)
		require.Equal(t, uint64(3001), captured.UserID)
		require.Equal(t, "", captured.City)
		require.Equal(t, "", captured.Introduce)
		require.Equal(t, uint32(0), captured.Fans)
	})
}

// =============================================================================
// toUserInfoDTO
// =============================================================================

func TestService_toUserInfoDTO(t *testing.T) {
	service, _, _ := setUpUserInfoService(t)

	t.Run("maps all fields correctly", func(t *testing.T) {
		birthday := time.Date(1992, 3, 20, 0, 0, 0, 0, time.UTC)
		userInfo := &UserInfo{
			UserID:    1001,
			City:      "Guangzhou",
			Introduce: "Experienced",
			Fans:      200,
			Followee:  80,
			Gender:    2,
			Birthday:  birthday,
			Credits:   500,
			Level:     10,
		}

		dto := service.toUserInfoDTO(userInfo)
		require.NotNil(t, dto)
		require.Equal(t, userInfo.UserID, dto.UserID)
		require.Equal(t, userInfo.City, dto.City)
		require.Equal(t, userInfo.Introduce, dto.Introduce)
		require.Equal(t, userInfo.Fans, dto.Fans)
		require.Equal(t, userInfo.Followee, dto.Followee)
		require.Equal(t, userInfo.Gender, dto.Gender)
		require.Equal(t, userInfo.Birthday, dto.Birthday)
		require.Equal(t, userInfo.Credits, dto.Credits)
		require.Equal(t, userInfo.Level, dto.Level)
	})

	t.Run("handles zero values", func(t *testing.T) {
		userInfo := &UserInfo{
			UserID: 0,
		}

		dto := service.toUserInfoDTO(userInfo)
		require.NotNil(t, dto)
		require.Equal(t, uint64(0), dto.UserID)
		require.Equal(t, "", dto.City)
		require.Equal(t, uint32(0), dto.Fans)
		require.Equal(t, uint8(0), dto.Gender)
	})
}
