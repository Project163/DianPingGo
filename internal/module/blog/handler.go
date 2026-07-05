package blog

import (
	"dianping/internal/middleware"
	"dianping/pkg/errmsg"
	"dianping/pkg/response"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	srv *Service
}

func NewHandler(srv *Service) *Handler {
	return &Handler{srv: srv}
}

// CreateBlog 创建博文，需要登录，从ctx中获取当前用户ID
func (h *Handler) CreateBlog(ctx *gin.Context) {
	var req CreateBlogReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}
	currentUserID, exists := ctx.Get(middleware.CtxUserIDKey)
	if !exists {
		response.Fail(ctx, &errmsg.ErrUnauthorized)
		return
	}
	blog := &Blog{
		UserId:  currentUserID.(uint64),
		ShopID:  req.ShopID,
		Title:   req.Title,
		Content: req.Content,
		Images:  req.Images,
	}

	_, err := h.srv.CreateBlog(ctx.Request.Context(), blog)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInternalSec, err))
		return
	}
	response.OK(ctx, nil)
}

// LikeBlog 点赞博文，需要登录，从ctx中获取当前用户ID
func (h *Handler) LikeBlog(ctx *gin.Context) {
	blogIDStr := ctx.Param("id")
	blogID, err := strconv.ParseUint(blogIDStr, 10, 64)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}
	userID, exists := ctx.Get(middleware.CtxUserIDKey)
	if !exists {
		response.Fail(ctx, &errmsg.ErrUnauthorized)
		return
	}
	err = h.srv.LikeBlog(ctx.Request.Context(), blogID, userID.(uint64))
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, nil)
}

// GetBlogSelf 获取当前用户的博文列表，需要登录，从ctx中获取当前用户ID
func (h *Handler) GetBlogSelf(ctx *gin.Context) {
	userID, exists := ctx.Get(middleware.CtxUserIDKey)
	if !exists {
		response.Fail(ctx, &errmsg.ErrUnauthorized)
		return
	}
	currentStr := ctx.DefaultQuery("current", "1")
	current, err := strconv.Atoi(currentStr)
	if err != nil || current < 1 {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}
	blogs, err := h.srv.QueryBlogsByUserID(ctx.Request.Context(), userID.(uint64), userID.(uint64), current)
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, blogs)
}

// GetBlogsHot 获取热门博文列表，不需要登录，但也可以从ctx中获取当前用户ID，如果没有登录，则userID为0，不进入点赞状态检查逻辑
func (h *Handler) GetBlogsHot(ctx *gin.Context) {
	currentUserID, _ := ctx.Get(middleware.CtxUserIDKey)
	userID, _ := currentUserID.(uint64)
	currentStr := ctx.DefaultQuery("current", "1")
	current, err := strconv.Atoi(currentStr)
	if err != nil || current < 1 {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}
	blogs, err := h.srv.QueryHotBlog(ctx.Request.Context(), userID, current)
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, blogs)
}

// GetBlogByUserID 获取指定用户的博文列表，需要登录，从ctx中获取当前用户ID
func (h *Handler) GetBlogByUserID(ctx *gin.Context) {
	userIDStr := ctx.Param("id")
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}
	currentUserID, exists := ctx.Get(middleware.CtxUserIDKey)
	if !exists {
		response.Fail(ctx, &errmsg.ErrUnauthorized)
		return
	}
	currentStr := ctx.DefaultQuery("current", "1")
	current, err := strconv.Atoi(currentStr)
	if err != nil || current < 1 {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}
	blogs, err := h.srv.QueryBlogsByUserID(ctx.Request.Context(), userID, currentUserID.(uint64), current)
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, blogs)
}

// GetBlogLikesByID 获取指定博文的点赞列表，不需要登录
func (h *Handler) GetBlogLikesByID(ctx *gin.Context) {
	blogIDStr := ctx.Param("id")
	blogID, err := strconv.ParseUint(blogIDStr, 10, 64)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}
	likes, err := h.srv.QueryBlogLikesByID(ctx.Request.Context(), blogID)
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, likes)
}

// GetBlogByID 获取指定博文的详细信息，需要登录，从ctx中获取当前用户ID
func (h *Handler) GetBlogByID(ctx *gin.Context) {
	blogIDStr := ctx.Param("id")
	currentUserID, exists := ctx.Get(middleware.CtxUserIDKey)
	if !exists {
		response.Fail(ctx, &errmsg.ErrUnauthorized)
		return
	}
	userID, _ := currentUserID.(uint64)
	blogID, err := strconv.ParseUint(blogIDStr, 10, 64)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}

	blog, err := h.srv.QueryBlogByID(ctx.Request.Context(), blogID, userID)
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, blog)
}

// GetBlogOfFollow 获取当前用户关注的博文列表，需要登录，从ctx中获取当前用户ID
func (h *Handler) GetBlogOfFollow(ctx *gin.Context) {
	currentUserID, exists := ctx.Get(middleware.CtxUserIDKey)
	if !exists {
		response.Fail(ctx, &errmsg.ErrUnauthorized)
		return
	}
	lastIDStr := ctx.DefaultQuery("last_id", "0")
	lastID, err := strconv.ParseInt(lastIDStr, 10, 64)
	if err != nil || lastID < 1 {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}
	offsetStr := ctx.DefaultQuery("offset", "0")
	offset, err := strconv.ParseInt(offsetStr, 10, 64)
	if err != nil || offset < 0 {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}
	blogs, err := h.srv.QueryBlogsOfFollow(ctx.Request.Context(), currentUserID.(uint64), lastID, offset)
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, blogs)
}
