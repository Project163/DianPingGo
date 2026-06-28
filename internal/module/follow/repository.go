package follow

import (
	"context"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{
		db: db,
	}
}

func (r *Repository) ListFollowedUserIDs(ctx context.Context, userID uint64) ([]uint64, error) {
	var userIDs []uint64
	err := r.db.WithContext(ctx).
		Table("follow").
		Where("follow_user_id = ?", userID).
		Pluck("user_id", &userIDs).Error
	return userIDs, err
}
