package validator

import "regexp"

var (
	mobileRegex = regexp.MustCompile(`^1[3-9]\d{9}$`) // 简单的中国大陆手机号正则表达式

	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`) // 简单的邮箱正则表达式

	usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{3,20}$`) // 用户名正则表达式，允许字母、数字和下划线，长度3-20
)

func ValidateMobile(phone string) bool {
	return mobileRegex.MatchString(phone)
}
