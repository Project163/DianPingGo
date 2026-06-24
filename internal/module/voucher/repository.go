package voucher

import (
	"context"
	"dianping/internal/module/seckillvoucher"
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

func (r *Repository) CreateVoucher(ctx context.Context, voucher *Voucher) error {
	return r.db.WithContext(ctx).Create(voucher).Error
}

func (r *Repository) CreateSeckillVoucher(ctx context.Context, v *Voucher, sv *seckillvoucher.SeckillVoucher) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(v).Error; err != nil {
			return err
		}
		sv.VoucherID = v.ID
		if err := tx.Create(sv).Error; err != nil {
			return err
		}
		return nil
	})
}

func (r *Repository) GetVoucherByID(ctx context.Context, id uint64) (*Voucher, error) {
	var voucher Voucher
	err := r.db.WithContext(ctx).First(&voucher, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &voucher, nil
}

func (r *Repository) GetByShopID(ctx context.Context, shopID uint64) ([]Voucher, error) {
	var vouchers []Voucher
	err := r.db.WithContext(ctx).Where("shop_id = ?", shopID).Order("create_time DESC").Find(&vouchers).Error
	if err != nil {
		return nil, err
	}
	return vouchers, nil
}
