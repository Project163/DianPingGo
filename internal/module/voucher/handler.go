package voucher

import (
	"context"
	"dianping/pkg/errmsg"
	"dianping/pkg/response"
	"strconv"

	"github.com/gin-gonic/gin"
)

// VoucherHandlerService defines the interface that the Handler depends on.
type VoucherHandlerService interface {
	CreateVoucher(ctx context.Context, voucher *Voucher) (uint64, error)
	CreateSeckillVoucher(ctx context.Context, svoucher *Voucher) (uint64, error)
	GetVoucherByID(ctx context.Context, id uint64) (*VoucherResp, error)
	GetVoucherByShopID(ctx context.Context, shopID uint64) ([]VoucherResp, error)
}

type Handler struct {
	srv VoucherHandlerService
}

func NewHandler(srv VoucherHandlerService) *Handler {
	return &Handler{srv: srv}
}

// CreateVoucher 处理创建普通优惠券请求，接受JSON格式的请求体，调用服务层进行创建，并返回结果
func (h *Handler) CreateVoucher(ctx *gin.Context) {
	var req CreateVoucherReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}
	voucher := &Voucher{
		ShopID:      req.ShopID,
		Title:       req.Title,
		SubTitle:    req.SubTitle,
		Rules:       req.Rules,
		PayValue:    req.PayValue,
		ActualValue: req.ActualValue,
		Type:        0, // 普通券
		Stock:       req.Stock,
		Status:      1, // 默认启用
		BeginTime:   req.BeginTime,
		EndTime:     req.EndTime,
	}
	id, err := h.srv.CreateVoucher(ctx.Request.Context(), voucher)
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
	voucher := &Voucher{
		ShopID:      req.ShopID,
		Type:        1, // 秒杀券
		Title:       req.Title,
		SubTitle:    req.SubTitle,
		Rules:       req.Rules,
		PayValue:    req.PayValue,
		ActualValue: req.ActualValue,
		Stock:       req.Stock,
		Status:      1, // 默认启用
		BeginTime:   req.BeginTime,
		EndTime:     req.EndTime,
	}
	id, err := h.srv.CreateSeckillVoucher(ctx.Request.Context(), voucher)
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, id)
}

func (h *Handler) GetVoucherByID(ctx *gin.Context) {
	voucherIDStr := ctx.Param("id")
	voucherID, err := strconv.ParseUint(voucherIDStr, 10, 64)
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}
	voucherResp, err := h.srv.GetVoucherByID(ctx.Request.Context(), voucherID)
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, voucherResp)
}

// GetVoucherByShopID 根据商户ID查询优惠券列表
func (h *Handler) GetVoucherByShopID(ctx *gin.Context) {
	shopIDStr := ctx.Param("shop_id")
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
