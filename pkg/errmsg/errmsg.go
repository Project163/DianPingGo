package errmsg

import (
	"fmt"
	"net/http"
)

// CustomError 定义一个包含HTTP状态码、业务错误码、错误信息和原始错误的结构体
type CustomError struct {
	HttpCode     int
	BusinessCode int
	Message      string
	Err          error
}

// Error 实现error接口，返回格式化的错误信息
func (e *CustomError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("HTTP %d, Business %d: %s - %v", e.HttpCode, e.BusinessCode, e.Message, e.Err)
	}
	return fmt.Sprintf("HTTP %d, Business %d: %s", e.HttpCode, e.BusinessCode, e.Message)
}

// Unwrap 实现Unwrap方法，返回原始错误
func (e *CustomError) Unwrap() error {
	return e.Err
}

// NewError 创建一个新的CustomError实例，接受一个基础错误和一个原始错误
func NewError(base CustomError, realErr error) *CustomError {
	return &CustomError{
		HttpCode:     base.HttpCode,
		BusinessCode: base.BusinessCode,
		Message:      base.Message,
		Err:          realErr,
	}
}

// 预定义一些常用的错误类型，包含HTTP状态码、业务错误码和错误信息
var (
	ErrInvalidParam    = CustomError{HttpCode: http.StatusBadRequest, BusinessCode: 4001, Message: "请求参数错误"}
	ErrNotFound        = CustomError{HttpCode: http.StatusNotFound, BusinessCode: 4002, Message: "资源不存在"}
	ErrUnauthorized    = CustomError{HttpCode: http.StatusUnauthorized, BusinessCode: 4003, Message: "未授权访问"}
	ErrInternalSec     = CustomError{HttpCode: http.StatusInternalServerError, BusinessCode: 5001, Message: "服务器内部错误"}
	ErrTooManyRequests = CustomError{HttpCode: http.StatusTooManyRequests, BusinessCode: 4006, Message: "请求过于频繁，请稍后再试"}

	ErrUserNotFound      = CustomError{HttpCode: http.StatusNotFound, BusinessCode: 4004, Message: "用户不存在"}
	ErrInvalidPassword   = CustomError{HttpCode: http.StatusUnauthorized, BusinessCode: 4005, Message: "密码错误"}
	ErrCodeExpired       = CustomError{HttpCode: http.StatusBadRequest, BusinessCode: 4007, Message: "验证码已过期"}
	ErrInvalidCode       = CustomError{HttpCode: http.StatusBadRequest, BusinessCode: 4008, Message: "验证码错误"}
	ErrUserAlreadyExists = CustomError{HttpCode: http.StatusConflict, BusinessCode: 4009, Message: "用户已存在"}
)
