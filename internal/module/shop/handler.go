package shop

import (
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

// GetShopByID 获取商户信息
func (h *Handler) GetShopByID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}

	shop, err := h.srv.GetShopByID(ctx.Request.Context(), id)
	if err != nil {
		response.Fail(ctx, err)
		return
	}

	response.OK(ctx, shop)
}

// UpdateShop 更新商户信息
func (h *Handler) UpdateShop(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}

	var req UpdateShopReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}

	err = h.srv.Update(ctx.Request.Context(), id, &req)
	if err != nil {
		response.Fail(ctx, err)
		return
	}

	response.OK(ctx, nil)
}

// GetShopsByType 获取商户列表
func (h *Handler) GetShopsByType(ctx *gin.Context) {
	typeIDStr := ctx.Query("typeId")
	typeID, err := strconv.ParseUint(typeIDStr, 10, 64)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}

	currentStr := ctx.DefaultQuery("current", "1")
	current, err := strconv.Atoi(currentStr)
	if err != nil || current < 1 {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}

	var x, y *float64
	if xStr := ctx.Query("x"); xStr != "" {
		xVal, _ := strconv.ParseFloat(xStr, 64)
		x = &xVal
	}
	if yStr := ctx.Query("y"); yStr != "" {
		yVal, _ := strconv.ParseFloat(yStr, 64)
		y = &yVal
	}

	shops, err := h.srv.GetShopsByType(ctx.Request.Context(), typeID, current, x, y)
	if err != nil {
		response.Fail(ctx, err)
		return
	}

	response.OK(ctx, shops)
}
