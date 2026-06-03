package user

import (
	"context"
	"crypto/rand"
	"dianping/internal/infra"
	"dianping/pkg/errmsg"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo      *Repository
	jwtSecret []byte
	rdb       *redis.Client
}

func NewService(repo *Repository, secret string) *Service {
	return &Service{
		repo:      repo,
		jwtSecret: []byte(secret),
		rdb:       infra.RedisClient,
	}
}

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

	// 密码学比较用户输入的密码和数据库中存储的哈希密码
	err = bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(req.Password))
	if err != nil {
		return nil, &errmsg.ErrInvalidPassword
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

	// 使用uuid生成token，替代jwt
	tokenStr := strings.ReplaceAll(uuid.New().String(), "-", "")
	// 将用户信息存储在Redis中，key为BizUserToken + tokenStr，value为用户ID、昵称和头像URL等信息，过期时间为24小时
	userDTO := &UserDTO{
		ID:       u.ID,
		NickName: u.NickName,
		Icon:     u.Icon,
	}
	// HSet支持一次设置多个字段，构造一个map[string]interface{}来存储用户信息
	userMap := map[string]interface{}{
		"id":       userDTO.ID,
		"nickname": userDTO.NickName,
		"icon":     userDTO.Icon,
	}
	// 使用事务管道一次执行HSet和Expire
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

	userMap := map[string]interface{}{
		"id":       userDTO.ID,
		"nickname": userDTO.NickName,
		"icon":     userDTO.Icon,
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

// SendCode 发送验证码
func (s *Service) SendCode(ctx context.Context, req *SendCodeReq) (*SendCodeResp, error) {
	phone := req.Phone
	// 通过设置分布式锁来限制同一手机号在短时间内重复发送验证码，锁的key为BizUserLockCode + phone，过期时间为1分钟
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
	var nickname string
	// 如果请求中没有提供昵称，则生成一个随机昵称，格式为"user_"加上5个随机字节的十六进制字符串，确保昵称唯一且不易被猜测
	if req.NickName == "" {
		bytes := make([]byte, 5)
		if _, err := rand.Read(bytes); err != nil {
			nickname = fmt.Sprintf("user_%d", time.Now().Unix())
		}
		nickname = "user_" + hex.EncodeToString(bytes)
	} else {
		nickname = req.NickName
	}

	user := &User{
		Phone:    req.Phone,
		NickName: nickname,
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
