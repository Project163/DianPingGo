package user

import (
	"context"
	"dianping/internal/middleware"
	"dianping/internal/module/userinfo"
	"dianping/pkg/errmsg"
	"dianping/pkg/response"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
)

type UserService interface {
	Sign(ctx context.Context, userID uint64) error
	SignCount(ctx context.Context, userID uint64) (int, error)
	Login(ctx context.Context, req *LoginReq) (*LoginResp, error)
	CodeLogin(ctx context.Context, req *CodeLoginReq) (*LoginResp, error)
	SendCode(ctx context.Context, req *SendCodeReq) (*SendCodeResp, error)
	GetUserByID(ctx context.Context, userID uint64) (*UserDTO, error)
}

type Handler struct {
	userSrv     UserService
	userInfoSrv userinfo.Service
}

func NewHandler(userSrv UserService, userInfoSrv userinfo.Service) *Handler {
	return &Handler{
		userSrv:     userSrv,
		userInfoSrv: userInfoSrv,
	}
}

// Login 用户登录处理函数
func (h *Handler) Login(ctx *gin.Context) {
	var req LoginReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}

	resp, err := h.userSrv.Login(ctx.Request.Context(), &req)
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

	resp, err := h.userSrv.CodeLogin(ctx.Request.Context(), &req)
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

	resp, err := h.userSrv.SendCode(ctx.Request.Context(), &req)
	if err != nil {
		response.Fail(ctx, err)
		return
	}

	response.OK(ctx, resp)
}

func (h *Handler) Sign(ctx *gin.Context) {
	userIDAny, exists := ctx.Get(middleware.CtxUserIDKey)
	if !exists {
		response.Fail(ctx, &errmsg.ErrUnauthorized)
		return
	}
	userID, ok := userIDAny.(uint64)
	if !ok {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, fmt.Errorf("invalid userID")))
		return
	}
	err := h.userSrv.Sign(ctx.Request.Context(), userID)
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, nil)
}

func (h *Handler) SignCount(ctx *gin.Context) {
	userIDAny, exists := ctx.Get(middleware.CtxUserIDKey)
	if !exists {
		response.Fail(ctx, &errmsg.ErrUnauthorized)
		return
	}
	userID, ok := userIDAny.(uint64)
	if !ok {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, fmt.Errorf("invalid userID")))
		return
	}
	count, err := h.userSrv.SignCount(ctx.Request.Context(), userID)
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, map[string]interface{}{"count": count})
}

func (h *Handler) GetUserByID(ctx *gin.Context) {
	userIDStr := ctx.Param("id")
	if userIDStr == "" {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, fmt.Errorf("missing userID")))
		return
	}
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, fmt.Errorf("invalid userID")))
		return
	}

	userDTO, err := h.userSrv.GetUserByID(ctx.Request.Context(), userID)
	if err != nil {
		response.Fail(ctx, err)
		return
	}

	response.OK(ctx, userDTO)
}

func (h *Handler) GetSelf(ctx *gin.Context) {
	userIDAny, exists := ctx.Get(middleware.CtxUserIDKey)
	if !exists {
		response.Fail(ctx, &errmsg.ErrUnauthorized)
		return
	}
	userID, ok := userIDAny.(uint64)
	if !ok {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, fmt.Errorf("invalid userID")))
		return
	}
	userDTO, err := h.userSrv.GetUserByID(ctx.Request.Context(), userID)
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, userDTO)
}

func (h *Handler) GetUserInfoByID(ctx *gin.Context) {
	userIDStr := ctx.Param("id")
	if userIDStr == "" {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, fmt.Errorf("missing userID")))
		return
	}
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, fmt.Errorf("invalid userID")))
		return
	}

	userInfoDTO, err := h.userInfoSrv.GetUserInfoByUserID(ctx.Request.Context(), userID)
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	if userInfoDTO == nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrUserInfoNotFound, fmt.Errorf("user info not found")))
		return
	}

	response.OK(ctx, userInfoDTO)
}
