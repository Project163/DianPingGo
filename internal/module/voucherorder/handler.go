package voucherorder

import (
	"dianping/internal/middleware"
	"dianping/pkg/errmsg"
	"dianping/pkg/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	srv *Service
}

func NewHandler(srv *Service) *Handler {
	return &Handler{srv: srv}
}

func (h *Handler) SeckillVoucher(ctx *gin.Context) {
	userID, ok := ctx.Get(middleware.CtxUserIDKey)
	if !ok {
		response.Fail(ctx, &errmsg.ErrUnauthorized)
		return
	}

	var req SeckillReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Fail(ctx, &errmsg.ErrInvalidParam)
		return
	}

	orderID, err := h.srv.SeckillVoucher(ctx.Request.Context(), req.VoucherID, userID.(uint64))
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	response.OK(ctx, orderID)
}
