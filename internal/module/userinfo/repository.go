package userinfo

import (
	"context"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetUserInfoByUserID(ctx context.Context, userID uint64) (*UserInfo, error) {
	var userInfo UserInfo
	err := r.db.Where("user_id = ?", userID).First(&userInfo).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &userInfo, nil
}

func (r *Repository) CreateUserInfo(ctx context.Context, userInfoDTO *UserInfoDTO) error {
	userInfo := &UserInfo{
		UserID:    userInfoDTO.UserID,
		City:      userInfoDTO.City,
		Introduce: userInfoDTO.Introduce,
		Fans:      userInfoDTO.Fans,
		Followee:  userInfoDTO.Followee,
		Gender:    userInfoDTO.Gender,
		Birthday:  userInfoDTO.Birthday,
		Credits:   userInfoDTO.Credits,
		Level:     userInfoDTO.Level,
	}
	return r.db.Create(userInfo).Error
}
