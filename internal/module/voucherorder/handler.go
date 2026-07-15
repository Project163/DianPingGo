package voucherorder

import (
	"context"
	"dianping/internal/middleware"
	"dianping/pkg/errmsg"
	"dianping/pkg/response"
	"strconv"

	"github.com/gin-gonic/gin"
)

// VoucherOrderHandlerService defines the interface that the Handler depends on.
type VoucherOrderHandlerService interface {
	SeckillVoucher(ctx context.Context, voucherID uint64, userID uint64) (int64, error)
	GetVoucherOrderByID(ctx context.Context, orderID uint64) (*VoucherOrder, error)
}

type Handler struct {
	srv VoucherOrderHandlerService
}

func NewHandler(srv VoucherOrderHandlerService) *Handler {
	return &Handler{srv: srv}
}

// SeckillVoucher 处理秒杀优惠券请求，接受JSON格式的请求体，调用服务层进行秒杀，并返回结果
func (h *Handler) SeckillVoucher(ctx *gin.Context) {
	userID, ok := ctx.Get(middleware.CtxUserIDKey)
	if !ok {
		response.Fail(ctx, &errmsg.ErrUnauthorized)
		return
	}

	voucherIdStr := ctx.Param("voucher_id")
	if voucherIdStr == "" {
		response.Fail(ctx, &errmsg.ErrInvalidParam)
		return
	}
	voucherId, err := strconv.ParseUint(voucherIdStr, 10, 64)
	if err != nil || voucherId == 0 {
		response.Fail(ctx, &errmsg.ErrInvalidParam)
		return
	}

	orderID, err := h.srv.SeckillVoucher(ctx.Request.Context(), voucherId, userID.(uint64))
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, orderID)
}

func (h *Handler) GetVoucherOrderByID(ctx *gin.Context) {
	orderIdStr := ctx.Param("voucher_id")
	if orderIdStr == "" {
		response.Fail(ctx, &errmsg.ErrInvalidParam)
		return
	}
	orderId, err := strconv.ParseUint(orderIdStr, 10, 64)
	if err != nil {
		response.Fail(ctx, &errmsg.ErrInvalidParam)
		return
	}
	orderResp, err := h.srv.GetVoucherOrderByID(ctx.Request.Context(), orderId)
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, orderResp)
}
