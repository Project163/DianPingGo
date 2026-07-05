package tx

import (
	"context"

	"gorm.io/gorm"
)

type Manager interface {
	Transaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// GormManager 基于 GORM 的事务管理器实现
type GormManager struct {
	db *gorm.DB
}

func NewGormManager(db *gorm.DB) *GormManager {
	return &GormManager{db: db}
}

// Transaction 开启 GORM 事务，将 tx 注入 context 后执行事务
func (m *GormManager) Transaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txctx := WithTx(ctx, tx)
		return fn(txctx)
	})
}
