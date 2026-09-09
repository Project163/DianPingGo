package middleware

import (
	"context"
	"dianping/internal/session"
	"dianping/pkg/errmsg"
	"dianping/pkg/requestctx"
	"dianping/pkg/response"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type SessionAuthenticator interface {
	Authenticate(context.Context, string) (session.Result, error)
}

type AuthObserver func(context.Context, session.Result, error, time.Duration)

// CtxUserIDKey 是上下文中存储用户ID的键，常量避免魔法字符串
const CtxUserIDKey = "userId"

// AuthMiddleware 验证用户登录状态，使用token从Redis获取用户信息，并将用户ID存储在上下文中
func AuthMiddleware(authenticator SessionAuthenticator, observe AuthObserver) gin.HandlerFunc {
	if observe == nil {
		observe = logAuthFailure
	}
	return func(ctx *gin.Context) {
		requestCtx := ctx.Request.Context()
		started := time.Now()
		var result session.Result
		var authErr error
		defer func() { observe(requestCtx, result, authErr, time.Since(started)) }()

		if err := requestCtx.Err(); err != nil {
			result.Outcome, authErr = "request_stopped", err
			stopAuth(ctx, err)
			return
		}
		token, ok := parseToken(ctx.Request.Header)
		if !ok {
			result = session.Result{Outcome: "invalid_token", Reason: "invalid_header"}
			authErr = &errmsg.ErrUnauthorized
			stopAuth(ctx, authErr)
			return
		}
		result, authErr = authenticator.Authenticate(requestCtx, token)
		if authErr != nil {
			stopAuth(ctx, authErr)
			return
		}
		if err := requestCtx.Err(); err != nil {
			result.Outcome, authErr = "request_stopped", err
			stopAuth(ctx, err)
			return
		}
		// 只统计鉴权耗时，不能把后续 Handler 耗时算进去。
		observe(requestCtx, result, nil, time.Since(started))
		observe = func(context.Context, session.Result, error, time.Duration) {}
		ctx.Set(CtxUserIDKey, result.UserID)
		ctx.Next()
	}
}

// 拒绝请求中的错误Authorization
func parseToken(header http.Header) (string, bool) {
	values := header.Values("Authorization")
	if len(values) != 1 || len(values[0]) != 32 {
		return "", false
	}
	token := values[0]
	for _, c := range token {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return "", false
		}
	}
	return token, true
}

func stopAuth(ctx *gin.Context, err error) {
	ctx.Abort()
	if errors.Is(err, context.Canceled) {
		return // 对端已取消，不再尝试写 JSON。
	}
	if errors.Is(err, context.DeadlineExceeded) {
		// Store 自己的超时已经包装成 503；仅父请求超时直接到这里。
		var custom *errmsg.CustomError
		if !errors.As(err, &custom) {
			response.Fail(ctx, &errmsg.ErrRequestTimeout)
			return
		}
	}
	var custom *errmsg.CustomError
	if errors.As(err, &custom) {
		// 原始错误交给 observer；避免 response.Fail 再打印未脱敏依赖错误。
		publicErr := *custom
		publicErr.Err = nil
		response.Fail(ctx, &publicErr)
		return
	}
	response.Fail(ctx, &errmsg.ErrInternalSec)
}

func logAuthFailure(ctx context.Context, result session.Result, err error, elapsed time.Duration) {
	if err == nil && result.MaintenanceErr == nil {
		return
	}
	if result.Outcome == "missing" || result.Outcome == "invalid_token" || result.Outcome == "request_stopped" {
		return
	}
	slog.WarnContext(ctx, "authentication issue", "trace_id", requestctx.TraceID(ctx),
		"outcome", result.Outcome, "reason", result.Reason,
		"maintenance_failed", result.MaintenanceErr != nil, "duration", elapsed)
}
