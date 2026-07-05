package voucher

import "time"

// CreateVoucherReq 定义了创建优惠券请求的结构体，包含商户ID、标题、副标题、规则、支付金额、实际金额、类型、库存、开始时间和结束时间
type CreateVoucherReq struct {
	ShopID      uint64    `json:"shop_id" binding:"min=1"`
	Title       string    `json:"title" binding:"required"`
	SubTitle    string    `json:"sub_title" binding:"required"`
	Rules       string    `json:"rules" binding:"required"`
	PayValue    uint64    `json:"pay_value" binding:"min=1"`
	ActualValue uint64    `json:"actual_value" binding:"min=1"`
	Type        uint      `json:"type" binding:"oneof=0 1"`
	Stock       uint      `json:"stock" binding:"min=1"`
	BeginTime   time.Time `json:"begin_time" binding:"required"`
	EndTime     time.Time `json:"end_time" binding:"required,gtfield=BeginTime"`
}

// UpdateVoucherReq 定义了更新优惠券请求的结构体，包含优惠券ID、标题、副标题、规则、支付金额、实际金额、类型、库存、开始时间和结束时间
type VoucherResp struct {
	ID          uint64    `json:"id"`
	ShopID      uint64    `json:"shop_id"`
	Title       string    `json:"title"`
	SubTitle    string    `json:"sub_title"`
	Rules       string    `json:"rules"`
	PayValue    uint64    `json:"pay_value"`
	ActualValue uint64    `json:"actual_value"`
	Type        uint      `json:"type"`
	Status      uint      `json:"status"`
	Stock       uint      `json:"stock"`
	BeginTime   time.Time `json:"begin_time"`
	EndTime     time.Time `json:"end_time"`
}
