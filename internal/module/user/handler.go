package user

import (
	"dianping/pkg/errmsg"
	"dianping/pkg/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	srv *Service
}

func NewHandler(srv *Service) *Handler {
	return &Handler{srv: srv}
}

// Login 用户登录处理函数
func (h *Handler) Login(ctx *gin.Context) {
	var req LoginReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}

	resp, err := h.srv.Login(ctx.Request.Context(), &req)
	if err != nil {
		response.Fail(ctx, err)
		return
	}

	response.OK(ctx, resp)
}

// CodeLogin 使用验证码登录处理函数
func (h *Handler) CodeLogin(ctx *gin.Context) {
	var req CodeLoginReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}

	resp, err := h.srv.CodeLogin(ctx.Request.Context(), &req)
	if err != nil {
		response.Fail(ctx, err)
		return
	}

	response.OK(ctx, resp)
}

// SendCode 发送验证码处理函数
func (h *Handler) SendCode(ctx *gin.Context) {
	var req SendCodeReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}

	resp, err := h.srv.SendCode(ctx.Request.Context(), &req)
	if err != nil {
		response.Fail(ctx, err)
		return
	}

	response.OK(ctx, resp)
}
