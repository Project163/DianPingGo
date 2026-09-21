package seckillvoucher

import "time"

type SeckillVoucher struct {
	VoucherID     uint64 `gorm:"primaryKey;column:voucher_id" json:"voucher_id"`
	Stock         uint   `gorm:"not null;column:stock" json:"stock"`
	PrepareStatus uint8  `gorm:"not null;column:prepare_status" json:"prepare_status"`

	CreateTime time.Time `gorm:"autoCreateTime;column:create_time" json:"-"`
	BeginTime  time.Time `gorm:"not null;column:begin_time" json:"begin_time"`
	EndTime    time.Time `gorm:"not null;column:end_time" json:"end_time"`
	UpdateTime time.Time `gorm:"autoUpdateTime;column:update_time" json:"-"`
}

func (SeckillVoucher) TableName() string {
	return "tb_seckill_voucher"
}

type SeckillInit struct {
	ID          uint64     `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement" json:"id"`
	VoucherID   uint64     `gorm:"column:voucher_id;type:bigint unsigned;not null;uniqueIndex:uk_seckill_init_voucher" json:"voucher_id"`
	Status      uint8      `gorm:"column:status;type:tinyint unsigned;not null;default:0;index:idx_seckill_init_pending,priority:1;index:idx_seckill_init_processing,priority:1" json:"status"`
	Attempts    uint32     `gorm:"column:attempts;type:int unsigned;not null;default:0" json:"attempts"`
	NextRetryAt time.Time  `gorm:"column:next_retry_at;type:datetime(3);not null;index:idx_seckill_init_pending,priority:2" json:"next_retry_at"`
	LeaseUntil  *time.Time `gorm:"column:lease_until;type:datetime(3);index:idx_seckill_init_processing,priority:2" json:"lease_until,omitempty"`
	ClaimToken  string     `gorm:"column:claim_token;type:varchar(36);not null;default:''" json:"-"`
	LastError   string     `gorm:"column:last_error;type:varchar(1024);not null;default:''" json:"-"`

	// 初始化参数快照：创建后不随剩余库存变化。
	InitialStock uint32    `gorm:"column:initial_stock;type:int unsigned;not null" json:"initial_stock"`
	BeginTime    time.Time `gorm:"column:begin_time;type:datetime(3);not null" json:"begin_time"`
	EndTime      time.Time `gorm:"column:end_time;type:datetime(3);not null" json:"end_time"`

	CreateTime time.Time `gorm:"column:create_time;type:datetime(3);not null;autoCreateTime" json:"-"`
	UpdateTime time.Time `gorm:"column:update_time;type:datetime(3);not null;autoUpdateTime" json:"-"`
}

func (SeckillInit) TableName() string {
	return "tb_seckill_init_task"
}
