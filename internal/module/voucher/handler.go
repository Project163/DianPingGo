package voucher

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

func (h *Handler) CreateVoucher(ctx *gin.Context) {
	var req CreateVoucherReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}
	id, err := h.srv.CreateVoucher(ctx.Request.Context(), &req)
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, id)
}

func (h *Handler) CreateSeckillVoucher(ctx *gin.Context) {
	var req CreateVoucherReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}
	id, err := h.srv.CreateSeckillVoucher(ctx.Request.Context(), &req)
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, id)
}

func (h *Handler) GetVoucherByShopID(ctx *gin.Context) {
	shopIDStr := ctx.Param("shopid")
	shopID, err := strconv.ParseUint(shopIDStr, 10, 64)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}
	vouchers, err := h.srv.GetVoucherByShopID(ctx.Request.Context(), shopID)
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, vouchers)
}
