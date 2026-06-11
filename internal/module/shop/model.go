package shop

import "time"

// Shop 商户模型
type Shop struct {
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Name       string    `gorm:"column:name" json:"name"`
	TypeID     uint64    `gorm:"column:type_id" json:"typeId"`
	Images     string    `gorm:"column:images" json:"images"`
	Area       string    `gorm:"column:area" json:"area"`
	Address    string    `gorm:"column:address" json:"address"`
	Longitude  float64   `gorm:"column:x" json:"longitude"`
	Latitude   float64   `gorm:"column:y" json:"latitude"`
	AvgPrice   uint64    `gorm:"column:avg_price" json:"avgPrice"`
	Sold       uint      `gorm:"column:sold" json:"sold"`
	Comments   uint      `gorm:"column:comment" json:"comment"`
	Score      uint      `gorm:"column:score" json:"score"`
	OpenTime   string    `gorm:"column:open_hours" json:"openTime"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime" json:"-"`
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime" json:"-"`

	Distance float64 `gorm:"-" json:"distance,omitempty"`
}

// TableName 指定 Shop 模型对应的数据库表名
func (Shop) TableName() string {
	return "tb_shop"
}
