package follow

import (
	"context"
	"dianping/internal/middleware"
	"dianping/internal/module/user"
	"dianping/pkg/errmsg"
	"dianping/pkg/response"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
)

type FollowService interface {
	Follow(ctx context.Context, follow *Follow) error
	Unfollow(ctx context.Context, follow *Follow) error
	IsFollowed(ctx context.Context, userID, followUserID uint64) (bool, error)
	FollowCommon(ctx context.Context, userID1, userID2 uint64) ([]user.UserDTO, error)
	ListFollowedUserIDs(ctx context.Context, userID uint64) ([]uint64, error)
	ListFollowerUserIDs(ctx context.Context, userID uint64) ([]uint64, error)
}

type Handler struct {
	srv FollowService
}

func NewHandler(srv FollowService) *Handler {
	return &Handler{
		srv: srv,
	}
}

// Follow 处理关注和取消关注请求，接受用户ID和关注状态(isFollow)作为参数，从ctx中获取当前用户ID
func (h *Handler) Follow(ctx *gin.Context) {
	followUserIDStr := ctx.Param("id")
	if followUserIDStr == "" {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, fmt.Errorf("missing followUserId")))
		return
	}
	followUserID, err := strconv.ParseUint(followUserIDStr, 10, 64)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}

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

	isFollowStr := ctx.Param("is_follow")
	isFollow, err := strconv.ParseBool(isFollowStr)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, fmt.Errorf("invalid isFollow")))
		return
	}

	follow := &Follow{
		UserID:       userID,
		FollowUserID: followUserID,
	}

	if isFollow {
		if err := h.srv.Follow(ctx.Request.Context(), follow); err != nil {
			response.Fail(ctx, err)
			return
		}
	} else {
		if err := h.srv.Unfollow(ctx.Request.Context(), follow); err != nil {
			response.Fail(ctx, err)
			return
		}
	}
	response.OK(ctx, nil)
}

// IsFollowed 处理查询是否关注请求，接受用户ID作为参数，从ctx中获取当前用户ID
func (h *Handler) IsFollowed(ctx *gin.Context) {
	followUserIDStr := ctx.Param("id")
	if followUserIDStr == "" {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, fmt.Errorf("missing followUserId")))
		return
	}
	followUserID, err := strconv.ParseUint(followUserIDStr, 10, 64)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}
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

	isFollowed, err := h.srv.IsFollowed(ctx.Request.Context(), userID, followUserID)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInternalSec, err))
		return
	}
	response.OK(ctx, gin.H{"is_followed": isFollowed})
}

// FollowCommon 处理查询共同关注请求，接受用户ID作为参数，从ctx中获取当前用户ID
func (h *Handler) FollowCommon(ctx *gin.Context) {
	userID1Any, exists := ctx.Get(middleware.CtxUserIDKey)
	if !exists {
		response.Fail(ctx, &errmsg.ErrUnauthorized)
		return
	}
	userID1, ok := userID1Any.(uint64)
	if !ok {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, fmt.Errorf("invalid userID1")))
		return
	}

	userID2Str := ctx.Param("id")
	if userID2Str == "" {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, fmt.Errorf("missing userID2")))
		return
	}
	userID2, err := strconv.ParseUint(userID2Str, 10, 64)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}

	commonUsers, err := h.srv.FollowCommon(ctx.Request.Context(), userID1, userID2)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInternalSec, err))
		return
	}
	response.OK(ctx, commonUsers)
}

// ListFollowedUserIDs 获取指定用户的所有关注用户ID
func (h *Handler) ListFollowedUserIDs(ctx *gin.Context) {
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

	followedIDs, err := h.srv.ListFollowedUserIDs(ctx, userID)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInternalSec, err))
		return
	}
	response.OK(ctx, followedIDs)
}

// ListFollowerUserIDs 获取指定用户的所有粉丝用户ID
func (h *Handler) ListFollowerUserIDs(ctx *gin.Context) {
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

	followedIDs, err := h.srv.ListFollowerUserIDs(ctx, userID)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInternalSec, err))
		return
	}
	response.OK(ctx, followedIDs)
}
