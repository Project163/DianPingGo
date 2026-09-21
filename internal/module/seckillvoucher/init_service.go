package seckillvoucher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"dianping/internal/tx"
	"dianping/pkg/errmsg"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type InitTaskStore interface {
	Candidates(context.Context, int) ([]uint64, error)
	Claim(context.Context, uint64, string, time.Duration) (*SeckillInit, error)
	Retry(context.Context, *SeckillInit, time.Duration, string) (bool, error)
	Finish(context.Context, *SeckillInit, uint8, string) (bool, error)
}

type PrepareStore interface {
	SetPrepareStatus(context.Context, uint64, uint8) (bool, error)
}

type InitService struct {
	tasks    InitTaskStore
	vouchers PrepareStore
	rdb      redis.Cmdable
	tx       tx.Manager
	log      *slog.Logger
}

func NewInitService(
	tasks InitTaskStore,
	vouchers PrepareStore,
	rdb redis.Cmdable,
	manager tx.Manager,
	logger *slog.Logger,
) *InitService {
	if logger == nil {
		logger = slog.Default()
	}
	return &InitService{
		tasks:    tasks,
		vouchers: vouchers,
		rdb:      rdb,
		tx:       manager,
		log:      logger,
	}
}

func (s *InitService) Tick(ctx context.Context) error {
	queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	ids, err := s.tasks.Candidates(queryCtx, 20)
	cancel()

	if err != nil {
		return errmsg.NewError(errmsg.ErrInternalSec, err)
	}

	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// 每条任务独立超时。查询到候选 ID 后才逐条领取，
		// 避免后面的任务在本地队列里等待到租约过期。
		taskCtx, taskCancel := context.WithTimeout(ctx, 4*time.Second)
		err := s.process(taskCtx, id)
		taskCancel()

		if err != nil && ctx.Err() == nil {
			s.log.Error("秒杀初始化任务处理失败",
				"task_id", id,
				"error", err,
			)
		}
	}

	return nil
}

func (s *InitService) process(ctx context.Context, id uint64) error {
	task, err := s.tasks.Claim(ctx, id, uuid.NewString(), 10*time.Second)
	if err != nil {
		return errmsg.NewError(errmsg.ErrInternalSec, err)
	}
	if task == nil {
		return nil
	}

	// 初始化身份固定；领取令牌每次改变，两者用途不同。
	initToken := fmt.Sprintf("seckill-init:%d", task.ID)

	redisCtx, cancel := context.WithTimeout(ctx, time.Second)
	code, redisErr := initScript.Run(
		redisCtx,
		s.rdb,
		[]string{ActivityKey(task.VoucherID)},
		initToken,
		task.InitialStock,
		task.BeginTime.UnixMilli(),
		task.EndTime.UnixMilli(),
	).Int()
	cancel()

	if redisErr != nil {
		// 使用父 ctx，不复用已经超时的 redisCtx。
		// 父 ctx 也失效时不强行回写，等待租约回收。
		if ctx.Err() != nil {
			return ctx.Err()
		}

		_, retryErr := s.tasks.Retry(
			ctx, task, retryDelay(task.Attempts), redisErr.Error(),
		)
		if retryErr != nil {
			return errmsg.NewError(
				errmsg.ErrInternalSec,
				errors.Join(redisErr, retryErr),
			)
		}

		if task.Attempts >= 5 || !time.Now().Before(task.BeginTime) {
			s.log.Warn("秒杀初始化结果仍未确认",
				"task_id", task.ID,
				"voucher_id", task.VoucherID,
				"attempts", task.Attempts,
				"error", redisErr,
			)
		}
		return nil
	}

	switch code {
	case InitCreated, InitAlreadyPrepared:
		return s.finish(ctx, task, InitTaskDone, PrepareReady, "")

	case InitInvalid, InitConflict, InitDeadlinePassed:
		message := fmt.Sprintf("Redis 初始化明确失败，返回码=%d", code)
		if err := s.finish(
			ctx, task, InitTaskFailed, PrepareFailed, message,
		); err != nil {
			return err
		}

		s.log.Error("秒杀初始化需要处理",
			"task_id", task.ID,
			"voucher_id", task.VoucherID,
			"reason", message,
		)
		return nil

	default:
		// 协议未知也不直接认定未初始化，保留核查线索。
		message := fmt.Sprintf("未知初始化返回码=%d", code)
		_, err := s.tasks.Retry(
			ctx, task, retryDelay(task.Attempts), message,
		)
		if err != nil {
			return errmsg.NewError(errmsg.ErrInternalSec, err)
		}
		s.log.Error(message, "task_id", task.ID)
		return nil
	}
}

func (s *InitService) finish(
	ctx context.Context,
	task *SeckillInit,
	taskStatus uint8,
	prepareStatus uint8,
	message string,
) error {
	err := s.tx.Transaction(ctx, func(txctx context.Context) error {
		owned, err := s.tasks.Finish(
			txctx, task, taskStatus, message,
		)
		if err != nil {
			return err
		}
		if !owned {
			// 已失去租约，不能继续修改活动状态。
			return nil
		}

		changed, err := s.vouchers.SetPrepareStatus(
			txctx, task.VoucherID, prepareStatus,
		)
		if err != nil {
			return err
		}
		if !changed {
			// 回滚 Finish，避免只完成任务却未更新活动。
			return errmsg.NewError(
				errmsg.ErrInternalSec,
				fmt.Errorf(
					"秒杀券准备状态冲突，voucher_id=%d",
					task.VoucherID,
				),
			)
		}
		return nil
	})
	if err != nil {
		return errmsg.NewError(errmsg.ErrInternalSec, err)
	}
	return nil
}

func retryDelay(attempts uint32) time.Duration {
	if attempts == 0 {
		attempts = 1
	}

	shift := attempts - 1
	if shift > 5 {
		shift = 5
	}

	base := time.Second * time.Duration(1<<shift)
	if base > 30*time.Second {
		base = 30 * time.Second
	}

	// 总间隔约为 1、2、4、8、16、30 秒，再加少量抖动。
	return base + time.Duration(rand.Int64N(int64(250*time.Millisecond)))
}

var initScript = redis.NewScript(`
local key = KEYS[1]
local token = ARGV[1]
local initial = tonumber(ARGV[2])
local beginMs = tonumber(ARGV[3])
local endMs = tonumber(ARGV[4])

if token == ''
    or not initial
    or initial <= 0
    or initial > 2147483647
    or initial ~= math.floor(initial)
    or not beginMs
    or not endMs
    or endMs <= beginMs then
    return -1
end

local keyType = redis.call('TYPE', key).ok

if keyType ~= 'none' then
    if keyType ~= 'hash' then
        return -2
    end

    local data = redis.call('HMGET', key,
        'init_token', 'state', 'initial_stock',
        'begin_ms', 'end_ms', 'stock')

    local remaining = tonumber(data[6])

    if data[1] == token
        and data[2] == 'READY'
        and tonumber(data[3]) == initial
        and tonumber(data[4]) == beginMs
        and tonumber(data[5]) == endMs
        and remaining
        and remaining >= 0
        and remaining <= initial
        and remaining == math.floor(remaining) then
        return 1
    end

    -- 发现冲突时禁止继续使用该活动，不覆盖库存。
    -- 本版本不支持重新发布或多个活动版本。
    if data[2] == 'READY' then
        redis.call('HSET', key, 'state', 'BLOCKED')
    end
    return -2
end

local t = redis.call('TIME')
local nowMs = tonumber(t[1]) * 1000
    + math.floor(tonumber(t[2]) / 1000)

-- 已初始化的重试在上面返回成功。
-- 只有首次初始化受这个截止时间限制。
if nowMs >= beginMs then
    return -3
end

redis.call('HSET', key,
    'init_token', token,
    'state', 'READY',
    'initial_stock', ARGV[2],
    'stock', ARGV[2],
    'begin_ms', ARGV[3],
    'end_ms', ARGV[4])

return 0
`)
