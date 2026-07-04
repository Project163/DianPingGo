package shoptype

import (
	"context"
	"dianping/pkg/errmsg"
	"dianping/pkg/response"
	"strconv"

	"github.com/gin-gonic/gin"
)

type ShopTypeService interface {
	CreateShopType(ctx context.Context, shopType *ShopType) error
	UpdateShopType(ctx context.Context, shopType *ShopType) error
	GetShopTypeByID(ctx context.Context, shopTypeId uint64) (*ShopType, error)
	GetShopTypeAll(ctx context.Context) ([]ShopType, error)
}

type Handler struct {
	srv ShopTypeService
}

func NewHandler(srv ShopTypeService) *Handler {
	return &Handler{
		srv: srv,
	}
}

func (h *Handler) CreateShopType(ctx *gin.Context) {
	var req CreateShopTypeReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Fail(ctx, &errmsg.ErrInvalidParam)
		return
	}

	shopType := &ShopType{
		Name: req.Name,
		Icon: req.Icon,
		Sort: req.Sort,
	}
	err := h.srv.CreateShopType(ctx, shopType)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInternalSec, err))
		return
	}
	response.OK(ctx, nil)
}

func (h *Handler) UpdateShopType(ctx *gin.Context) {
	var req CreateShopTypeReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Fail(ctx, &errmsg.ErrInvalidParam)
		return
	}

	shopType := &ShopType{
		Name: req.Name,
		Icon: req.Icon,
		Sort: req.Sort,
	}
	err := h.srv.UpdateShopType(ctx, shopType)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInternalSec, err))
		return
	}
	response.OK(ctx, nil)
}

func (h *Handler) GetShopTypeByID(ctx *gin.Context) {
	shopTypeIDStr := ctx.Param("id")
	shopTypeID, err := strconv.ParseUint(shopTypeIDStr, 10, 64)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}
	shopType, err := h.srv.GetShopTypeByID(ctx, shopTypeID)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInternalSec, err))
		return
	}

	response.OK(ctx, shopType)
}

func (h *Handler) GetShopTypeAll(ctx *gin.Context) {
	shopTypes, err := h.srv.GetShopTypeAll(ctx)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInternalSec, err))
		return
	}
	response.OK(ctx, shopTypes)
}
