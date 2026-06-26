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

// CreateVoucherOrder 在数据库中创建一个新的优惠券订单记录，接受上下文和优惠券订单对象作为参数，返回错误
func (r *Repository) CreateVoucherOrder(ctx context.Context, order *VoucherOrder) error {
	return r.db.WithContext(ctx).Create(order).Error
}

// CountByUserAndVoucher 统计用户购买某个优惠券的数量，接受上下文、用户ID和优惠券ID作为参数，返回购买数量和错误
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

// GetVoucherOrderByID 根据订单ID查询优惠券订单，接受上下文和订单ID作为参数，返回优惠券订单对象和错误
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
