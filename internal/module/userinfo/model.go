package userinfo

import "time"

type UserInfo struct {
	UserID    uint64    `gorm:"column:user_id;primaryKey:autoIncrement:false;type:bigint(20) unsigned" json:"user_id"`
	City      string    `gorm:"column:city;type:varchar(64);not null;default:''" json:"city"`
	Introduce string    `gorm:"column:introduce;type:varchar(128);default:null" json:"introduce"`
	Fans      uint32    `gorm:"column:fans;type:int(8) unsigned;not null;default 0" json:"fans"`
	Followee  uint32    `gorm:"column:followee;type:int(8) unsigned;not null;default 0" json:"followee"`
	Gender    uint8     `gorm:"column:gender;type:tinyint(1) unsigned;not null;default 0" json:"gender"`
	Birthday  string    `gorm:"column:birthday;type:date;default:null" json:"birthday"`
	Credits   uint32    `gorm:"column:credits;type:int(8) unsigned;not null;default 0" json:"credits"`
	Level     uint8     `gorm:"column:level;type:tinyint(1) unsigned;not null;default 0" json:"level"`
	CreatedAt time.Time `gorm:"column:created_time;type:timestamp;not null;default CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_time;type:timestamp;not null;default CURRENT_TIMESTAMP" json:"updated_at"`
}

func (UserInfo) TableName() string {
	return "tb_user_info"
}
