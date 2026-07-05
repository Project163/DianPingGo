package seckillvoucher

import (
	"context"
	"dianping/internal/tx"

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

func (r *Repository) getDB(ctx context.Context) *gorm.DB {
	if txDB := tx.FromContext(ctx); txDB != nil {
		return txDB.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

func (r *Repository) CreateSeckillVoucher(ctx context.Context, sv *SeckillVoucher) error {
	return r.getDB(ctx).Create(sv).Error
}

func (r *Repository) GetSeckillVoucherByID(ctx context.Context, voucherID uint64) (*SeckillVoucher, error) {
	var sv SeckillVoucher
	err := r.getDB(ctx).Where("voucher_id = ?", voucherID).First(&sv).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &sv, nil
}

// InnoDB行锁来避免超卖问题，只有当stock > 0时才会扣减库存
// 教程中教学了使用乐观锁来避免超卖问题，但UPDATE的语句在MySQL中默认是使用行锁的
// 所以修改库存的操作默认是串行的线程安全的，使用乐观锁反而会增加额外的开销
// 因为乐观锁需要不断重试，而行锁会自动排队等待锁释放
// 但问题在于悲观锁在极大的高并发下会导致大量的阻塞问题
// 所以前面有一层Redis的Lua脚本来保证库存扣减的原子性，避免了悲观锁的阻塞问题
func (r *Repository) DeductStock(ctx context.Context, voucherID uint64) (bool, error) {
	result := r.getDB(ctx).Model(&SeckillVoucher{}).
		Where("voucher_id = ? AND stock > 0", voucherID).
		Update("stock", gorm.Expr("stock - ?", 1))
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}
