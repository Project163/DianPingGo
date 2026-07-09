package voucherorder

const (
	StreamOrderKey     = "stream:orders" // 订单消息队列
	StreamGroupName    = "g1"            // 消费组名称
	StreamConsumerName = "c1"            // 消费者名称

	LockOrderKey = "lock:order:" // 分布式锁的Redis key模板

	SeckillSuccess       = 0 // 秒杀成功
	SeckillNoStock       = 1 // 秒杀失败，库存不足
	SeckillRepeatedOrder = 2 // 秒杀失败，重复下单

	MaxRetries       = 3                    // 最大重试次数
	RetryKey         = "stream:order:retry" // 重试计数的Redis key
	DeadStreamKey    = "stream:order:dead"  // 死信队列的Redis key
	PendingBatchSize = 5                    // 批量处理待处理消息的数量
)
