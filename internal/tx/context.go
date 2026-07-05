package tx

import (
	"context"

	"gorm.io/gorm"
)

type ctxKey struct{}

// WithTx 将 GORM 事务实例注入 context
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, ctxKey{}, tx)
}

// FormContext 从当前 context 中提取事务实例
func FromContext(ctx context.Context) *gorm.DB {
	tx, _ := ctx.Value(ctxKey{}).(*gorm.DB)
	return tx
}
