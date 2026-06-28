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
	// 通用错误
	ErrInvalidParam    = CustomError{HttpCode: http.StatusBadRequest, BusinessCode: 4001, Message: "请求参数错误"}
	ErrNotFound        = CustomError{HttpCode: http.StatusNotFound, BusinessCode: 4002, Message: "资源不存在"}
	ErrUnauthorized    = CustomError{HttpCode: http.StatusUnauthorized, BusinessCode: 4010, Message: "未登陆或登录已过期"}
	ErrForbidden       = CustomError{HttpCode: http.StatusForbidden, BusinessCode: 4030, Message: "没有权限访问"}
	ErrTooManyRequests = CustomError{HttpCode: http.StatusTooManyRequests, BusinessCode: 4290, Message: "请求过于频繁，请稍后再试"}
	ErrInternalSec     = CustomError{HttpCode: http.StatusInternalServerError, BusinessCode: 5000, Message: "服务器内部错误"}

	// 用户模块错误
	ErrUserNotFound      = CustomError{HttpCode: http.StatusNotFound, BusinessCode: 4101, Message: "用户不存在"}
	ErrInvalidPhone      = CustomError{HttpCode: http.StatusBadRequest, BusinessCode: 4102, Message: "手机号格式错误"}
	ErrInvalidPassword   = CustomError{HttpCode: http.StatusUnauthorized, BusinessCode: 4103, Message: "密码错误"}
	ErrCodeExpired       = CustomError{HttpCode: http.StatusBadRequest, BusinessCode: 4104, Message: "验证码已过期"}
	ErrInvalidCode       = CustomError{HttpCode: http.StatusBadRequest, BusinessCode: 4105, Message: "验证码错误"}
	ErrUserAlreadyExists = CustomError{HttpCode: http.StatusConflict, BusinessCode: 4106, Message: "用户已存在"}

	// 商户模块错误
	ErrShopNotFound = CustomError{HttpCode: http.StatusNotFound, BusinessCode: 4201, Message: "商户不存在"}

	// 订单模块错误
	ErrNoStock       = CustomError{HttpCode: http.StatusBadRequest, BusinessCode: 4301, Message: "库存不足"}
	ErrRepeatedOrder = CustomError{HttpCode: http.StatusBadRequest, BusinessCode: 4302, Message: "请勿重复下单"}
	ErrOrderNotFound = CustomError{HttpCode: http.StatusNotFound, BusinessCode: 4303, Message: "订单不存在"}

	// 文件上传模块错误
	ErrFileTooLarge = CustomError{HttpCode: http.StatusBadRequest, BusinessCode: 4401, Message: "文件过大"}
	ErrFileType     = CustomError{HttpCode: http.StatusBadRequest, BusinessCode: 4402, Message: "文件类型不支持"}
	ErrFileUpload   = CustomError{HttpCode: http.StatusInternalServerError, BusinessCode: 4403, Message: "文件上传失败"}

	// 博客模块错误
	ErrBlogNotFound = CustomError{HttpCode: http.StatusNotFound, BusinessCode: 4501, Message: "博客不存在"}
)
