package voucher

import "time"

// CreateVoucherReq 定义了创建优惠券请求的结构体，包含商户ID、标题、副标题、规则、支付金额、实际金额、类型、库存、开始时间和结束时间
type CreateVoucherReq struct {
	ShopID      uint64    `json:"shopId" binding:"min=1"`
	Title       string    `json:"title" binding:"required"`
	SubTitle    string    `json:"subTitle" binding:"required"`
	Rules       string    `json:"rules" binding:"required"`
	PayValue    uint64    `json:"payValue" binding:"min=1"`
	ActualValue uint64    `json:"actualValue" binding:"min=1"`
	Type        uint      `json:"type" binding:"oneof=0 1"`
	Stock       uint      `json:"stock" binding:"min=1"`
	BeginTime   time.Time `json:"beginTime" binding:"required"`
	EndTime     time.Time `json:"endTime" binding:"required,gtfield=BeginTime"`
}

// UpdateVoucherReq 定义了更新优惠券请求的结构体，包含优惠券ID、标题、副标题、规则、支付金额、实际金额、类型、库存、开始时间和结束时间
type VoucherResp struct {
	ID          uint64    `json:"id"`
	ShopID      uint64    `json:"shopId"`
	Title       string    `json:"title"`
	SubTitle    string    `json:"subTitle"`
	Rules       string    `json:"rules"`
	PayValue    uint64    `json:"payValue"`
	ActualValue uint64    `json:"actualValue"`
	Type        uint      `json:"type"`
	Status      uint      `json:"status"`
	Stock       uint      `json:"stock"`
	BeginTime   time.Time `json:"beginTime"`
	EndTime     time.Time `json:"endTime"`
}
