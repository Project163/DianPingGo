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

// CreateVoucher 处理创建普通优惠券请求，接受JSON格式的请求体，调用服务层进行创建，并返回结果
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

// CreateSeckillVoucher 处理创建秒杀优惠券请求，接受JSON格式的请求体，调用服务层进行创建，并返回结果
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

// GetVoucherByShopID 根据商户ID查询优惠券列表
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
