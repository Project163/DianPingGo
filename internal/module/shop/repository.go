package shop

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

func (r *Repository) GetShopByID(ctx context.Context, id uint64) (*Shop, error) {
	var shop Shop
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&shop).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &shop, nil
}

func (r *Repository) UpdateShop(ctx context.Context, shop *Shop) error {
	err := r.db.WithContext(ctx).Save(shop).Error
	if err != nil {
		return err
	}
	return nil
}

func (r *Repository) GetShopsByType(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error) {
	var shops []Shop
	err := r.db.WithContext(ctx).
		Where("type_id = ?", typeID).
		Offset(offset).
		Limit(limit).
		Find(&shops).Error

	return shops, err
}

func (r *Repository) GetShopsByIDs(ctx context.Context, ids []uint64) ([]Shop, error) {
	var shops []Shop
	err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Find(&shops).Error

	return shops, err
}
