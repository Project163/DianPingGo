package voucherorder

import (
	"context"
	"errors"
	"log"
	"strconv"
	"sync"
	"time"

	"dianping/internal/module/seckillvoucher"
	"dianping/pkg/errmsg"
	"dianping/pkg/idgen"

	"github.com/redis/go-redis/v9"
)

type VoucherOrderRepository interface {
	CreateVoucherOrder(ctx context.Context, order *VoucherOrder) error
	CountByUserAndVoucher(ctx context.Context, userID uint64, voucherID uint64) (int64, error)
	GetVoucherOrderByID(ctx context.Context, orderID uint64) (*VoucherOrder, error)
}

type SeckillVoucherRepository interface {
	GetSeckillVoucherByID(ctx context.Context, voucherID uint64) (*seckillvoucher.SeckillVoucher, error)
	DeductStock(ctx context.Context, voucherID uint64) (bool, error)
}

// 先用SISMEMBER来判断用户是否已经下过单
// 如果已经下过单，返回2表示重复下单
// 如果没下过单，继续判断库存是否足够
// 如果库存不足，返回1表示库存不足
// 如果库存足够，扣减库存，SADD用户ID到订单集合中，XADD消息到Stream中，返回0表示下单成功
var SeckillLuaScript = redis.NewScript(`
local voucherId = ARGV[1]
local userId = ARGV[2]
local orderId = ARGV[3]

local stockKey = 'seckill:stock:' .. voucherId
local orderKey = 'seckill:order:' .. voucherId .. ':' .. userId
local streamKey = 'stream:orders'

local exists = redis.call('SISMEMBER', orderKey, userId)
if exists == 1 then
    return 2
end

local stock = redis.call('GET', stockKey)
if not stock or tonumber(stock) <= 0 then
    return 1
end

redis.call('DECRBY', stockKey, 1)

redis.call('SADD', orderKey, userId)

redis.call('XADD', streamKey, '*', 'userId', userId, 'voucherId', voucherId, 'orderId', orderId)

return 0
`)

var unlockScript = redis.NewScript(`
local key = KEYS[1]
local token = ARGV[1]

if redis.call("GET", key) == token then
	return redis.call("DEL", key)
else
	return 0
end
`)

type Service struct {
	rdb         redis.Cmdable
	repo        VoucherOrderRepository
	seckillRepo SeckillVoucherRepository
	idWorker    *idgen.RedisIDWorker

	mu      sync.Mutex
	started bool

	ctx    context.Context
	cancel context.CancelFunc
}

func NewService(repo VoucherOrderRepository, seckillRepo SeckillVoucherRepository, rdb redis.Cmdable, idWorker *idgen.RedisIDWorker) *Service {
	return &Service{
		repo:        repo,
		seckillRepo: seckillRepo,
		rdb:         rdb,
		idWorker:    idWorker,
	}
}

// Start 启动服务，创建消费者组并启动消息消费 goroutine，消费者组的消息来自Lua脚本(XADD)
func (s *Service) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return
	}
	s.started = true
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// 在服务启动时创建消费者组，组键为 StreamOrderKey，组名为 StreamGroupName，起始ID为 "0"
	s.rdb.XGroupCreateMkStream(context.Background(), StreamOrderKey, StreamGroupName, "0")

	go s.consumeNewMessages()
	go s.consumePendingMessages()
}

func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.started = false
}

// SeckillVoucher 处理秒杀优惠券请求，接受优惠券ID和用户ID作为参数，调用Lua脚本进行秒杀逻辑，并返回订单ID或错误
func (s *Service) SeckillVoucher(ctx context.Context, voucherID uint64, userID uint64) (int64, error) {
	// 用随机ID生成器生成一个唯一的订单ID，作为订单的标识
	orderID, err := s.idWorker.NextID(ctx, "order")
	if err != nil {
		return 0, err
	}
	// 用Lua脚本来处理秒杀逻辑，会返回一个整数表示结果，0表示成功，1表示库存不足，2表示重复下单
	// Lua本身控制的是缓存，通过XADD传递消息到数据库
	// 数据库的订单处理逻辑在StreamConsumer中异步处理
	res, err := SeckillLuaScript.Run(ctx, s.rdb, []string{}, voucherID, userID, orderID).Int()

	if err != nil {
		return 0, err
	}

	switch res {
	case SeckillSuccess:
		return orderID, nil
	case SeckillNoStock:
		return 0, &errmsg.ErrNoStock
	case SeckillRepeatedOrder:
		return 0, &errmsg.ErrRepeatedOrder
	default:
		return 0, &errmsg.ErrInternalSec
	}
}

// CreateVoucherOrder 在数据库中创建一个新的优惠券订单记录，接受上下文和优惠券订单对象作为参数，返回错误
func (s *Service) CreateVoucherOrder(ctx context.Context, order *VoucherOrder) error {
	ok, err := s.seckillRepo.DeductStock(ctx, order.VoucherID)
	if err != nil {
		return err
	}
	if !ok {
		return &errmsg.ErrNoStock
	}
	return s.repo.CreateVoucherOrder(ctx, order)
}

// GetVoucherOrderByID 根据订单ID查询优惠券订单，接受上下文和订单ID作为参数，返回优惠券订单对象和错误
func (s *Service) GetVoucherOrderByID(ctx context.Context, orderID uint64) (*VoucherOrder, error) {
	order, err := s.repo.GetVoucherOrderByID(ctx, orderID)
	if order == nil {
		return nil, &errmsg.ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return order, nil
}

// HandleVoucherOrder 处理优惠券订单，实现一人一单
func (s *Service) HandleVoucherOrder(ctx context.Context, order *VoucherOrder) error {
	// 对于单条订单，用分布式锁来控制同一用户的并发请求，锁的键为 LockOrderKey + userID，过期时间为 10 秒
	lockKey := LockOrderKey + strconv.FormatUint(order.UserID, 10)
	ok, err := s.rdb.SetNX(ctx, lockKey, order.UserID, 10*time.Second).Result()
	if err != nil {
		return err
	}
	if !ok {
		return &errmsg.ErrRepeatedOrder
	}
	defer func() {
		unlockScript.Run(ctx, s.rdb, []string{lockKey}, order.UserID)
	}()

	return s.CreateVoucherOrder(ctx, order)
}

func (s *Service) consumeNewMessages() {
	// 处理新消息的逻辑
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			// 读取新消息并处理
		}

		msgs, err := s.rdb.XReadGroup(s.ctx, &redis.XReadGroupArgs{
			Group:    StreamGroupName,
			Consumer: StreamConsumerName,
			Streams:  []string{StreamOrderKey, ">"},
			Count:    1,
			Block:    2 * time.Second,
		}).Result()

		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Printf("读取Stream新消息失败：%v", err)
			continue
		}

		for _, msg := range msgs[0].Messages {
			order := ParseMessage(msg.Values)
			s.processMessage(s.ctx, msg.ID, order)
		}
	}
}

func (s *Service) consumePendingMessages() {
	// 处理待处理消息的逻辑
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			s.processPendingList()
			return
		case <-ticker.C:
			s.processPendingList()
		}
	}
}

func (s *Service) processPendingList() {
	ctx := context.Background()
	// 通过“0”得到已投递但未处理的消息列表
	msgs, err := s.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    StreamGroupName,
		Consumer: StreamConsumerName,
		Streams:  []string{StreamOrderKey, "0"},
		Count:    int64(PendingBatchSize),
		Block:    0,
	}).Result()

	if err != nil || len(msgs) == 0 || len(msgs[0].Messages) == 0 {
		return
	}
	// 对于每条待处理的消息，进行处理
	for _, msg := range msgs[0].Messages {
		order := ParseMessage(msg.Values)
		s.processMessage(ctx, msg.ID, order)
	}
}

// processMessage 处理单条消息，调用 HandleVoucherOrder 方法处理订单，并根据处理结果进行确认或重试
func (s *Service) processMessage(ctx context.Context, msgID string, order *VoucherOrder) {
	if order == nil {
		// 对于无法解析的消息，直接确认并删除，避免阻塞消费者组
		s.rdb.XAck(ctx, StreamOrderKey, StreamGroupName, msgID)
		return
	}

	err := s.HandleVoucherOrder(ctx, order)
	if err == nil {
		// 处理成功，确认消息
		s.rdb.XAck(ctx, StreamOrderKey, StreamGroupName, msgID)
		s.rdb.HDel(ctx, RetryKey, msgID)
	}

	// 判断是否为永久性错误，如果是，则将消息移动到死信队列，并确认消息
	if isPermanentError(err) {
		s.moveToDLQ(ctx, msgID, order, err, -1)
		s.rdb.XAck(ctx, StreamOrderKey, StreamGroupName, msgID)
		s.rdb.HDel(ctx, RetryKey, msgID)
		log.Printf("[voucherorder] 订单 %d 重试%d次后仍重试，已加入DLQ：%v",
			order.ID, -1, err)
		return
	}
	// 对于非永久性错误，增加重试计数
	retryCount, _ := s.rdb.HIncrBy(ctx, RetryKey, msgID, 1).Result()
	if retryCount > MaxRetries {
		// 超过最大重试次数，将消息移动到死信队列，并确认消息
		s.moveToDLQ(ctx, msgID, order, err, int(retryCount))
		s.rdb.XAck(ctx, StreamOrderKey, StreamGroupName, msgID)
		s.rdb.HDel(ctx, RetryKey, msgID)
		log.Printf("[voucherorder] 订单 %d 重试%d次后仍重试，已加入DLQ：%v",
			order.ID, retryCount, err)
		return
	}

	log.Printf("[voucherorder] 订单 %d 处理失败(第%d/%d次): %v，留在PEL等待重试",
		order.ID, 1, MaxRetries, err)
}

func isPermanentError(err error) bool {
	var ce *errmsg.CustomError
	// 使用 errors.As 检查错误类型，如果是自定义错误类型，则根据具体的错误类型判断是否为永久性错误
	if errors.As(err, &ce) {
		switch {
		case ce == &errmsg.ErrNoStock:
			return true
		case ce == &errmsg.ErrRepeatedOrder:
			return true
		case ce == &errmsg.ErrInvalidParam:
			return true
		default:
			return false
		}
	}
	return false
}

// moveToDLQ 将处理失败的消息移动到死信队列，记录原始消息ID、订单信息、错误信息和重试次数
func (s *Service) moveToDLQ(ctx context.Context, msgID string, order *VoucherOrder, err error, retryCount int) {
	s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: DeadStreamKey,
		Values: map[string]interface{}{
			"original_id": msgID,
			"userId":      order.UserID,
			"voucherId":   order.VoucherID,
			"orderId":     order.ID,
			"error":       err.Error(),
			"retry_count": retryCount,
			"moved_at":    time.Now().Unix(),
		},
	})
}

// ParseMessage 解析Redis Stream消息，将消息内容转换为VoucherOrder对象，便于后续处理
func ParseMessage(values map[string]interface{}) *VoucherOrder {
	if values == nil {
		return nil
	}
	vid, _ := strconv.ParseUint(strVal(values["voucherId"]), 10, 64)
	uid, _ := strconv.ParseUint(strVal(values["userId"]), 10, 64)
	oid, _ := strconv.ParseUint(strVal(values["orderId"]), 10, 64)

	if vid == 0 || uid == 0 || oid == 0 {
		return nil
	}
	return &VoucherOrder{
		ID:        oid,
		UserID:    uid,
		VoucherID: vid,
	}
}

// strVal 将interface{}类型的值转换为字符串，如果值为nil则返回空字符串
func strVal(v interface{}) string {
	if v == nil {
		return ""
	}
	return v.(string)
}
