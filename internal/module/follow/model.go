package follow

import "time"

type Follow struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID       uint64    `gorm:"column:user_id;uniqueIndex:uk_user_follow" json:"userId"`
	FollowUserID uint64    `gorm:"column:follow_user_id;uniqueIndex:uk_user_follow;index:idx_follow_user_id" json:"followId"`
	CreateTime   time.Time `gorm:"column:create_time;autoCreateTime" json:"-"`
}

func (Follow) TableName() string {
	return "tb_follow"
}
