package middleware

import (
	"dianping/internal/module/user"
	"dianping/pkg/errmsg"
	"dianping/pkg/response"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// CtxUserIDKey 是上下文中存储用户ID的键，常量避免魔法字符串
const CtxUserIDKey = "userId"

// AuthMiddleware 验证用户登录状态，使用token从Redis获取用户信息，并将用户ID存储在上下文中
func AuthMiddleware(rdb redis.Cmdable) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// 从请求头获取token
		token := ctx.GetHeader("Authorization")
		if token == "" {
			response.Fail(ctx, &errmsg.ErrUnauthorized)
			ctx.Abort()
			return
		}

		// 复用user模块的BizUserToken前缀，构造Redis键
		tokenKey := user.BizUserToken + token
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
		rdb.Expire(ctx.Request.Context(), tokenKey, user.BizUserTokenTTL)

		// 将用户ID存储在上下文中，供后续处理函数使用
		ctx.Set(CtxUserIDKey, userID)
		ctx.Next()
	}
}
