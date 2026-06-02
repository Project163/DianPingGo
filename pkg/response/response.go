package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Result 定义一个通用的API响应结构体，包含成功标志、错误信息、数据和总数
type Result struct {
	Success  bool        `json:"success"`
	ErrorMsg string      `json:"error_msg,omitempty"`
	Data     interface{} `json:"data,omitempty"`
	Total    int64       `json:"total,omitempty"`
}

// OK 返回一个成功的响应，包含数据，对应Java中的Result.ok(Object data)
func OK(ctx *gin.Context, data interface{}) {
	ctx.JSON(http.StatusOK, Result{
		Success: true,
		Data:    data,
	})
}

// OKWithoutData 返回一个成功的响应，不包含数据，对应Java中的Result.ok()
func OKWithoutData(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, Result{
		Success: true,
	})
}

// OKWithTotal 返回一个成功的响应，包含数据和总数，对应Java中的Result.ok(List<?> data, long total)
func OKWithTotal(ctx *gin.Context, data interface{}, total int64) {
	ctx.JSON(http.StatusOK, Result{
		Success: true,
		Data:    data,
		Total:   total,
	})
}

// Fail 返回一个失败的响应，包含错误信息，对应Java中的Result.fail(String errorMsg)
func Fail(ctx *gin.Context, errorMsg string) {
	ctx.JSON(http.StatusOK, Result{
		Success:  false,
		ErrorMsg: errorMsg,
	})
}
