package blog

import (
	"context"
	"errors"

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

// CreateBlog 创建博文
func (r *Repository) CreateBlog(ctx context.Context, blog *Blog) error {
	return r.db.WithContext(ctx).Create(blog).Error
}

// GetBlogByID 根据ID获取博文
func (r *Repository) GetBlogByID(ctx context.Context, id uint64) (*Blog, error) {
	var blog Blog
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&blog).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &blog, nil
}

// ListHotBlogs 获取热门博文列表，按点赞数降序排序
func (r *Repository) ListHotBlogs(ctx context.Context, offset, limit int) ([]Blog, error) {
	var blogs []Blog
	err := r.db.WithContext(ctx).
		Order("liked DESC").
		Offset(offset).
		Limit(limit).
		Find(&blogs).Error
	return blogs, err
}

// ListBlogsByUserID 获取用户的博文列表，按更新时间降序排序
func (r *Repository) ListBlogsByUserID(ctx context.Context, userID uint64, offset, limit int) ([]Blog, error) {
	var blogs []Blog
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("update_time DESC").
		Offset(offset).
		Limit(limit).
		Find(&blogs).Error
	return blogs, err
}

// IncrementLiked 原子增加博文点赞数
func (r *Repository) IncrementLiked(ctx context.Context, blogID uint64) error {
	return r.db.WithContext(ctx).Model(&Blog{}).
		Where("id = ?", blogID).
		UpdateColumn("liked", gorm.Expr("liked + ?", 1)).Error
}

// DecrementLiked 原子减少博文点赞数
func (r *Repository) DecrementLiked(ctx context.Context, blogID uint64) error {
	return r.db.WithContext(ctx).Model(&Blog{}).
		Where("id = ?", blogID).
		UpdateColumn("liked", gorm.Expr("liked - ?", 1)).Error
}

// ListBlogsByIDs 根据ID列表获取博文列表
func (r *Repository) ListBlogsByIDs(ctx context.Context, blogIDs []uint64) ([]Blog, error) {
	var blogs []Blog
	err := r.db.WithContext(ctx).
		Where("id IN ?", blogIDs).
		Find(&blogs).Error
	return blogs, err
}
