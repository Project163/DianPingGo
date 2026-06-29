package follow

import (
	"context"
	"dianping/pkg/errmsg"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{
		db: db,
	}
}

// Follow 添加关注关系，如果已经存在则不做任何操作
func (r *Repository) Follow(ctx context.Context, userID, followUserID uint64) (bool, error) {
	if userID == followUserID {
		return false, errmsg.NewError(errmsg.ErrFollowYourself, nil)
	}
	follow := &Follow{
		UserID:       userID,
		FollowUserID: followUserID,
	}
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			DoNothing: true,
		}).
		Create(follow)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// Unfollow 删除关注关系，如果不存在则不做任何操作
func (r *Repository) Unfollow(ctx context.Context, userID, followUserID uint64) (bool, error) {
	if userID == followUserID {
		return false, errmsg.NewError(errmsg.ErrFollowYourself, nil)
	}
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND follow_user_id = ?", userID, followUserID).
		Delete(&Follow{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// IsFollowed 检查用户是否关注了指定用户
func (r *Repository) IsFollowed(ctx context.Context, userID, followUserID uint64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&Follow{}).
		Where("user_id = ? AND follow_user_id = ?", userID, followUserID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ListFollowerUserIDs 获取指定用户的所有粉丝用户ID
func (r *Repository) ListFollowerUserIDs(ctx context.Context, userID uint64) ([]uint64, error) {
	var userIDs []uint64
	err := r.db.WithContext(ctx).
		Model(&Follow{}).
		Where("follow_user_id = ?", userID).
		Pluck("user_id", &userIDs).Error
	return userIDs, err
}

// ListFollowedUserIDs 获取指定用户的所有关注用户ID
func (r *Repository) ListFollowedUserIDs(ctx context.Context, userID uint64) ([]uint64, error) {
	var userIDs []uint64
	err := r.db.WithContext(ctx).
		Model(&Follow{}).
		Where("user_id = ?", userID).
		Pluck("follow_user_id", &userIDs).Error
	return userIDs, err
}
