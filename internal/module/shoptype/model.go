package shoptype

import "time"

type ShopType struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Name       string    `gorm:"type:varchar(255);not null;column:name" json:"name"`
	Icon       string    `gorm:"type:varchar(255);not null;column:icon" json:"icon"`
	Sort       uint      `gorm:"not null;column:sort" json:"sort"`
	CreateTime time.Time `gorm:"autoCreateTime;column:create_time" json:"-"`
	UpdateTime time.Time `gorm:"autoUpdateTime;column:update_time" json:"-"`
}

func (ShopType) TableName() string {
	return "tb_shop_type"
}
