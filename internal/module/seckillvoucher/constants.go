package seckillvoucher

import "strconv"

const (
	PreparePending uint8 = iota // 0：准备中
	PrepareReady                // 1：准备完成
	PrepareFailed               // 2：准备失败
)

const (
	InitTaskPending uint8 = iota
	InitTaskProcessing
	InitTaskDone
	InitTaskFailed
)

const (
	InitCreated         = 0
	InitAlreadyPrepared = 1
	InitInvalid         = -1
	InitConflict        = -2
	InitDeadlinePassed  = -3
)

func ActivityKey(voucherID uint64) string {
	return "seckill:activity:" + strconv.FormatUint(voucherID, 10)
}
