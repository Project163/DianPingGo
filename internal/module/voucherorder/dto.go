package voucherorder

// SeckillReq 定义了秒杀请求的结构体，包含优惠券ID
type SeckillReq struct {
	VoucherID uint64 `json:"voucher_id" binding:"min=1"`
}
