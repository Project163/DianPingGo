package voucherorder

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

func (r *Repository) CreateVoucherOrder(ctx context.Context, order *VoucherOrder) error {
	return r.db.WithContext(ctx).Create(order).Error
}

func (r *Repository) CountByUserAndVoucher(ctx context.Context, userID uint64, voucherID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&VoucherOrder{}).
		Where("user_id = ? AND voucher_id = ?", userID, voucherID).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *Repository) GetVoucherOrderByID(ctx context.Context, orderID uint64) (*VoucherOrder, error) {
	var order VoucherOrder
	err := r.db.WithContext(ctx).Where("id = ?", orderID).First(&order).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}
