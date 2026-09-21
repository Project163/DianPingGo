package seckillvoucher

import (
	"context"
	"log/slog"
	"time"
)

type InitRunner interface {
	Tick(context.Context) error
}

type InitWorker struct {
	service InitRunner
	log     *slog.Logger
}

func NewInitWorker(service InitRunner, logger *slog.Logger) *InitWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &InitWorker{
		service: service,
		log:     logger,
	}
}

// Run 是阻塞方法，由 App 在独立 goroutine 中运行。
// ctx 取消后退出，App 等待退出再关闭数据库和 Redis。
func (w *InitWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		if ctx.Err() != nil {
			return
		}

		if err := w.service.Tick(ctx); err != nil && ctx.Err() == nil {
			w.log.Error("扫描秒杀初始化任务失败", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
