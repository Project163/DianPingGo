package user

import "time"

// User 用户模型
type User struct {
	ID         uint64    `gorm:"column:id;primaryKey" json:"id"`
	Phone      string    `gorm:"column:phone;unique" json:"-"`
	Password   string    `gorm:"column:password" json:"-"`
	NickName   string    `gorm:"column:nickname" json:"nickname"`
	Icon       string    `gorm:"column:icon" json:"icon"`
	CreateTime time.Time `gorm:"autoCreateTime" json:"-"`
	UpdateTime time.Time `gorm:"autoUpdateTime" json:"-"`
}

// TableName 指定 User 模型对应的数据库表名
func (User) TableName() string {
	return "tb_user"
}
