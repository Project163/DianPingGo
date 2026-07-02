package middleware

import (
	"dianping/pkg/errmsg"
	"dianping/pkg/response"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type AuthConfig struct {
	TokenPrefix string
	TokenTTL    time.Duration
}

func DefaultAuthConfig() *AuthConfig {
	return &AuthConfig{
		TokenPrefix: "login:token:",
		TokenTTL:    30 * time.Minute,
	}
}

type AuthOptions func(*AuthConfig)

func WithTokenPrefix(prefix string) AuthOptions {
	return func(cfg *AuthConfig) {
		cfg.TokenPrefix = prefix
	}
}

func WithTokenTTL(ttl time.Duration) AuthOptions {
	return func(cfg *AuthConfig) {
		cfg.TokenTTL = ttl
	}
}

// CtxUserIDKey 是上下文中存储用户ID的键，常量避免魔法字符串
const CtxUserIDKey = "userId"

// AuthMiddleware 验证用户登录状态，使用token从Redis获取用户信息，并将用户ID存储在上下文中
func AuthMiddleware(rdb redis.Cmdable, opts ...AuthOptions) gin.HandlerFunc {
	cfg := DefaultAuthConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	return func(ctx *gin.Context) {
		// 从请求头获取token
		token := ctx.GetHeader("Authorization")
		if token == "" {
			response.Fail(ctx, &errmsg.ErrUnauthorized)
			ctx.Abort()
			return
		}

		// 构造Redis键
		tokenKey := cfg.TokenPrefix + token
		result, err := rdb.HGetAll(ctx.Request.Context(), tokenKey).Result()
		if err != nil {
			response.Fail(ctx, &errmsg.ErrUnauthorized)
			ctx.Abort()
			return
		}

		// HGetAll返回的结果是一个map[string]string，包含用户信息
		idStr, ok := result["id"]
		if !ok {
			response.Fail(ctx, &errmsg.ErrUnauthorized)
			ctx.Abort()
			return
		}

		userID, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			response.Fail(ctx, &errmsg.ErrUnauthorized)
			ctx.Abort()
			return
		}
		// 刷新Token过期时间，保持用户在线状态
		rdb.Expire(ctx.Request.Context(), tokenKey, cfg.TokenTTL)

		// 将用户ID存储在上下文中，供后续处理函数使用
		ctx.Set(CtxUserIDKey, userID)
		ctx.Next()
	}
}
