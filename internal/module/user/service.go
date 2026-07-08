package user

import (
	"context"
	"crypto/rand"
	"dianping/internal/cache"
	"dianping/pkg/errmsg"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

// UserRepository 定义了用户仓库接口，包含创建用户和根据手机号查询用户的方法（便于测试）
type UserRepository interface {
	CreateUser(ctx context.Context, user *User) error
	GetUserByPhone(ctx context.Context, phone string) (*User, error)
	GetUserByID(ctx context.Context, userID uint64) (*User, error)
	ListUsersByIDs(ctx context.Context, userIDs []uint64) ([]User, error)
}

// Service 定义了用户服务接口，包含登录、验证码登录、发送验证码和注册新用户的方法
type Service struct {
	repo UserRepository
	// jwtSecret []byte
	cacheClient *cache.CacheClient
	rdb         redis.Cmdable
}

func NewService(repo UserRepository, rdb redis.Cmdable, pool *cache.RefreshPool) *Service {
	return &Service{
		repo: repo,
		// jwtSecret: []byte(secret),
		cacheClient: cache.NewCacheClient(rdb, pool),
		rdb:         rdb,
	}
}

// Login 使用密码登录，返回token和昵称
func (s *Service) Login(ctx context.Context, req *LoginReq) (*LoginResp, error) {
	phone := req.Phone
	// 根据手机号查询用户
	u, err := s.repo.GetUserByPhone(ctx, phone)
	if err != nil {
		return nil, err
	}
	// 用户不存在（密码登陆暂时没写注册）
	if u == nil {
		return nil, &errmsg.ErrUserNotFound
	}

	// TODO: 多次密码错误限制，防止暴力破解
	// 密码学比较用户输入的密码和数据库中存储的哈希密码
	err = bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(req.Password))
	if err != nil {
		return nil, &errmsg.ErrInvalidPassword
	}

	// 为什么放弃了jwt？
	// 因为jwt的token是可以被伪造的，虽然可以通过签名来验证，但是如果密钥泄露了，就会有安全问题。
	// 并且如果一个jwt token被盗了，由于jwt是无状态的，无法在服务端直接注销这个token，只能继续使用，直到过期。
	// 使用uuid生成的token，可以存储在Redis中，设置过期时间，这样就可以实现单点登录，而且可以随时注销用户的登录状态。
	// uuid无非是无状态的，即每次服务都会查一次缓存，但对于目前的体量仍然是可以接受的，且可以随时注销用户的登录状态。
	// claims := jwt.MapClaims{
	// 	"userId": u.ID,
	// 	"exp":    time.Now().Add(24 * time.Hour).Unix(),
	// }
	// tokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// tokenStr, err := tokenObj.SignedString(s.jwtSecret)
	// if err != nil {
	// 	return nil, err
	// }

	// 使用uuid生成token，替代jwt
	tokenStr := strings.ReplaceAll(uuid.New().String(), "-", "")
	// 将用户信息存储在Redis中，key为BizUserToken + tokenStr，value为用户ID、昵称和头像URL等信息
	userDTO := &UserDTO{
		ID:       u.ID,
		NickName: u.NickName,
		Icon:     u.Icon,
	}
	IDStr := strconv.FormatUint(userDTO.ID, 10)
	// HSet支持一次设置多个字段，构造一个map[string]interface{}来存储用户信息
	userMap := map[string]interface{}{
		"id":        IDStr,
		"nick_name": userDTO.NickName,
		"icon":      userDTO.Icon,
	}
	// 使用管道一次执行HSet和Expire
	tokenKey := BizUserToken + tokenStr
	pipe := s.rdb.TxPipeline()

	pipe.HSet(ctx, tokenKey, userMap)
	pipe.Expire(ctx, tokenKey, BizUserTokenTTL)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}

	return &LoginResp{
		Token:    tokenStr,
		NickName: u.NickName,
	}, nil
}

// CodeLogin 使用验证码登录，用户不存在时自动注册，用uuid生成token（替代jwt）
func (s *Service) CodeLogin(ctx context.Context, req *CodeLoginReq) (*LoginResp, error) {
	phone := req.Phone
	// 从Redis中获取验证码，key为BizUserLoginCode + phone
	codeKey := BizUserLoginCode + phone
	storedCode, err := s.rdb.Get(ctx, codeKey).Result()
	if err == redis.Nil {
		return nil, &errmsg.ErrCodeExpired
	} else if err != nil {
		return nil, err
	}
	// 验证码直接明文比较就行吧感觉
	if storedCode != req.Code {
		return nil, &errmsg.ErrInvalidCode
	}
	// 根据手机号查询用户，如果用户不存在则自动注册
	u, err := s.repo.GetUserByPhone(ctx, phone)
	if err != nil {
		return nil, err
	}
	if u == nil {
		u, err = s.Register(ctx, &CreateUserReq{
			Phone:    phone,
			Password: "",
			NickName: "",
		})
		if err != nil {
			return nil, err
		}
	}

	// claims := jwt.MapClaims{
	// 	"userId": u.ID,
	// 	"exp":    time.Now().Add(24 * time.Hour).Unix(),
	// }
	// tokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// tokenStr, err := tokenObj.SignedString(s.jwtSecret)
	// if err != nil {
	// 	return nil, err
	// }

	// 以下同上
	tokenStr := strings.ReplaceAll(uuid.New().String(), "-", "")

	userDTO := &UserDTO{
		ID:       u.ID,
		NickName: u.NickName,
		Icon:     u.Icon,
	}

	IDStr := strconv.FormatUint(userDTO.ID, 10)

	userMap := map[string]interface{}{
		"id":        IDStr,
		"nick_name": userDTO.NickName,
		"icon":      userDTO.Icon,
	}

	tokenKey := BizUserToken + tokenStr
	pipe := s.rdb.TxPipeline()

	pipe.HSet(ctx, tokenKey, userMap)
	pipe.Expire(ctx, tokenKey, BizUserTokenTTL)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}

	s.rdb.Del(ctx, codeKey)
	return &LoginResp{
		Token:    tokenStr,
		NickName: u.NickName,
	}, nil
}

// Logout 注销用户登录状态，删除Redis中的token
func (s *Service) Logout(ctx context.Context, userId uint64, token string) error {
	tokenKey := BizUserToken + token
	result, err := s.rdb.HGet(ctx, tokenKey, "id").Result()
	if err == redis.Nil {
		return nil
	} else if err != nil {
		return err
	}

	if result != strconv.FormatUint(userId, 10) {
		return &errmsg.ErrUnauthorized
	}

	return s.rdb.Del(ctx, tokenKey).Err()
}

// SendCode 发送验证码
func (s *Service) SendCode(ctx context.Context, req *SendCodeReq) (*SendCodeResp, error) {
	phone := req.Phone
	// 分布式锁来限制同一手机号在短时间内重复发送验证码
	// 锁的key为BizUserLockCode + phone，过期时间为1分钟
	lockKey := BizUserLockCode + phone
	success, err := s.rdb.SetNX(ctx, lockKey, "1", BizUserLockTTL).Result()
	if err != nil {
		return nil, err
	}
	if !success {
		return nil, &errmsg.ErrTooManyRequests
	}

	// 生成6位随机验证码，使用crypto/rand包来生成安全的随机数
	var code string
	for i := 0; i < 6; i++ {
		num, _ := rand.Int(rand.Reader, big.NewInt(10))
		code += num.String()
	}

	// 将验证码存储在Redis中，key为BizUserLoginCode + phone，value为验证码，过期时间为5分钟
	codeKey := BizUserLoginCode + phone
	err = s.rdb.Set(ctx, codeKey, code, BizUserLoginCodeTTL).Err()
	if err != nil {
		s.rdb.Del(ctx, lockKey)
		return nil, err
	}

	fmt.Printf("发送验证码 %s 到手机号 %s\n", code, phone)
	return &SendCodeResp{
		Message: "发送成功",
	}, nil
}

// Register 创建新用户
func (s *Service) Register(ctx context.Context, req *CreateUserReq) (*User, error) {
	var nick_name string
	// 如果请求中没有提供昵称，则生成一个随机昵称，格式为"user_"加上5个随机字节的十六进制字符串，确保昵称唯一且不易被猜测
	if req.NickName == "" {
		bytes := make([]byte, 5)
		if _, err := rand.Read(bytes); err != nil {
			nick_name = fmt.Sprintf("user_%d", time.Now().Unix())
		} else {
			nick_name = "user_" + hex.EncodeToString(bytes)
		}
	} else {
		nick_name = req.NickName
	}

	user := &User{
		Phone:    req.Phone,
		NickName: nick_name,
	}

	// 如果请求中提供了密码，则对密码进行哈希处理后存储在数据库中，使用bcrypt算法来生成安全的哈希密码
	if req.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		user.Password = string(hashedPassword)
	}

	// 尝试创建用户，如果发生唯一键冲突（错误代码1062），则查询现有用户并返回，避免重复注册
	err := s.repo.CreateUser(ctx, user)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			existingUser, errGet := s.repo.GetUserByPhone(ctx, req.Phone)
			if errGet != nil || existingUser == nil {
				return nil, &errmsg.ErrUserAlreadyExists
			}
			return existingUser, nil
		}
		return nil, err
	}

	return user, nil
}

// GetUserByID 根据用户ID查询用户信息，返回UserDTO
func (s *Service) GetUserByID(ctx context.Context, userID uint64) (*UserDTO, error) {
	key := CacheUserKey + strconv.FormatUint(userID, 10)
	var user User
	err := s.cacheClient.QueryWithPassThrough(ctx, key, &user,
		CacheUserTTL, CacheNullTTL, func() (any, error) {
			return s.repo.GetUserByID(ctx, userID)
		})
	if err != nil {
		if errors.Is(err, cache.ErrDataNotFound) {
			return nil, &errmsg.ErrUserNotFound
		}
		return nil, err
	}
	return &UserDTO{ID: user.ID, NickName: user.NickName, Icon: user.Icon}, nil
}

// ListUsersByIDs 根据用户ID列表批量查询用户信息，返回UserDTO列表
// TODO: 尚未实现列表查询的缓存优化，后续可以考虑使用Redis的MGET或管道操作来批量获取用户信息
func (s *Service) ListUsersByIDs(ctx context.Context, userIDs []uint64) ([]UserDTO, error) {
	users, err := s.repo.ListUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	userDTOs := toUserDTOs(users)
	return userDTOs, nil
}

func (s *Service) Sign(ctx context.Context, userID uint64) error {
	now := time.Now()
	yyyyMM := now.Format("2006:01:")
	key := BizUserSignKey + yyyyMM + strconv.FormatUint(userID, 10)
	dayOfMonth := now.Day() - 1 // BitMap的偏移量从0开始，所以要减1
	_, err := s.rdb.SetBit(ctx, key, int64(dayOfMonth), 1).Result()
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) SignCount(ctx context.Context, userID uint64) (int, error) {
	yyyyMM := time.Now().Format("2006:01:")
	key := BizUserSignKey + yyyyMM + strconv.FormatUint(userID, 10)
	dayOfMonth := time.Now().Day() - 1 // BitMap的偏移量从0开始，所以要减1
	result := make([]int64, dayOfMonth+1)
	result, err := s.rdb.BitField(ctx, key, "GET", fmt.Sprintf("u%d", dayOfMonth+1), 0).Result()
	if err != nil {
		return 0, err
	}
	if len(result) == 0 {
		return 0, nil
	}
	num := result[0]
	var count int = 0
	for n := num; n&1 == 1; n >>= 1 {
		count++
	}
	return count, nil
}

// toUserDTOs 将User列表转换为UserDTO列表
func toUserDTOs(users []User) []UserDTO {
	userDTOs := make([]UserDTO, len(users))
	for i, user := range users {
		userDTOs[i] = UserDTO{
			ID:       user.ID,
			NickName: user.NickName,
			Icon:     user.Icon,
		}
	}
	return userDTOs
}
