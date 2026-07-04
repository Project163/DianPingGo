package shoptype

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateShopType(ctx context.Context, shopType *ShopType) error {
	return r.db.WithContext(ctx).Create(shopType).Error
}

func (r *Repository) UpdateShopType(ctx context.Context, shopType *ShopType) error {
	err := r.db.WithContext(ctx).Model(shopType).Select("*").Updates(shopType).Error
	if err != nil {
		return err
	}
	return nil
}

func (r *Repository) GetShopTypeByID(ctx context.Context, shopTypeId uint64) (*ShopType, error) {
	var shopType ShopType
	err := r.db.WithContext(ctx).Where("ID = ?", shopTypeId).First(&shopType).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &shopType, nil
}

func (r *Repository) GetShopTypeAll(ctx context.Context) ([]ShopType, error) {
	var shopTypes []ShopType
	err := r.db.WithContext(ctx).Find(&shopTypes).Error
	return shopTypes, err
}
