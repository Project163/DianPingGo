package response

import (
	"dianping/pkg/errmsg"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 定义一个通用的API响应结构体，包含成功标志、业务状态码、消息、数据和TraceID等字段
type Response struct {
	Success      bool        `json:"success"`
	BusinessCode int         `json:"code"`
	Message      string      `json:"message"`
	Data         interface{} `json:"data,omitempty"`
	TraceID      string      `json:"traceId,omitempty"`
}

// OK 返回一个成功的响应
func OK(ctx *gin.Context, data interface{}) {
	ctx.JSON(http.StatusOK, Response{
		Success:      true,
		BusinessCode: 2000,
		Message:      "success",
		Data:         data,
		TraceID:      GetTraceID(ctx),
	})
}

// Fail 返回一个失败的响应，根据错误类型区分业务错误和系统错误
func Fail(ctx *gin.Context, err error) {
	var customErr *errmsg.CustomError

	// 如果错误是业务错误，返回对应的业务错误响应
	if errors.As(err, &customErr) {
		if customErr.Err != nil {
			fmt.Printf("business error: %v\n", customErr)
		}
		ctx.JSON(customErr.HttpCode, Response{
			Success:      false,
			BusinessCode: customErr.BusinessCode,
			Message:      customErr.Message,
			TraceID:      GetTraceID(ctx),
		})
		return
	}

	// 对于未知错误，返回一个通用的系统错误响应，并记录日志
	ctx.JSON(http.StatusInternalServerError, Response{
		Success:      false,
		BusinessCode: errmsg.ErrInternalSec.BusinessCode,
		Message:      errmsg.ErrInternalSec.Message,
		TraceID:      GetTraceID(ctx),
	})
}

// GetTraceID 从Gin上下文中获取TraceID，如果不存在则返回空字符串
func GetTraceID(ctx *gin.Context) string {
	traceID, exists := ctx.Get("traceId")
	if !exists {
		return ""
	}
	if str, ok := traceID.(string); ok {
		return str
	}
	return ""
}
