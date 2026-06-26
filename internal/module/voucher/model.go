package voucher

import "time"

// Voucher 定义了优惠券模型，包含ID、商户ID、标题、副标题、规则、支付金额、实际金额、类型、状态、库存、开始时间、结束时间、创建时间和更新时间
type Voucher struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	ShopID      uint64    `gorm:"not null;column:shop_id" json:"shop_id"`
	Title       string    `gorm:"type:varchar(255);not null;column:title" json:"title"`
	SubTitle    string    `gorm:"type:varchar(255);not null;column:sub_title" json:"sub_title"`
	Rules       string    `gorm:"type:varchar(255);not null;column:rules" json:"rules"`
	PayValue    uint64    `gorm:"not null;column:pay_value" json:"pay_value"`
	ActualValue uint64    `gorm:"not null;column:actual_value" json:"actual_value"`
	Type        uint      `gorm:"not null;column:type" json:"type"`
	Status      uint      `gorm:"not null;column:status" json:"status"`
	Stock       uint      `gorm:"not null;column:stock" json:"stock"`
	BeginTime   time.Time `gorm:"not null;column:begin_time" json:"begin_time"`
	EndTime     time.Time `gorm:"not null;column:end_time" json:"end_time"`
	CreateTime  time.Time `gorm:"autoCreateTime;column:create_time" json:"-"`
	UpdateTime  time.Time `gorm:"autoUpdateTime;column:update_time" json:"-"`
}

func (Voucher) TableName() string {
	return "tb_voucher"
}
