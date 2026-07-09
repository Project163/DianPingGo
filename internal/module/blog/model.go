package blog

import "time"

type Blog struct {
	ID         uint64    `gorm:"column:id;primaryKey" json:"id"`
	ShopID     uint64    `gorm:"column:shop_id" json:"shop_id"`
	UserId     uint64    `gorm:"column:user_id" json:"user_id"`
	Title      string    `gorm:"column:title" json:"title"`
	Images     string    `gorm:"column:images" json:"images"`
	Content    string    `gorm:"column:content" json:"content"`
	Liked      int       `gorm:"column:liked" json:"liked"`
	Comments   int       `gorm:"column:comments" json:"comments"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime" json:"create_time"`
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime" json:"update_time"`

	Icon   string `gorm:"-" json:"icon"`
	Name   string `gorm:"-" json:"name"`
	IsLike bool   `gorm:"-" json:"is_like"`
}

func (Blog) TableName() string {
	return "tb_blog"
}
