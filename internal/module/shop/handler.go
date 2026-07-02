package shop

import (
	"dianping/pkg/errmsg"
	"dianping/pkg/response"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	srv *Service
}

func NewHandler(srv *Service) *Handler {
	return &Handler{srv: srv}
}

func (h *Handler) CreateShop(ctx *gin.Context) {
	var req CreateShopReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}

	shop := &Shop{
		Name:     req.Name,
		TypeID:   req.TypeID,
		Images:   req.Images,
		Area:     req.Area,
		Address:  req.Address,
		OpenTime: req.OpenTime,
	}

	err := h.srv.CreateShop(ctx.Request.Context(), shop)
	if err != nil {
		response.Fail(ctx, err)
		return
	}

	response.OK(ctx, shop)
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
	if typeIDStr == "" {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, fmt.Errorf("typeId is required")))
		return
	}
	typeID, err := strconv.ParseUint(typeIDStr, 10, 64)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}

	currentStr := ctx.DefaultQuery("current", "1")
	if currentStr == "" {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, fmt.Errorf("current is required")))
		return
	}
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

func (h *Handler) GetShopsByName(ctx *gin.Context) {
	name := ctx.Query("name")
	if name == "" {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, nil))
		return
	}

	currentStr := ctx.DefaultQuery("current", "1")
	current, err := strconv.Atoi(currentStr)
	if err != nil || current < 1 {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}

	shops, err := h.srv.GetShopsByName(ctx.Request.Context(), name, current)
	if err != nil {
		response.Fail(ctx, err)
		return
	}

	response.OK(ctx, shops)
}
