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
	u, err := s.repo.GetUserByPhone(ctx, req.Phone)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, &errmsg.ErrUserNotFound
	}

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

	return &LoginResp{
		Token:    tokenStr,
		NickName: u.NickName,
	}, nil
}

// CodeLogin 使用验证码登录，用户不存在时自动注册，用uuid生成token（替代jwt）
func (s *Service) CodeLogin(ctx context.Context, req *CodeLoginReq) (*LoginResp, error) {
	phone := req.Phone

	codeKey := BizUserLoginCode + phone
	storedCode, err := s.rdb.Get(ctx, codeKey).Result()
	if err == redis.Nil {
		return nil, &errmsg.ErrCodeExpired
	} else if err != nil {
		return nil, err
	}

	if storedCode != req.Code {
		return nil, &errmsg.ErrInvalidCode
	}

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

	lockKey := BizUserLockCode + phone
	success, err := s.rdb.SetNX(ctx, lockKey, "1", BizUserLockTTL).Result()
	if err != nil {
		return nil, err
	}
	if !success {
		return nil, &errmsg.ErrTooManyRequests
	}

	var code string
	for i := 0; i < 6; i++ {
		num, _ := rand.Int(rand.Reader, big.NewInt(10))
		code += num.String()
	}

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

	if req.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		user.Password = string(hashedPassword)
	}

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
