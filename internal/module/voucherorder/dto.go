package voucherorder

type SeckillReq struct {
	VoucherID uint64 `json:"voucherId" binding:"min=1"`
}
