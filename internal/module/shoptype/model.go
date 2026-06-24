package shoptype

type ShopType struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Name       string `gorm:"type:varchar(255);not null;column:name" json:"name"`
	Icon       string `gorm:"type:varchar(255);not null;column:icon" json:"icon"`
	Sort       uint   `gorm:"not null;column:sort" json:"sort"`
	CreateTime int64  `gorm:"autoCreateTime;column:create_time" json:"-"`
	UpdateTime int64  `gorm:"autoUpdateTime;column:update_time" json:"-"`
}
