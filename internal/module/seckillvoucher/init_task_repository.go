package seckillvoucher

import (
	"context"
	"errors"
	"time"

	"dianping/internal/tx"

	"gorm.io/gorm"
)

type InitTaskRepository struct {
	db *gorm.DB
}

func NewInitTaskRepository(db *gorm.DB) *InitTaskRepository {
	return &InitTaskRepository{db: db}
}

func (r *InitTaskRepository) getDB(ctx context.Context) *gorm.DB {
	if db := tx.FromContext(ctx); db != nil {
		return db.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

// Candidates 只查询候选任务，不代表已经获得执行权。
// 分开查询，使两个联合索引都能发挥作用。
func (r *InitTaskRepository) Candidates(
	ctx context.Context,
	limit int,
) ([]uint64, error) {
	var expired []uint64
	err := r.getDB(ctx).Model(&SeckillInit{}).
		Where(
			"status = ? AND lease_until <= CURRENT_TIMESTAMP(3)",
			InitTaskProcessing,
		).
		Order("lease_until, id").
		Limit(limit).
		Pluck("id", &expired).Error
	if err != nil {
		return nil, err
	}

	var pending []uint64
	err = r.getDB(ctx).Model(&SeckillInit{}).
		Where(
			"status = ? AND next_retry_at <= CURRENT_TIMESTAMP(3)",
			InitTaskPending,
		).
		Order("next_retry_at, id").
		Limit(limit).
		Pluck("id", &pending).Error
	if err != nil {
		return nil, err
	}

	return append(expired, pending...), nil
}

// Claim 使用条件 UPDATE 争抢任务。
// 查询候选任务与这里之间允许发生并发变化。
func (r *InitTaskRepository) Claim(
	ctx context.Context,
	id uint64,
	token string,
	lease time.Duration,
) (*SeckillInit, error) {
	leaseUS := lease.Microseconds()

	result := r.getDB(ctx).Exec(`
		UPDATE tb_seckill_init_task
		SET status = ?,
		    claim_token = ?,
		    lease_until = TIMESTAMPADD(
		        MICROSECOND, ?, CURRENT_TIMESTAMP(3)
		    ),
		    attempts = attempts + 1,
		    update_time = CURRENT_TIMESTAMP(3)
		WHERE id = ?
		  AND (
		      (status = ? AND next_retry_at <= CURRENT_TIMESTAMP(3))
		      OR
		      (status = ? AND lease_until <= CURRENT_TIMESTAMP(3))
		  )
	`,
		InitTaskProcessing, token, leaseUS, id,
		InitTaskPending, InitTaskProcessing,
	)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, nil
	}

	var task SeckillInit
	err := r.getDB(ctx).
		Where(
			"id = ? AND status = ? AND claim_token = ? "+
				"AND lease_until > CURRENT_TIMESTAMP(3)",
			id, InitTaskProcessing, token,
		).
		Take(&task).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// Retry 只有当前租约持有者才能安排重试。
func (r *InitTaskRepository) Retry(
	ctx context.Context,
	task *SeckillInit,
	delay time.Duration,
	message string,
) (bool, error) {
	result := r.getDB(ctx).Model(&SeckillInit{}).
		Where(
			"id = ? AND status = ? AND claim_token = ? "+
				"AND lease_until > CURRENT_TIMESTAMP(3)",
			task.ID, InitTaskProcessing, task.ClaimToken,
		).
		Updates(map[string]any{
			"status": InitTaskPending,
			"next_retry_at": gorm.Expr(
				"TIMESTAMPADD(MICROSECOND, ?, CURRENT_TIMESTAMP(3))",
				delay.Microseconds(),
			),
			"claim_token": "",
			"lease_until": nil,
			"last_error":  truncateError(message),
			"update_time": gorm.Expr("CURRENT_TIMESTAMP(3)"),
		})

	return result.RowsAffected == 1, result.Error
}

// Finish 必须与活动准备状态更新放进同一个事务。
func (r *InitTaskRepository) Finish(
	ctx context.Context,
	task *SeckillInit,
	status uint8,
	message string,
) (bool, error) {
	result := r.getDB(ctx).Model(&SeckillInit{}).
		Where(
			"id = ? AND status = ? AND claim_token = ? "+
				"AND lease_until > CURRENT_TIMESTAMP(3)",
			task.ID, InitTaskProcessing, task.ClaimToken,
		).
		Updates(map[string]any{
			"status":      status,
			"claim_token": "",
			"lease_until": nil,
			"last_error":  truncateError(message),
			"update_time": gorm.Expr("CURRENT_TIMESTAMP(3)"),
		})

	return result.RowsAffected == 1, result.Error
}

func truncateError(message string) string {
	runes := []rune(message)
	if len(runes) > 1024 {
		runes = runes[:1024]
	}
	return string(runes)
}
