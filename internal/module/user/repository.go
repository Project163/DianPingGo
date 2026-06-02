package user

import (
	"context"
	"dianping/internal/infra"
	"errors"

	"gorm.io/gorm"
)

type Repository struct{}

func NewRepository() *Repository {
	return &Repository{}
}

func (r *Repository) CreateUser(ctx context.Context, u *User) error {
	return infra.DB.WithContext(ctx).Create(u).Error
}

func (r *Repository) GetUserByPhone(ctx context.Context, phone string) (*User, error) {
	var u User
	err := infra.DB.WithContext(ctx).Where("phone = ?", phone).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}
