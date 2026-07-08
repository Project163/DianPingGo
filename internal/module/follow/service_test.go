package follow

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"dianping/internal/module/user"
	"dianping/pkg/errmsg"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Mock Implementations for Service Tests
// =============================================================================

type mockFollowRepoForService struct {
	followFunc              func(ctx context.Context, userID, followUserID uint64) (bool, error)
	unfollowFunc            func(ctx context.Context, userID, followUserID uint64) (bool, error)
	isFollowedFunc          func(ctx context.Context, userID, followUserID uint64) (bool, error)
	listFollowerUserIDsFunc func(ctx context.Context, userID uint64) ([]uint64, error)
	listFollowedUserIDsFunc func(ctx context.Context, userID uint64) ([]uint64, error)
}

func (m *mockFollowRepoForService) Follow(ctx context.Context, userID, followUserID uint64) (bool, error) {
	if m.followFunc != nil {
		return m.followFunc(ctx, userID, followUserID)
	}
	return false, nil
}

func (m *mockFollowRepoForService) Unfollow(ctx context.Context, userID, followUserID uint64) (bool, error) {
	if m.unfollowFunc != nil {
		return m.unfollowFunc(ctx, userID, followUserID)
	}
	return false, nil
}

func (m *mockFollowRepoForService) IsFollowed(ctx context.Context, userID, followUserID uint64) (bool, error) {
	if m.isFollowedFunc != nil {
		return m.isFollowedFunc(ctx, userID, followUserID)
	}
	return false, nil
}

func (m *mockFollowRepoForService) ListFollowerUserIDs(ctx context.Context, userID uint64) ([]uint64, error) {
	if m.listFollowerUserIDsFunc != nil {
		return m.listFollowerUserIDsFunc(ctx, userID)
	}
	return nil, nil
}

func (m *mockFollowRepoForService) ListFollowedUserIDs(ctx context.Context, userID uint64) ([]uint64, error) {
	if m.listFollowedUserIDsFunc != nil {
		return m.listFollowedUserIDsFunc(ctx, userID)
	}
	return nil, nil
}

type mockUserSrvForService struct {
	listUsersByIDsFunc func(ctx context.Context, userIDs []uint64) ([]user.UserDTO, error)
}

func (m *mockUserSrvForService) ListUsersByIDs(ctx context.Context, userIDs []uint64) ([]user.UserDTO, error) {
	if m.listUsersByIDsFunc != nil {
		return m.listUsersByIDsFunc(ctx, userIDs)
	}
	return nil, nil
}

// =============================================================================
// Test Helper
// =============================================================================

// setUpFollowService creates a Service with mock repository, mock user service,
// and a miniredis instance for testing.
func setUpFollowService(t *testing.T) (*Service, *mockFollowRepoForService, *mockUserSrvForService, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })

	repo := new(mockFollowRepoForService)
	userSrv := new(mockUserSrvForService)
	service := NewService(repo, userSrv, rdb)
	return service, repo, userSrv, mr
}

// =============================================================================
// Follow
// =============================================================================

func TestService_Follow(t *testing.T) {
	t.Run("follow user successfully", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		called := false
		repo.followFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			called = true
			require.Equal(t, uint64(1), userID)
			require.Equal(t, uint64(2), followUserID)
			return true, nil
		}

		f := &Follow{UserID: 1, FollowUserID: 2}
		err := service.Follow(ctx, f)
		require.NoError(t, err)
		require.True(t, called)
	})

	t.Run("follow yourself returns ErrFollowYourself", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.followFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			t.Fatalf("repo should not be called for self-follow")
			return false, nil
		}

		f := &Follow{UserID: 1, FollowUserID: 1}
		err := service.Follow(ctx, f)
		require.Error(t, err)
		var ce *errmsg.CustomError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, errmsg.ErrFollowYourself.BusinessCode, ce.BusinessCode)
	})

	t.Run("follow with repo error returns ErrInternalSec", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.followFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			return false, fmt.Errorf("db connection lost")
		}

		f := &Follow{UserID: 1, FollowUserID: 2}
		err := service.Follow(ctx, f)
		require.Error(t, err)
		var ce *errmsg.CustomError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, errmsg.ErrInternalSec.BusinessCode, ce.BusinessCode)
	})

	t.Run("follow updates cache when cache is hot", func(t *testing.T) {
		service, repo, _, mr := setUpFollowService(t)
		ctx := context.Background()

		// Pre-warm the cache: set loadKey to make the cache "hot"
		key, loadKey := followCacheKey(1)
		mr.Set(loadKey, "1")
		mr.SetTTL(loadKey, BizFollowerTTL)

		repo.followFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			return true, nil
		}

		f := &Follow{UserID: 1, FollowUserID: 2}
		err := service.Follow(ctx, f)
		require.NoError(t, err)

		// Verify the cache was updated: followUserID should be in the set
		members, err := mr.SMembers(key)
		require.NoError(t, err)
		require.Contains(t, members, strconv.FormatUint(2, 10))
	})
}

// =============================================================================
// Unfollow
// =============================================================================

func TestService_Unfollow(t *testing.T) {
	t.Run("unfollow user successfully", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		called := false
		repo.unfollowFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			called = true
			require.Equal(t, uint64(1), userID)
			require.Equal(t, uint64(2), followUserID)
			return true, nil
		}

		f := &Follow{UserID: 1, FollowUserID: 2}
		err := service.Unfollow(ctx, f)
		require.NoError(t, err)
		require.True(t, called)
	})

	t.Run("unfollow yourself returns ErrFollowYourself", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.unfollowFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			t.Fatalf("repo should not be called for self-unfollow")
			return false, nil
		}

		f := &Follow{UserID: 1, FollowUserID: 1}
		err := service.Unfollow(ctx, f)
		require.Error(t, err)
		var ce *errmsg.CustomError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, errmsg.ErrFollowYourself.BusinessCode, ce.BusinessCode)
	})

	t.Run("unfollow with repo error returns ErrInternalSec", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.unfollowFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			return false, fmt.Errorf("db connection lost")
		}

		f := &Follow{UserID: 1, FollowUserID: 2}
		err := service.Unfollow(ctx, f)
		require.Error(t, err)
		var ce *errmsg.CustomError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, errmsg.ErrInternalSec.BusinessCode, ce.BusinessCode)
	})

	t.Run("unfollow removes from cache when cache is hot", func(t *testing.T) {
		service, repo, _, mr := setUpFollowService(t)
		ctx := context.Background()

		// Pre-warm the cache with followUserID=2 in the set
		key, loadKey := followCacheKey(1)
		_, err := mr.SAdd(key, "2")
		require.NoError(t, err)
		mr.Set(loadKey, "1")
		mr.SetTTL(loadKey, BizFollowerTTL)

		repo.unfollowFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			return true, nil
		}

		f := &Follow{UserID: 1, FollowUserID: 2}
		err = service.Unfollow(ctx, f)
		require.NoError(t, err)

		// Verify the user was removed from the cache set
		members, err := mr.SMembers(key)
		require.NoError(t, err)
		require.NotContains(t, members, strconv.FormatUint(2, 10))
	})
}

// =============================================================================
// IsFollowed
// =============================================================================

func TestService_IsFollowed(t *testing.T) {
	t.Run("user is followed returns true", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.isFollowedFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			require.Equal(t, uint64(1), userID)
			require.Equal(t, uint64(2), followUserID)
			return true, nil
		}

		isFollowed, err := service.IsFollowed(ctx, 1, 2)
		require.NoError(t, err)
		require.True(t, isFollowed)
	})

	t.Run("user is not followed returns false", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.isFollowedFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			return false, nil
		}

		isFollowed, err := service.IsFollowed(ctx, 1, 999)
		require.NoError(t, err)
		require.False(t, isFollowed)
	})

	t.Run("repo error returns ErrInternalSec", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.isFollowedFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			return false, fmt.Errorf("db connection lost")
		}

		isFollowed, err := service.IsFollowed(ctx, 1, 2)
		require.Error(t, err)
		require.False(t, isFollowed)
		var ce *errmsg.CustomError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, errmsg.ErrInternalSec.BusinessCode, ce.BusinessCode)
	})
}

// =============================================================================
// ListFollowerUserIDs
// =============================================================================

func TestService_ListFollowerUserIDs(t *testing.T) {
	t.Run("returns follower IDs", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.listFollowerUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			require.Equal(t, uint64(1), userID)
			return []uint64{2, 3, 4}, nil
		}

		ids, err := service.ListFollowerUserIDs(ctx, 1)
		require.NoError(t, err)
		require.Equal(t, []uint64{2, 3, 4}, ids)
	})

	t.Run("returns empty slice when no followers", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.listFollowerUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			return []uint64{}, nil
		}

		ids, err := service.ListFollowerUserIDs(ctx, 1)
		require.NoError(t, err)
		require.Empty(t, ids)
	})

	t.Run("repo error returns ErrInternalSec", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.listFollowerUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			return nil, fmt.Errorf("db connection lost")
		}

		ids, err := service.ListFollowerUserIDs(ctx, 1)
		require.Error(t, err)
		require.Nil(t, ids)
		var ce *errmsg.CustomError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, errmsg.ErrInternalSec.BusinessCode, ce.BusinessCode)
	})
}

// =============================================================================
// ListFollowedUserIDs
// =============================================================================

func TestService_ListFollowedUserIDs(t *testing.T) {
	t.Run("returns followed user IDs", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			require.Equal(t, uint64(1), userID)
			return []uint64{5, 6}, nil
		}

		ids, err := service.ListFollowedUserIDs(ctx, 1)
		require.NoError(t, err)
		require.Equal(t, []uint64{5, 6}, ids)
	})

	t.Run("returns empty slice when no followed users", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			return []uint64{}, nil
		}

		ids, err := service.ListFollowedUserIDs(ctx, 1)
		require.NoError(t, err)
		require.Empty(t, ids)
	})

	t.Run("repo error returns ErrInternalSec", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			return nil, fmt.Errorf("db connection lost")
		}

		ids, err := service.ListFollowedUserIDs(ctx, 1)
		require.Error(t, err)
		require.Nil(t, ids)
		var ce *errmsg.CustomError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, errmsg.ErrInternalSec.BusinessCode, ce.BusinessCode)
	})
}

// =============================================================================
// FollowCommon
// =============================================================================

func TestService_FollowCommon(t *testing.T) {
	t.Run("returns common users", func(t *testing.T) {
		service, repo, userSrv, _ := setUpFollowService(t)
		ctx := context.Background()

		// ensureCache loads followed IDs from repo for user 1 and user 2
		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			if userID == 1 {
				return []uint64{2, 3, 4}, nil
			}
			return []uint64{2, 4, 5}, nil
		}

		// SInter will find common IDs {2, 4}
		userSrv.listUsersByIDsFunc = func(ctx context.Context, userIDs []uint64) ([]user.UserDTO, error) {
			require.ElementsMatch(t, []uint64{2, 4}, userIDs)
			return []user.UserDTO{
				{ID: 2, NickName: "Bob", Icon: "/imgs/bob.png"},
				{ID: 4, NickName: "Dave", Icon: "/imgs/dave.png"},
			}, nil
		}

		commonUsers, err := service.FollowCommon(ctx, 1, 2)
		require.NoError(t, err)
		require.Len(t, commonUsers, 2)
	})

	t.Run("returns empty when no common users", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			if userID == 1 {
				return []uint64{2, 3}, nil
			}
			return []uint64{4, 5}, nil // no overlap
		}

		commonUsers, err := service.FollowCommon(ctx, 1, 2)
		require.NoError(t, err)
		require.Empty(t, commonUsers)
	})

	t.Run("returns empty when one user follows nobody", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			if userID == 1 {
				return []uint64{2, 3}, nil
			}
			return []uint64{}, nil // user 2 follows nobody
		}

		commonUsers, err := service.FollowCommon(ctx, 1, 2)
		require.NoError(t, err)
		require.Empty(t, commonUsers)
	})

	t.Run("ensureCache repo error returns ErrInternalSec", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			return nil, fmt.Errorf("db connection lost")
		}

		commonUsers, err := service.FollowCommon(ctx, 1, 2)
		require.Error(t, err)
		require.Nil(t, commonUsers)
		var ce *errmsg.CustomError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, errmsg.ErrInternalSec.BusinessCode, ce.BusinessCode)
	})

	t.Run("userSrv error returns ErrInternalSec", func(t *testing.T) {
		service, repo, userSrv, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			if userID == 1 {
				return []uint64{2}, nil
			}
			return []uint64{2}, nil
		}

		userSrv.listUsersByIDsFunc = func(ctx context.Context, userIDs []uint64) ([]user.UserDTO, error) {
			return nil, fmt.Errorf("user service unavailable")
		}

		commonUsers, err := service.FollowCommon(ctx, 1, 2)
		require.Error(t, err)
		require.Nil(t, commonUsers)
		var ce *errmsg.CustomError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, errmsg.ErrInternalSec.BusinessCode, ce.BusinessCode)
	})
}

// =============================================================================
// followCacheKey
// =============================================================================

func Test_followCacheKey(t *testing.T) {
	t.Run("generates correct key and loadKey", func(t *testing.T) {
		key, loadKey := followCacheKey(1)
		require.Equal(t, "follows:1", key)
		require.Equal(t, "follows:1:loaded", loadKey)
	})

	t.Run("generates correct keys for different user IDs", func(t *testing.T) {
		key, loadKey := followCacheKey(9999)
		require.Equal(t, "follows:9999", key)
		require.Equal(t, "follows:9999:loaded", loadKey)
	})
}

// =============================================================================
// ensureCache
// =============================================================================

func TestService_ensureCache(t *testing.T) {
	t.Run("loads and caches followed IDs when cache is cold", func(t *testing.T) {
		service, repo, _, mr := setUpFollowService(t)
		ctx := context.Background()

		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			require.Equal(t, uint64(1), userID)
			return []uint64{10, 20, 30}, nil
		}

		err := service.ensureCache(ctx, 1)
		require.NoError(t, err)

		// Verify loadKey was set
		_, loadKey := followCacheKey(1)
		exists := mr.Exists(loadKey)
		require.True(t, exists)

		// Verify followed IDs were cached in the set
		key, _ := followCacheKey(1)
		members, err := mr.SMembers(key)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"10", "20", "30"}, members)
	})

	t.Run("skips loading when cache is already hot", func(t *testing.T) {
		service, repo, _, mr := setUpFollowService(t)
		ctx := context.Background()

		// Pre-warm the cache
		_, loadKey := followCacheKey(1)
		mr.Set(loadKey, "1")

		// Repo should NOT be called
		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			t.Fatalf("repo should not be called when cache is hot")
			return nil, nil
		}

		err := service.ensureCache(ctx, 1)
		require.NoError(t, err)
	})

	t.Run("returns error when repo fails", func(t *testing.T) {
		service, repo, _, _ := setUpFollowService(t)
		ctx := context.Background()

		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			return nil, fmt.Errorf("db error")
		}

		err := service.ensureCache(ctx, 1)
		require.Error(t, err)
		require.Contains(t, err.Error(), "db error")
	})

	t.Run("handles empty followed list", func(t *testing.T) {
		service, repo, _, mr := setUpFollowService(t)
		ctx := context.Background()

		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			return []uint64{}, nil
		}

		err := service.ensureCache(ctx, 1)
		require.NoError(t, err)

		// loadKey should be set even when there are no followed users
		_, loadKey := followCacheKey(1)
		exists := mr.Exists(loadKey)
		require.True(t, exists)
	})
}
