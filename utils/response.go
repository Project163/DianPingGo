package utils

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Result struct {
	Success  bool        `json:"success"`
	ErrorMsg string      `json:"errormsg,omitempty"`
	Data     interface{} `json:"data,omitempty"`
	Total    int64       `json:"total,omitempty"`
}

func SuccessResult(message string) *Result {
	return &Result{
		Success: true,
		Data:    message,
	}
}

func SuccessResultWithData(data interface{}) *Result {
	return &Result{
		Success: true,
		Data:    data,
	}
}

func ErrorResult(errorMsg string) *Result {
	return &Result{
		Success:  false,
		ErrorMsg: errorMsg,
	}
}

func PageResult(data interface{}, total int64, page, size int) *Result {
	return &Result{
		Success: true,
		Data: gin.H{
			"list":  data,
			"total": total,
			"page":  page,
			"size":  size,
		},
	}
}

func Response(ctx *gin.Context, result *Result) {
	if result.Success {
		ctx.JSON(http.StatusOK, result)
	} else {
		ctx.JSON(http.StatusBadRequest, result)
	}
}

func SuccessResponse(ctx *gin.Context, data interface{}) {
	ctx.JSON(http.StatusOK, SuccessResultWithData(data))
}

func ErrorResponse(ctx *gin.Context, code int, message string) {
	ctx.JSON(code, ErrorResult(message))
}

func PageResponse(ctx *gin.Context, data interface{}, total int64, page, size int) {
	ctx.JSON(http.StatusOK, PageResult(data, total, page, size))
}
