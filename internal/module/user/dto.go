package user

// UserDTO 用户数据传输对象
type UserDTO struct {
	ID       uint64 `json:"id"`
	NickName string `json:"nick_name"`
	Icon     string `json:"icon"`
}

// LoginReq 登录请求参数
type LoginReq struct {
	Phone    string `json:"phone" binding:"required,len=11,zh_mobile"`
	Password string `json:"password" binding:"required,min=6,max=20"`
}

// LoginResp 登录响应数据
type LoginResp struct {
	Token    string `json:"token"`
	NickName string `json:"nick_name"`
}

// SendCodeReq 发送验证码请求参数
type SendCodeReq struct {
	Phone string `json:"phone" form:"phone" binding:"required,len=11,zh_mobile"`
}

// SendCodeResp 发送验证码响应数据
type SendCodeResp struct {
	Message string `json:"message"`
}

// CodeLoginReq 验证码登录请求参数
type CodeLoginReq struct {
	Phone string `json:"phone" form:"phone" binding:"required,len=11,zh_mobile"`
	Code  string `json:"code" form:"code" binding:"required,len=6"`
}

// CreateUserReq 创建用户请求参数
type CreateUserReq struct {
	Phone    string `json:"phone" binding:"required,len=11,zh_mobile"`
	Password string `json:"password" binding:"required,min=6,max=20"`
	NickName string `json:"nick_name" binding:"required,min=2,max=20"`
}

// CreateUserResp 创建用户响应数据
type CreateUserResp struct {
	UserID uint64 `json:"user_id"`
}
