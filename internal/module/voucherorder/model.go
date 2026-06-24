package voucherorder

import "time"

type VoucherOrder struct {
	// 订单ID不可自增
	// 原因在于规律性太明显，容易被攻击者猜测到订单ID，进而进行恶意攻击（如暴力破解订单信息、伪造订单等）。
	// 其次在业务量较大的情况下，单张表难以承载大量订单数据，使用自增ID可能会导致性能瓶颈和数据分布不均的问题。
	// 使用全局唯一ID（如UUID、雪花算法等）可以有效避免上述问题，提高系统的安全性和性能。
	// 全局ID需要保证唯一性和高性能生成，同时需要确保高可用性和安全性，最后最好保持递增性以优化数据库性能。
	ID         uint64    `gorm:"primaryKey;column:id" json:"id"`
	UserID     uint64    `gorm:"not null;column:user_id" json:"user_id"`
	VoucherID  uint64    `gorm:"not null;column:voucher_id" json:"voucher_id"`
	PayType    uint      `gorm:"not null;column:pay_type" json:"pay_type"`
	Status     uint      `gorm:"not null;column:status" json:"status"`
	CreateTime time.Time `gorm:"autoCreateTime;column:create_time" json:"-"`
	PayTime    time.Time `gorm:"column:pay_time" json:"pay_time,omitempty"`
	UseTime    time.Time `gorm:"column:use_time" json:"use_time,omitempty"`
	RefundTime time.Time `gorm:"column:refund_time" json:"refund_time,omitempty"`
	UpdateTime time.Time `gorm:"autoUpdateTime;column:update_time" json:"-"`
}

func (VoucherOrder) TableName() string {
	return "tb_voucher_order"
}
