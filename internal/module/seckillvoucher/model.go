package seckillvoucher

import "time"

type SeckillVoucher struct {
	VoucherID uint64 `gorm:"primaryKey;column:voucher_id" json:"voucher_id"`
	Stock     uint   `gorm:"not null;column:stock" json:"stock"`

	CreateTime time.Time `gorm:"autoCreateTime;column:create_time" json:"-"`
	BeginTime  time.Time `gorm:"not null;column:begin_time" json:"begin_time"`
	EndTime    time.Time `gorm:"not null;column:end_time" json:"end_time"`
	UpdateTime time.Time `gorm:"autoUpdateTime;column:update_time" json:"-"`
}

func (SeckillVoucher) TableName() string {
	return "tb_seckill_voucher"
}
