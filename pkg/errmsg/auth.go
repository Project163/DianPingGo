package errmsg

import "net/http"

var (
	ErrDependencyUnavailable = CustomError{HttpCode: http.StatusServiceUnavailable, BusinessCode: 5030, Message: "服务不可用"}
	ErrSessionCorrupted      = CustomError{HttpCode: http.StatusInternalServerError, BusinessCode: 5001, Message: "会话异常"}
	ErrRequestTimeout        = CustomError{HttpCode: http.StatusGatewayTimeout, BusinessCode: 5040, Message: "请求超时"}
)
