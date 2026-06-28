package blog

import "time"

type Blog struct {
	ID         uint64    `gorm:"column:id;primaryKey" json:"id"`
	ShopID     uint64    `gorm:"column:shop_id" json:"shopId"`
	UserId     uint64    `gorm:"column:user_id" json:"userId"`
	Title      string    `gorm:"column:title" json:"title"`
	Images     string    `gorm:"column:images" json:"images"`
	Content    string    `gorm:"column:content" json:"content"`
	Liked      int       `gorm:"column:liked" json:"liked"`
	Comments   int       `gorm:"column:comments" json:"comments"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time" json:"updateTime"`

	Icon   string `gorm:"-" json:"icon"`
	Name   string `gorm:"-" json:"name"`
	IsLike bool   `gorm:"-" json:"isLike"`
}

func (Blog) TableName() string {
	return "tb_blog"
}
