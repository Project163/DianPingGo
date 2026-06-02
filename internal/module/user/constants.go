package user

import (
	"time"
)

const (
	BizUserLoginCode    = "login:code:"   // 存储登录验证码的Redis key模板，%s为手机号占位符
	BizUserLoginCodeTTL = 5 * time.Minute // 登录验证码的过期时间

	BizUserToken    = "login:token:"   // 存储用户登录Token的Redis key模板，%d为用户ID占位符
	BizUserTokenTTL = 30 * time.Minute // 用户登录Token的过期时间

	BizUserLockCode = "login:lock:"   // 存储登录锁定状态的Redis key模板，%s为手机号占位符
	BizUserLockTTL  = 1 * time.Minute // 登录锁定的持续时间
)
