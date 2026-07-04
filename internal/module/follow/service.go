package follow

import (
	"context"
	"dianping/internal/module/user"
	"dianping/pkg/errmsg"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

var followCacheAddLuaScript = redis.NewScript(`
local key = KEYS[1]
local loadKey = KEYS[2]
local followUserID = ARGV[1]
local ttl = tonumber(ARGV[2])

if redis.call("EXISTS", loadKey) == 0 then
	return 0
end

redis.call("SADD", key, followUserID)
redis.call("EXPIRE", key, ARGV[2])
redis.call("EXPIRE", loadKey, ARGV[2])
return 1
`)

var followCacheRemoveScript = redis.NewScript(`
local key = KEYS[1]
local loadKey = KEYS[2]
local followUserID = ARGV[1]
local ttl = tonumber(ARGV[2])

if redis.call("EXISTS", loadKey) == 0 then
	return 0
end

redis.call("SREM", key, followUserID)
if redis.call("EXISTS", key) == 0 then
	redis.call("EXPIRE", key, ARGV[2])
end
redis.call("EXPIRE", loadKey, ARGV[2])
return 1
`)

type UserService interface {
	ListUsersByIDs(ctx context.Context, userIDs []uint64) ([]user.UserDTO, error)
}

type FollowRepository interface {
	Follow(ctx context.Context, userID, followUserID uint64) (bool, error)
	Unfollow(ctx context.Context, userID, followUserID uint64) (bool, error)
	IsFollowed(ctx context.Context, userID, followUserID uint64) (bool, error)
	ListFollowerUserIDs(ctx context.Context, userID uint64) ([]uint64, error)
	ListFollowedUserIDs(ctx context.Context, userID uint64) ([]uint64, error)
}

type Service struct {
	repo    FollowRepository
	userSrv UserService
	rdb     redis.Cmdable
}

func NewService(repo FollowRepository, userSrv UserService, rdb redis.Cmdable) *Service {
	return &Service{
		repo:    repo,
		userSrv: userSrv,
		rdb:     rdb,
	}
}

// Follow 关注用户
func (s *Service) Follow(ctx context.Context, follow *Follow) error {
	if follow.UserID == follow.FollowUserID {
		return errmsg.NewError(errmsg.ErrFollowYourself, fmt.Errorf("cannot follow yourself"))
	}

	_, err := s.repo.Follow(ctx, follow.UserID, follow.FollowUserID)
	if err != nil {
		return errmsg.NewError(errmsg.ErrInternalSec, err)
	}

	key, loadKey := followCacheKey(follow.UserID)
	ttl := int64(BizFollowerTTL.Seconds())
	_, err = followCacheAddLuaScript.Run(ctx, s.rdb, []string{key, loadKey}, strconv.FormatUint(follow.FollowUserID, 10), ttl).Result()
	if err != nil {
		if err = s.rdb.Del(ctx, key, loadKey).Err(); err != nil {
			return errmsg.NewError(errmsg.ErrInternalSec, err)
		}
		return err
	}
	// // 更新缓存
	// // 缓存冷（loadedKey不存在）时，直接删除缓存，不因为写操作而重建
	// // 缓存热（loadedKey存在）时，直接更新缓存，避免下一次查询时再去数据库查询
	// loaded, err := s.rdb.Exists(ctx, loadKey).Result()
	// if err != nil {
	// 	_ = s.rdb.Del(ctx, key, loadKey).Err()
	// 	return nil
	// }
	// if loaded == 0 {
	// 	return nil
	// }
	// pipe := s.rdb.TxPipeline()
	// pipe.SAdd(ctx, key, strconv.FormatUint(follow.FollowUserID, 10))
	// pipe.Expire(ctx, key, BizFollowerTTL)
	// _, err = pipe.Exec(ctx)
	// if err != nil {
	// 	_ = s.rdb.Del(ctx, key, loadKey).Err()
	// 	return nil
	// }
	// // 更新缓存
	// // 直接删除缓存，下一次查询时会重新加载
	// // 关注本身并不查询缓存，因此直接删除缓存等待真正的查询时进行重建即可
	// err = s.rdb.Del(ctx, key, loadKey).Err()
	// if err != nil {
	// 	return errmsg.NewError(errmsg.ErrInternalSec, err)
	// }
	return nil
}

// Unfollow 取消关注用户
func (s *Service) Unfollow(ctx context.Context, follow *Follow) error {
	if follow.UserID == follow.FollowUserID {
		return errmsg.NewError(errmsg.ErrFollowYourself, fmt.Errorf("cannot unfollow yourself"))
	}

	_, err := s.repo.Unfollow(ctx, follow.UserID, follow.FollowUserID)
	if err != nil {
		return errmsg.NewError(errmsg.ErrInternalSec, err)
	}

	key, loadKey := followCacheKey(follow.UserID)
	ttl := int64(BizFollowerTTL.Seconds())
	_, err = followCacheRemoveScript.Run(ctx, s.rdb, []string{key, loadKey}, strconv.FormatUint(follow.FollowUserID, 10), ttl).Result()
	if err != nil {
		if err = s.rdb.Del(ctx, key, loadKey).Err(); err != nil {
			return errmsg.NewError(errmsg.ErrInternalSec, err)
		}
		return err
	}
	// loaded, err := s.rdb.Exists(ctx, loadKey).Result()
	// if err != nil {
	// 	_ = s.rdb.Del(ctx, key, loadKey).Err()
	// 	return nil
	// }
	// if loaded == 0 {
	// 	return nil
	// }
	// pipe := s.rdb.TxPipeline()
	// pipe.SRem(ctx, key, strconv.FormatUint(follow.FollowUserID, 10))
	// // 如果Set删除了最后一个元素，Redis会自动删除该Set
	// // 但是loadKey会保留，以表示已加载但为空的状态
	// pipe.Expire(ctx, key, BizFollowerTTL)
	// pipe.Expire(ctx, loadKey, BizFollowerTTL)
	// _, err = pipe.Exec(ctx)
	// if err != nil {
	// 	_ = s.rdb.Del(ctx, key, loadKey).Err()
	// }
	// // 更新缓存
	// // 直接删除缓存，下一次查询时会重新加载
	// // 关注本身并不查询缓存，因此直接删除缓存等待真正的查询时进行重建即可
	// err = s.rdb.Del(ctx, key, strconv.FormatUint(follow.FollowUserID, 10)).Err()
	// if err != nil {
	// 	return errmsg.NewError(errmsg.ErrInternalSec, err)
	// }
	return nil
}

// IsFollowed 检查用户是否关注了指定用户
func (s *Service) IsFollowed(ctx context.Context, userID, followUserID uint64) (bool, error) {
	isFollowed, err := s.repo.IsFollowed(ctx, userID, followUserID)
	if err != nil {
		return false, errmsg.NewError(errmsg.ErrInternalSec, err)
	}
	return isFollowed, nil
}

// ListFollowerUserIDs 获取指定用户的所有粉丝用户ID
func (s *Service) ListFollowerUserIDs(ctx context.Context, userID uint64) ([]uint64, error) {
	followerIDs, err := s.repo.ListFollowerUserIDs(ctx, userID)
	if err != nil {
		return nil, errmsg.NewError(errmsg.ErrInternalSec, err)
	}
	return followerIDs, nil
}

// ListFollowedUserIDs 获取指定用户的所有关注用户ID
func (s *Service) ListFollowedUserIDs(ctx context.Context, userID uint64) ([]uint64, error) {
	followedIDs, err := s.repo.ListFollowedUserIDs(ctx, userID)
	if err != nil {
		return nil, errmsg.NewError(errmsg.ErrInternalSec, err)
	}
	return followedIDs, nil
}

// FollowCommon 获取两个用户的共同关注用户列表
func (s *Service) FollowCommon(ctx context.Context, userID1, userID2 uint64) ([]user.UserDTO, error) {
	key1, _ := followCacheKey(userID1)
	key2, _ := followCacheKey(userID2)

	if err := s.ensureCache(ctx, userID1); err != nil {
		return nil, errmsg.NewError(errmsg.ErrInternalSec, err)
	}

	if err := s.ensureCache(ctx, userID2); err != nil {
		return nil, errmsg.NewError(errmsg.ErrInternalSec, err)
	}

	commonIDs, err := s.rdb.SInter(ctx, key1, key2).Result()
	if err != nil {
		return nil, errmsg.NewError(errmsg.ErrInternalSec, err)
	}

	if len(commonIDs) == 0 {
		return []user.UserDTO{}, nil
	}

	commonUserIDs := make([]uint64, len(commonIDs))
	for i, idStr := range commonIDs {
		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			return nil, errmsg.NewError(errmsg.ErrInternalSec, err)
		}
		commonUserIDs[i] = id
	}

	commonUsers, err := s.userSrv.ListUsersByIDs(ctx, commonUserIDs)
	if err != nil {
		return nil, errmsg.NewError(errmsg.ErrInternalSec, err)
	}
	return commonUsers, nil
}

// ensureCache 确保用户关注缓存已加载
func (s *Service) ensureCache(ctx context.Context, userID uint64) error {
	key, loadKey := followCacheKey(userID)
	exists, err := s.rdb.Exists(ctx, loadKey).Result()
	if err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}

	ids, err := s.repo.ListFollowedUserIDs(ctx, userID)
	if err != nil {
		return err
	}

	IDStrs := make([]interface{}, len(ids))
	for i, id := range ids {
		IDStrs[i] = strconv.FormatUint(id, 10)
	}

	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, key)
	pipe.Set(ctx, loadKey, "1", BizFollowerTTL)

	if len(IDStrs) > 0 {
		pipe.SAdd(ctx, key, IDStrs...)
		pipe.Expire(ctx, key, BizFollowerTTL)
	}

	_, err = pipe.Exec(ctx)
	return err
}

// followCacheKey 生成关注缓存的key和loadKey
func followCacheKey(userID uint64) (string, string) {
	key := BizFollowKey + strconv.FormatUint(userID, 10)
	loadKey := key + BizFollowedSuffix
	return key, loadKey
}
