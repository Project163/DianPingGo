package blog

import (
	"context"
	"dianping/internal/module/user"
	"dianping/pkg/errmsg"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var likedLuaScript = redis.NewScript(`
local key = KEYS[1]
local member = ARGV[1]
local score = ARGV[2]

local rank = redis.call('ZRANK', key, member)

if not rank then
	redis.call('ZADD', key, score, member)
	return 1
else
	redis.call('ZREM', key, member)
	return 0
end
`)

type UserService interface {
	GetUserByID(ctx context.Context, userID uint64) (*user.UserDTO, error)
	ListUsersByIDs(ctx context.Context, userIDs []uint64) ([]user.UserDTO, error)
}

type FollowService interface {
	ListFollowedUserIDs(ctx context.Context, userID uint64) ([]uint64, error)
	ListFollowerUserIDs(ctx context.Context, userID uint64) ([]uint64, error)
}

type BlogRepository interface {
	CreateBlog(ctx context.Context, blog *Blog) error
	GetBlogByID(ctx context.Context, id uint64) (*Blog, error)
	ListHotBlogs(ctx context.Context, offset, limit int) ([]Blog, error)
	ListBlogsByUserID(ctx context.Context, userID uint64, offset, limit int) ([]Blog, error)
	IncrementLiked(ctx context.Context, blogID uint64) error
	DecrementLiked(ctx context.Context, blogID uint64) error
	ListBlogsByIDs(ctx context.Context, blogIDs []uint64) ([]Blog, error)
}

type Service struct {
	repo      BlogRepository
	rdb       redis.Cmdable
	userSrv   UserService
	followSrv FollowService
}

func NewService(repo BlogRepository, rdb redis.Cmdable, userSrv UserService, followSrv FollowService) *Service {
	return &Service{
		repo:      repo,
		rdb:       rdb,
		userSrv:   userSrv,
		followSrv: followSrv,
	}
}

// CreateBlog 创建博文，同时将博文推送到粉丝的feed中
func (s *Service) CreateBlog(ctx context.Context, blog *Blog) (uint64, error) {
	if err := s.repo.CreateBlog(ctx, blog); err != nil {
		return 0, err
	}

	if s.followSrv != nil {
		followerIDs, err := s.followSrv.ListFollowerUserIDs(ctx, blog.UserId)
		if err == nil && len(followerIDs) > 0 {
			s.pushFeed(ctx, followerIDs, blog.ID)
		}
	}
	return blog.ID, nil
}

// LikeBlog 点赞或取消点赞博文，使用Redis的ZSet来存储用户的点赞状态，保证高并发下的数据一致性
func (s *Service) LikeBlog(ctx context.Context, blogID uint64, userID uint64) error {
	key := BizBlogLikedKey + strconv.FormatUint(blogID, 10)
	uidStr := strconv.FormatUint(userID, 10)
	now := float64(time.Now().UnixMilli())

	// Lua保证了redis操作的原子性，Redis状态永远是正确的
	// 除非DB出错，一般是不会出现数据不一致问题的
	// TODO: 但还是最好用消息队列解决
	res, err := likedLuaScript.Run(ctx, s.rdb, []string{key}, uidStr, now).Int()
	if err != nil {
		return err
	}

	switch res {
	case 1:
		// 点赞成功，数据库点赞数+1
		return s.repo.IncrementLiked(ctx, blogID)
	case 0:
		// 取消点赞成功，数据库点赞数-1
		return s.repo.DecrementLiked(ctx, blogID)
	}
	return &errmsg.ErrInternalSec
	// 回滚有数据不一致的风险
	// ZRem本身就有失败的可能，如果ZRem失败的话，数据库执行成功，但redis执行失败
	// 导致数据库的点赞数就会比Redis的点赞集合多1，出现数据不一致
	// // ZAdd是原子操作，返回值addedCount表示是否新增了一个元素
	// // 如果是新增的元素则执行点赞数+1，否则执行点赞数-1
	// // 该策略将查和改合并为一个原子操作，避免了高并发下的点赞数不一致问题
	// addedCount, err := s.rdb.ZAdd(ctx, key, redis.Z{Score: now, Member: uidStr}).Result()
	// if err != nil {
	// 	return err
	// }
	// // 回滚有数据不一致的风险
	// // 具体体现在ZRem本身失败的话，数据库的点赞数就会比Redis的点赞集合多1，导致数据不一致
	// if addedCount > 0 {
	// 	if err := s.repo.IncrementLiked(ctx, blogID); err != nil {
	// 		s.rdb.ZRem(ctx, key, uidStr) // 回滚
	// 		return err
	// 	}
	// 	return nil
	// }
	// if err := s.repo.DecrementLiked(ctx, blogID); err != nil {
	// 	return err
	// }
	// return s.rdb.ZRem(ctx, key, uidStr).Err()

	// 先查再改的策略无法应对高并发的点赞/取消点赞操作，可能会出现数据不一致的情况
	// 例如用户疯狂按点赞，两次点赞操作ZScore检查时都返回nil，Redis由于其Set的特性会只保留一次（这是正确的）
	// 但是数据库会执行两次IncrementLiked增加两次点赞数，导致数据不一致
	// // 用ZScore检查blogID的点赞集合中是否存在userID
	// _, err := s.rdb.ZScore(ctx, key, strconv.FormatUint(userID, 10)).Result()
	// if err != nil && err != redis.Nil {
	// 	// 用户未点赞，执行点赞操作
	// 	return err
	// }
	// if err == redis.Nil {
	// 	// 用户未点赞，执行点赞操作
	// 	if err := s.repo.IncrementLiked(ctx, blogID); err != nil {
	// 		return err
	// 	}
	// 	// 将userID添加到blogID的点赞集合中，score为当前时间戳
	// 	return s.rdb.ZAdd(ctx, key, redis.Z{
	// 		Score:  float64(time.Now().UnixMilli()),
	// 		Member: strconv.FormatUint(userID, 10),
	// 	}).Err()
	// }
	// if err := s.repo.DecrementLiked(ctx, blogID); err != nil {
	// 	return err
	// }
	// // 将userID从blogID的点赞集合中移除
	// return s.rdb.ZRem(ctx, key, strconv.FormatUint(userID, 10)).Err()
}

// QueryHotBlog 查询热门博文列表，按点赞数降序排序，并填充作者信息和当前用户的点赞状态
func (s *Service) QueryHotBlog(ctx context.Context, currentUserID uint64, current int) ([]Blog, error) {
	offset := (current - 1) * MaxPageSize
	blogs, err := s.repo.ListHotBlogs(ctx, offset, MaxPageSize)
	if err != nil {
		return nil, err
	}
	s.populateBlogs(ctx, blogs, currentUserID)
	return blogs, nil
}

// QueryBlogByID 查询博文详情，填充作者信息和当前用户的点赞状态
func (s *Service) QueryBlogByID(ctx context.Context, blogID uint64, currentUserID uint64) (*Blog, error) {
	blog, err := s.repo.GetBlogByID(ctx, blogID)
	if err != nil {
		return nil, err
	}
	if blog == nil {
		return nil, &errmsg.ErrBlogNotFound
	}
	s.populateBlog(ctx, blog, currentUserID)
	return blog, nil
}

// QueryBlogLikesByID 查询博文的点赞用户列表，按点赞时间降序排序，返回前5个用户信息
func (s *Service) QueryBlogLikesByID(ctx context.Context, blogID uint64) ([]user.UserDTO, error) {
	key := BizBlogLikedKey + strconv.FormatUint(blogID, 10)
	// 获取点赞集合中score最高的前5个用户ID（顺序排列）
	userIDs, err := s.rdb.ZRevRange(ctx, key, 0, 4).Result()
	if err != nil {
		return nil, err
	}
	if len(userIDs) == 0 {
		return []user.UserDTO{}, nil
	}
	// 将用户ID从string转换为uint64
	ids := make([]uint64, len(userIDs))
	for i, uid := range userIDs {
		ids[i], err = strconv.ParseUint(uid, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	users, err := s.userSrv.ListUsersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	// 根据Redis中获取的用户ID顺序对users进行排序，保证返回的用户信息顺序与点赞顺序一致
	order := make(map[uint64]int, len(ids))
	for i, id := range ids {
		// Key是用户ID，Value是用户ID在Redis中出现的顺序索引
		order[id] = i
	}
	// 使用sort.Slice对users进行排序，排序依据是用户ID在Redis中出现的顺序索引
	sort.Slice(users, func(i, j int) bool {
		return order[users[i].ID] < order[users[j].ID]
	})
	return users, nil
}

// QueryBlogsByUserID 查询指定用户的博文列表，按更新时间降序排序，并填充作者信息和当前用户的点赞状态
func (s *Service) QueryBlogsByUserID(ctx context.Context, targetUserID uint64, currentUserID uint64, current int) ([]Blog, error) {
	offset := (current - 1) * MaxPageSize
	blogs, err := s.repo.ListBlogsByUserID(ctx, targetUserID, offset, MaxPageSize)
	if err != nil {
		return nil, err
	}
	s.populateBlogs(ctx, blogs, currentUserID)
	return blogs, nil
}

// QueryBlogsOfFollow 查询当前用户关注的博文列表，按发布时间降序排序，返回分页结果
func (s *Service) QueryBlogsOfFollow(ctx context.Context, currentUserID uint64, max int64, offset int64) (*ScrollResult, error) {
	key := BizBlogFeedKey + strconv.FormatUint(currentUserID, 10)
	// ZRevRangeByScoreWithScores返回的是一个包含元素和分数的切片
	// 分数是时间戳，元素是博文ID
	tuples, err := s.rdb.ZRevRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
		Min:    "0",
		Max:    strconv.FormatInt(max, 10),
		Offset: offset,
		Count:  MaxPageSize,
	}).Result()
	if err != nil {
		return nil, err
	}
	if len(tuples) == 0 {
		return &ScrollResult{
			List:    []Blog{},
			MinTime: 0,
			Offset:  0,
		}, nil
	}
	// 将Redis返回的博文ID从string转换为uint64，并计算最小时间戳和偏移量
	ids := make([]uint64, len(tuples))
	var minTime int64
	var os int64 = 1
	// TODO: offset有计算漏洞
	// 如果上一页结尾有2条时间戳为X的数据，这一页开头又有2条时间戳为X的数据
	// os会在这一页被错误地重置为1或仅从本页开始数，导致下一页查询时漏掉或重复读数据。
	for i, t := range tuples {
		ids[i], _ = strconv.ParseUint(t.Member.(string), 10, 64)
		ts := int64(t.Score)
		if ts == minTime {
			os++
		} else {
			minTime = ts
			os = 1
		}
	}
	// 根据Redis中获取的博文ID顺序查询数据库中的博文信息
	blogs, err := s.repo.ListBlogsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	// 并根据Redis中获取的顺序对博文进行排序，保证返回的博文顺序与发布时间顺序一致
	order := make(map[uint64]int, len(ids))
	for i, id := range ids {
		order[id] = i
	}
	sort.Slice(blogs, func(i, j int) bool {
		return order[blogs[i].ID] < order[blogs[j].ID]
	})
	s.populateBlogs(ctx, blogs, currentUserID)
	return &ScrollResult{
		List:    blogs,
		MinTime: minTime,
		Offset:  os,
	}, nil
}

// TODO:推模式在粉丝量大的情况下可能会有性能问题，一个巨大的PipeLine会榨干Go内存
// 可以考虑改为拉模式，或用分批用消息队列异步处理
// 但是消息队列还没学:)
// pushFeed 将新博文推送到粉丝的feed中
func (s *Service) pushFeed(ctx context.Context, followerIDs []uint64, blogID uint64) {
	pipe := s.rdb.Pipeline()
	now := float64(time.Now().UnixMilli())
	for _, fid := range followerIDs {
		pipe.ZAdd(ctx, BizBlogFeedKey+strconv.FormatUint(fid, 10), redis.Z{
			Score:  now,
			Member: blogID,
		})
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		log.Printf("pushFeed ZAdd error: %v", err)
	}
}

// populateBlog 填充单个博文的作者信息和当前用户的点赞状态
func (s *Service) populateBlog(ctx context.Context, blog *Blog, currentUserID uint64) {
	s.populateBlogUser(ctx, blog)
	s.checkIsLiked(ctx, []*Blog{blog}, currentUserID)
}

// populateBlogUser 填充博文的作者信息
func (s *Service) populateBlogUser(ctx context.Context, blog *Blog) {
	if s.userSrv == nil {
		return
	}
	user, err := s.userSrv.GetUserByID(ctx, blog.UserId)
	if err != nil || user == nil {
		return
	}
	blog.Icon = user.Icon
	blog.Name = user.NickName
}

// populateBlogs 填充多个博文的作者信息和当前用户的点赞状态
func (s *Service) populateBlogs(ctx context.Context, blogs []Blog, currentUserID uint64) {
	if s.userSrv != nil && len(blogs) > 0 {
		// 找到所有博文的作者ID，用map去重，避免重复查询用户信息
		userIDs := make(map[uint64]struct{}, len(blogs))
		for i := range blogs {
			userIDs[blogs[i].UserId] = struct{}{}
		}
		// 将map的key（去重后的作者ID）转换为slice
		ids := make([]uint64, 0, len(userIDs))
		for id := range userIDs {
			ids = append(ids, id)
		}
		// 一次查出所有作者信息，避免N+1查询问题
		users, err := s.userSrv.ListUsersByIDs(ctx, ids)
		if err == nil {
			// 将用户信息存入map，并根据作者ID为每篇博文填充作者信息
			userMap := make(map[uint64]*user.UserDTO, len(users))
			for i := range users {
				userMap[users[i].ID] = &users[i]
			}
			for i := range blogs {
				if user, ok := userMap[blogs[i].UserId]; ok {
					blogs[i].Icon = user.Icon
					blogs[i].Name = user.NickName
				}
			}
		}
	}
	blogPtrs := make([]*Blog, len(blogs))
	for i := range blogs {
		blogPtrs[i] = &blogs[i]
	}
	s.checkIsLiked(ctx, blogPtrs, currentUserID)
}

// checkIsLiked 检查当前用户是否点赞了指定的博文列表
// 使用Redis的ZScore命令查询用户ID是否存在于博文的点赞集合中
func (s *Service) checkIsLiked(ctx context.Context, blogs []*Blog, userID uint64) {
	if userID == 0 || len(blogs) == 0 {
		return
	}
	pipe := s.rdb.Pipeline()
	cmders := make([]*redis.FloatCmd, len(blogs))
	for i := range blogs {
		key := BizBlogLikedKey + strconv.FormatUint(blogs[i].ID, 10)
		cmders[i] = pipe.ZScore(ctx, key, strconv.FormatUint(userID, 10))
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		log.Printf("checkIsLiked Pipeline error: %v", err)
	}
	for i := range blogs {
		_, err := cmders[i].Result()
		if err != nil {
			if err != redis.Nil {
				log.Printf("checkIsLiked ZScore error: %v", err)
			}
			blogs[i].IsLike = false
			continue
		}
		blogs[i].IsLike = true
	}
}
