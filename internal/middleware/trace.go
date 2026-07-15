package middleware

import (
	"crypto/rand"
	"dianping/pkg/errmsg"
	"dianping/pkg/requestctx"
	"dianping/pkg/response"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

const TraceIDHeader = "X-Trace-ID"

func TraceIDMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		traceID := ctx.GetHeader(TraceIDHeader)
		if !validTraceID(traceID) {
			var err error
			traceID, err = newTraceID()
			if err != nil {
				response.Fail(ctx, &errmsg.ErrInternalSec)
				ctx.Abort()
				return
			}
		}
		ctx.Set(requestctx.GinTraceIDKey, traceID)
		requestCtx := requestctx.WithTraceID(ctx.Request.Context(), traceID)
		ctx.Request = ctx.Request.WithContext(requestCtx)
		ctx.Header(TraceIDHeader, traceID)
		ctx.Next()
	}
}

func validTraceID(traceID string) bool {
	if len(traceID) != 32 {
		return false
	}
	decoded, err := hex.DecodeString(traceID)
	if err != nil {
		return false
	}
	// 全 0 ID 在 W3C Trace Context 中属于无效 ID。
	for _, b := range decoded {
		if b != 0 {
			return true
		}
	}
	return false
}

func newTraceID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
