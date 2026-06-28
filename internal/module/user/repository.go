package user

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// Repository 定义了用户仓库接口，包含创建用户和根据手机号查询用户的方法
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建一个新的用户仓库实例，接受一个GORM数据库连接作为参数
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{
		db: db,
	}
}

// CreateUser 在数据库中创建一个新的用户记录，接受上下文和用户对象作为参数，返回错误
func (r *Repository) CreateUser(ctx context.Context, u *User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

// GetUserByPhone 根据手机号查询用户记录，接受上下文和手机号作为参数，返回用户对象和错误
func (r *Repository) GetUserByPhone(ctx context.Context, phone string) (*User, error) {
	var u User
	err := r.db.WithContext(ctx).Where("phone = ?", phone).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// GetUserByID 根据用户ID查询用户记录，接受上下文和用户ID作为参数，返回用户对象和错误
func (r *Repository) GetUserByID(ctx context.Context, userID uint64) (*User, error) {
	var u User
	err := r.db.WithContext(ctx).Where("id = ?", userID).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// ListUsersByIDs 根据用户ID列表批量查询用户记录，接受上下文和用户ID切片作为参数，返回用户对象切片和错误
func (r *Repository) ListUsersByIDs(ctx context.Context, userIDs []uint64) ([]User, error) {
	var users []User
	err := r.db.WithContext(ctx).Where("id IN ?", userIDs).Find(&users).Error
	if err != nil {
		return nil, err
	}
	return users, nil
}
